// Package updater 实现 webcurl-desktop 的自动更新流水线。
//
// 每次更新都走同样的三个阶段：
//
//  1. 检查（Check）—— 向更新源询问是否存在比当前更新的版本。支持两种来源：
//     GitHub Releases（github.go）与自定义 HTTP API（check.go）。
//  2. 应用（Apply）—— 下载发布包，校验 Ed25519 签名与 SHA256 摘要，
//     解压到临时目录，然后拉起与当前可执行文件同目录的外部助手程序（apply.go）。
//  3. 退出（Exit）—— 由调用方关闭进程，助手程序才能覆盖二进制文件。
//     Apply 通过返回 ErrRestartRequired 来传达这一信号，自己绝不终止进程，
//     因此它既能被测试调用，也能直接在 GUI 事件回调里调用。
//
// Windows 无法覆盖正在运行的 .exe，所以最后的替换动作始终交给独立的助手进程完成。
// 协议格式与助手的命令行约定见 docs/updating.md。
package updater

import "errors"

// ErrRestartRequired 由 Apply 在助手程序启动后返回。关闭进程的责任在调用方：
// 本进程必须退出，助手程序才能替换正在运行的二进制。Apply 自身不会终止进程。
var ErrRestartRequired = errors.New("restart required to finish update")

// 硬性体积上限，避免恶意或损坏的发布包耗尽磁盘。
// 压缩包头里声明的大小不可信，因此写盘时复制循环会用同样的上限再兜一次底
// （见 writeZipEntry）。
const (
	maxAPIResponseSize   = 4 << 20 // 4 MiB —— 更新清单本身很小
	maxChecksumFileSize  = 1 << 20 // 1 MiB —— SHA256SUMS 通常只有几百字节
	maxSignatureFileSize = 8 << 10 // 8 KiB —— Ed25519 签名固定 64 字节，base64 后也很小
	maxSingleFileExtract = 512 << 20
	maxExtractedTotal    = 1 << 30 // 单次更新解压后总量上限 1 GiB

	// maxSignedArchiveSize 是允许读入内存做签名校验的发布包上限。
	// Ed25519 无法流式验证（必须拿到完整消息），因此这里必须有个界，
	// 否则一个超大或伪造的发布包会让更新过程把内存吃光。
	maxSignedArchiveSize = 256 << 20
)

// UpdateInfo 描述更新 API 或 GitHub 返回的一次发布。
//
// Checksum 由各检查函数统一归一化为 "sha256:<小写十六进制>"。
// Checksum 为空表示来源没有公布摘要：此时 Apply 会记一条警告并继续安装，
// 而不是直接失败，避免缺少清单把所有人的升级路径堵死。
//
// Signature 是发布包签名的文本表示（base64 或十六进制）。它与 Checksum 的策略不同：
// 只要二进制内嵌了公钥（见 EmbeddedPublicKey），签名就是**必需**的，
// 缺失即拒绝安装 —— 否则"钉住公钥"将毫无意义。
type UpdateInfo struct {
	HasUpdate    bool   `json:"has_update"`
	Version      string `json:"version"`
	URL          string `json:"url"`
	Checksum     string `json:"checksum"` // "sha256:<hex>"，为空表示无法校验
	Signature    string `json:"signature"`
	ReleaseNotes string `json:"release_notes"`
	Size         int64  `json:"size"`
	AssetName    string `json:"asset_name"`
}
