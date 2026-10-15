package service

import (
	"errors"

	"webcurl-desktop/internal/config"
	"webcurl-desktop/internal/updater"
)

// RestartingCode 由 CheckAndInstall 在更新已就绪、应用即将重启以完成安装时返回。
// 前端正是匹配这个字符串来展示"正在重启"的提示。
const RestartingCode = "restarting"

// UpdateService 向前端暴露与更新相关的功能。
// checkFn 在构造时注入，因此本包完全不依赖 app 包，
// 也不需要外部的接线钩子来补全自身状态。
type UpdateService struct {
	cfg     *config.Config
	checkFn func() error
}

// NewUpdateService 返回面向 cfg 的更新服务。
// checkFn 为执行一次完整更新流程的回调；正常启动流程下必然非 nil，
// 为 nil 时 CheckAndInstall 会返回"未初始化"，仅作为防御性兜底。
func NewUpdateService(cfg *config.Config, checkFn func() error) *UpdateService {
	return &UpdateService{cfg: cfg, checkFn: checkFn}
}

// GetVersion 返回当前运行的版本字符串。
func (s *UpdateService) GetVersion() string {
	return s.cfg.App.Version
}

// CheckAndInstall 触发完整更新流程。操作失败时返回错误信息；
// 已是最新返回空串；更新已就绪、应用即将重启时返回 RestartingCode。
func (s *UpdateService) CheckAndInstall() string {
	if s.cfg.Update.Provider == "" {
		return "update not configured"
	}
	if s.checkFn == nil {
		return "updater not initialised"
	}

	err := s.checkFn()
	switch {
	case err == nil:
		return ""
	case errors.Is(err, updater.ErrRestartRequired):
		return RestartingCode
	default:
		return err.Error()
	}
}
