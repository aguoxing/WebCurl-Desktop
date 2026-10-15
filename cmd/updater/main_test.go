package main

import (
	"errors"
	"flag"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOptionsValidate 覆盖参数校验的每一条拒绝分支。
// 助手是被程序启动的，参数错误只可能来自调用方改动，
// 因此这里必须"宁可报错也不带错参数去动安装目录"。
func TestOptionsValidate(t *testing.T) {
	src := t.TempDir()
	target := t.TempDir()

	cases := []struct {
		name    string
		opts    options
		wantErr string // 空串表示应当通过
	}{
		{
			name: "合法参数",
			opts: options{source: src, target: target, exe: "app", timeout: time.Second},
		},
		{
			name:    "缺少 source",
			opts:    options{target: target, exe: "app", timeout: time.Second},
			wantErr: "缺少 --source",
		},
		{
			name:    "缺少 target",
			opts:    options{source: src, exe: "app", timeout: time.Second},
			wantErr: "缺少 --target",
		},
		{
			name:    "缺少 exe",
			opts:    options{source: src, target: target, timeout: time.Second},
			wantErr: "缺少 --exe",
		},
		{
			name:    "exe 含目录分隔符",
			opts:    options{source: src, target: target, exe: filepath.Join("sub", "app"), timeout: time.Second},
			wantErr: "不能包含路径",
		},
		{
			name:    "exe 含上级目录引用",
			opts:    options{source: src, target: target, exe: filepath.Join("..", "app"), timeout: time.Second},
			wantErr: "不能包含路径",
		},
		{
			name:    "timeout 为零",
			opts:    options{source: src, target: target, exe: "app"},
			wantErr: "必须为正数",
		},
		{
			name:    "timeout 为负",
			opts:    options{source: src, target: target, exe: "app", timeout: -time.Second},
			wantErr: "必须为正数",
		},
		{
			name:    "source 不存在",
			opts:    options{source: filepath.Join(src, "nope"), target: target, exe: "app", timeout: time.Second},
			wantErr: "不可用",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate() = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validate() = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// TestParseOptionsDefaults 固定参数的默认值。
// 助手是被程序启动的，默认值（尤其是 --timeout）一旦漂移，调用方与实现对不上就会出现
// "更新卡住"或"过早放弃"这类只在真实升级时暴露的问题。
func TestParseOptionsDefaults(t *testing.T) {
	opts, err := parseOptions([]string{"-source", "/s", "-target", "/t", "-exe", "app"}, io.Discard)
	if err != nil {
		t.Fatalf("parseOptions() error: %v", err)
	}

	if opts.pid != 0 {
		t.Fatalf("pid 默认值 = %d, want 0", opts.pid)
	}
	if opts.timeout != defaultTimeout {
		t.Fatalf("timeout 默认值 = %s, want %s", opts.timeout, defaultTimeout)
	}
	if opts.noRestart {
		t.Fatal("no-restart 默认应为 false")
	}
	if opts.logPath != defaultLogPath() {
		t.Fatalf("log 默认值 = %q, want %q", opts.logPath, defaultLogPath())
	}
}

// TestParseOptionsAllFlags 逐个覆盖参数名，防止拼写与类型漂移。
func TestParseOptionsAllFlags(t *testing.T) {
	opts, err := parseOptions([]string{
		"-source", "/s",
		"-target", "/t",
		"-exe", "app",
		"-pid", "4242",
		"-timeout", "7s",
		"-no-restart",
		"-log", "/tmp/u.log",
	}, io.Discard)
	if err != nil {
		t.Fatalf("parseOptions() error: %v", err)
	}

	if opts.source != "/s" || opts.target != "/t" || opts.exe != "app" {
		t.Fatalf("必填参数解析错误: %+v", opts)
	}
	if opts.pid != 4242 {
		t.Fatalf("pid = %d, want 4242", opts.pid)
	}
	if opts.timeout != 7*time.Second {
		t.Fatalf("timeout = %s, want 7s", opts.timeout)
	}
	if !opts.noRestart {
		t.Fatal("no-restart 未被解析为 true")
	}
	if opts.logPath != "/tmp/u.log" {
		t.Fatalf("log = %q, want %q", opts.logPath, "/tmp/u.log")
	}
}

// TestParseOptionsUnknownFlag 验证未知参数会返回错误而不是被忽略：
// 静默忽略意味着调用方改了参数名却以为生效了。
func TestParseOptionsUnknownFlag(t *testing.T) {
	if _, err := parseOptions([]string{"-nope"}, io.Discard); err == nil {
		t.Fatal("parseOptions() = nil, want an error for an unknown flag")
	}
}

// TestParseOptionsHelp 验证 -h 返回 flag.ErrHelp，由 main 据此以 0 退出。
func TestParseOptionsHelp(t *testing.T) {
	_, err := parseOptions([]string{"-h"}, io.Discard)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parseOptions(-h) error = %v, want flag.ErrHelp", err)
	}
}
