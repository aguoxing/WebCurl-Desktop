# WebCurl Desktop

> **网页版 API 调试与 Mock 工具的桌面版** —— 单文件分发、完全离线、数据只留在本机。

[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8.svg)](https://go.dev)
[![Wails](https://img.shields.io/badge/Wails-v3%20beta.28-red.svg)](https://wails.io)
[![Vue](https://img.shields.io/badge/Vue-3-42B883.svg)](https://vuejs.org)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey.svg)](#-下载与安装)

WebCurl Desktop 把 WebCurl 的 **HTTP 请求调试** 与 **Mock 服务** 两项能力搬进了桌面：

- **前端**用 Vue 3 + TypeScript 重写，Postman / Apifox 式的经典布局，浅色为主、可切深色；
- **后端**是纯粹的 Wails v3 应用 —— 界面由 webview 直接加载内嵌资源，能力全部通过
  Go 绑定（binding）与事件（events）暴露，**没有**承载界面的内嵌 HTTP 服务；
- **Mock 引擎**是唯一的例外：它必须真实监听端口，因为它的用途就是被别的程序连上。

界面、请求、历史都在本地进程里，不经过任何第三方服务，也不需要登录。

---

## ✨ 特性

### 请求调试工作台

| 能力 | 说明 |
|------|------|
| 请求配置 | 方法（GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS）、地址、Params、Headers、Cookies |
| 请求体 | `none` / JSON / XML / Text / HTML / `x-www-form-urlencoded` / `multipart/form-data`（支持选文件）/ 二进制（选文件直传） |
| 认证 | 无 / Bearer / Basic / API Key（可放 Header 或查询参数）；手填的同名请求头优先级更高 |
| 选项 | 超时、重试次数与间隔（网络错误或 5xx 触发）、是否跟随 3XX、是否校验 TLS 证书、代理、Accept-Encoding |
| 响应查看 | 状态码 / 耗时 / 大小 / 重定向次数 / 重试次数；Body 的 JSON 美化与原始视图、图片预览、二进制另存、超限截断提示；响应头与 `Set-Cookie` 两个独立页签 |
| 多标签页 | 同时编辑多个请求，中键关闭、右键关闭其他 / 全部 |
| 取消 | 发送中可随时取消，不会留下悬空请求 |

### 集合、历史、变量

- **集合**：按接口分组管理，支持新建 / 重命名 / 删除，接口行带方法色标。
- **批量运行**：对某个集合串行跑一遍，带进度与逐条结果。
- **历史**：自动记录每次发送，按「今天 / 更早」分组，带状态码标签与耗时，可搜索、可逐条删除、可一键回填到标签页。
- **环境变量**：多套环境（如 dev / staging / prod），在地址、请求头、请求体里用 `{{变量名}}` 引用；发送前做替换，地址栏里保留 `{{}}` 原文。

### curl 互通

- **导入**：粘贴 curl 命令解析成请求（自写 tokenizer，能正确处理引号、转义与 `\` 续行）。
- **导出**：把当前请求复制成等价的 curl 命令。

### Mock 控制台

- JSON 配置编辑器，带实时合法性校验、「试加载」与「保存并热加载」。
- 分监听端口展示状态；配置改动后**无需重启应用**即可换路由。
- 请求日志实时推送到界面（环形缓冲上限 500 条），可查看请求头、请求体，一键复制。

### 桌面集成

系统托盘（显示 / 隐藏、启停 Mock、打开数据目录、退出）、单实例、开机自启、应用内自动更新（GitHub Releases 或自定义更新源，支持 Ed25519 验签）。

> **首版未包含**：WebSocket / SSE 调试面板与 WebTools 工具箱，留待后续迭代。

---

## 🖥 界面结构

```
┌───────────────────────────────────────────────────────────────────┐
│ 顶栏   页面标题          环境选择器  语言  主题  设置             │
├──────┬────────────────────────────────────────────────────────────┤
│ 图标 │ 标签栏  ① 请求 A ×  ② 请求 B ×  ＋                         │
│ 侧栏 ├────────────────────────────────────────────────────────────┤
│      │ 请求行  [GET ▾] [https://…        ] [发送] [保存][curl][历史]│
│ 工作 ├────────────────────────────────────────────────────────────┤
│ 台   │ Params │ Headers │ Cookies │ Body │ Auth │ 选项             │
│ Mock │        （配置区）                                          │
│ 设置 ├────────────────────────────────────────────────────────────┤
│ 关于 │ 响应   200 OK · 128 ms · 1.2 KB · 重定向 1 · 重试 2         │
│      │        Body │ 响应头 │ Cookies                            │
└──────┴────────────────────────────────────────────────────────────┘
   左侧栏（工作台内）：集合树 ⇄ 历史，可拖拽调宽
```

工作台内容区被 `SplitPane` 分成上下两块，分隔线可拖动 —— 调试请求体时把响应区拉大，
看响应时反过来。

### 快捷键

| 快捷键 | 动作 |
|--------|------|
| `Ctrl/Cmd + T` | 新建标签页 |
| `Ctrl/Cmd + S` | 保存请求到集合 |
| `Ctrl/Cmd + H` | 切到历史面板 |
| `Ctrl/Cmd + I` | 打开 curl 导入 |
| `Ctrl/Cmd + Enter` | 发送当前请求 |

> 快捷键只在工作台页面内生效；焦点在输入框里时 `Ctrl + Enter` 交给输入框自己处理。

---

## 📦 下载与安装

发布包按平台提供（Windows `.exe` + NSIS 安装器、macOS `.app`、Linux AppImage），
从 Release 页取对应压缩包解压即用。若尚未发布，直接[从源码构建](#-从源码构建)。

| 平台 | 要求 |
|------|------|
| Windows | 10 / 11，需要 WebView2 运行时（Win11 与较新的 Win10 已内置） |
| macOS | 11+ |
| Linux | 需要 GTK3/GTK4 与 WebKitGTK |

应用会把发现的新版本下载、验签、校验摘要后，交由一个独立的助手程序在主进程退出后
替换二进制并重启，因此更新过程里不会出现「自己覆盖自己」的失败。

---

## 🔨 从源码构建

### 环境要求

| 工具 | 版本 | 说明 |
|------|------|------|
| Go | 1.26+ | 见 `go.mod` |
| Node.js | 20+ | 前端构建 |
| pnpm | 9+ | 前端包管理（`frontend/pnpm-lock.yaml`） |
| Task | 3.x | 任务编排，读取 `Taskfile.yml` |
| Wails CLI | v3（与 beta.28 对齐） | `go install github.com/wailsapp/wails/v3/cmd/wails3@latest` |

### 构建

```bash
gofmt -l internal cmd main.go   # 必须无输出
go vet ./internal/... ./cmd/... .
go test ./internal/... ./cmd/...
```

或用 Task 一把梭：

```bash
task verify         # 类型检查 + 编译 + 单元测试（CI 用，不依赖 Wails 工具链）
task dev            # 开发模式，热重载（Vite 端口 9245）
task build          # 构建当前平台的可执行文件 → bin/
task run            # 构建并运行
task package        # 打安装包（NSIS / DMG / AppImage）
task build:server   # 以 -tags server 构建无界面的 HTTP 服务版
```

发布（版本号、可选签名）：

```bash
./scripts/build.sh --version 1.2.3
./scripts/build.sh --version 1.2.3 --release --platform all
./scripts/build.sh --version 1.2.3 --release --platform all --sign-key ~/.ssh/update_ed25519
```

### 前端单独开发

```bash
cd frontend
pnpm install
pnpm dev      # Vite 开发服务器
pnpm build    # 产出 dist/，即 main.go 内嵌的资源
```

> `main.go` 通过 `//go:embed all:frontend/dist` 内嵌前端产物，因此**首次构建前必须先构建
> 前端**，否则内嵌的是空目录（`all:` 前缀保证空目录也能编译通过）。

改了 Go 侧**导出的类型名或方法**后，需要重新生成前端绑定：

```bash
task common:generate:bindings   # 输出到 frontend/src/bindings
```

绑定生成在 `frontend/src/bindings` 而不是 Wails 的默认目录，因此视图里可以直接写
`@/bindings/...`。这个路径写在 `build/Taskfile.yml` 的 `generate:bindings` 任务里，
**改动它要同步改前端别名。**

---

## ⚙️ 配置

配置文件按以下顺序解析，第一个存在者生效：

1. `{exe_dir}/configs/config.yaml` —— 可执行文件同目录，优先级最高；
2. `{XDG_CONFIG_HOME}/webcurl-desktop/config.yaml` —— 用户级配置；
3. 回退到第 1 项路径（文件不存在时用默认值）。

`{exe_dir}` 由 `os.Executable()` 推导而非当前工作目录，所以双击图标启动也能找到配置。
完整注释版模板见 [`configs/config.example.yaml`](configs/config.example.yaml)：

```bash
cp configs/config.example.yaml configs/config.yaml
```

也可以直接在前端**设置页**里改（部分项即时生效，如开机自启；另一些需重启，如日志级别）。

```yaml
app:
  name: "WebCurl"
  version: "1.0.1"          # 仅在二进制未内嵌版本时生效（本地 go run / go test）

window:
  title: "WebCurl"
  url: "/"                  # 前端路由入口
  background_colour: "#1B2636"
  min_width: 800            # 窗口可拖到的最小尺寸，0 = 不设下限
  min_height: 600

tray:
  close_behaviour: "quit"   # "quit" = 关闭即退出 | "minimize" = 隐藏到托盘
  tooltip: "WebCurl"

startup:
  enabled: 0                # 1 = 注册开机自启

update:
  auto_check: 0             # 1 = 启动 2 秒后检查一次更新
  provider: "github"        # "" = 关闭更新 | "github" | "custom"
  api_url: "aguoxing/webcurl-desktop"   # github 填 "owner/repo"；custom 填完整 URL
  github_token: ""          # 建议留空，改用 GOWAILS_GITHUB_TOKEN 环境变量
  public_key: ""            # Ed25519 验签公钥；通常留空，正式构建由 -ldflags 内嵌

log:
  level: "debug"            # debug | info | warn | error
  file: ""                  # 空 = {exe_dir}/logs/app.log
  wails_file: ""            # 空 = 与主日志合并
  max_size: 10              # 单个日志文件上限（MB）
  max_backups: 5
  max_age: 30
  compress: 1
  console: 1

server:                     # 仅 `task build:server` 构建读取这一节
  host: "localhost"
  port: 8080

request:                    # 只影响"之后新建的请求"，不改写已保存的请求
  default_timeout_sec: 30   # 0 = 不设超时
  default_retry_count: 0    # 不含首次尝试
  default_retry_delay_sec: 1
  follow_redirect: 1        # 1 = 默认跟随 3XX
  verify_ssl: 1             # 1 = 默认校验 TLS 证书
  history_limit: 500        # 历史保留条数上限

mock:
  enabled: 1                # 1 = 随应用启动引擎；0 = 只在 Mock 页面手动启动
  config_path: ""           # 空 = 数据目录下的 mock.json

storage:
  dir: ""                   # 空 = 用户数据目录下的 webcurl-desktop/data
```

布尔开关统一用 `0` / `1`。

**几处刻意的语义划分：**

- `request` 一节只决定「新建请求时勾选什么」。用户在某个请求里关掉重定向、加了 30 秒超时，
  那是这条请求自身的属性，随它一起保存进集合与历史 —— 否则调试某个接口时的临时放宽
  会顺手污染所有后续请求。
- **凭据只走环境变量或内存，不落盘也不回传前端。** `GOWAILS_GITHUB_TOKEN` 优先级高于
  YAML 的 `github_token`，且来源是环境变量时 `Save()` 会把它从写盘内容里剔除；
  `GetAllConfig()` 返回给前端的令牌一律替换为 `***`；前端把 `***` 或空串提交回来时，
  按「不改」处理。保存文件权限为 `0600`。
- **属于二进制自身的东西不进配置文件。** 版本号与验签公钥都由 `-ldflags -X` 注入，
  注入值优先于 YAML —— 否则用户改一行配置就能让更新检查的「当前版本」漂移，
  或把信任锚换成自己的密钥。

### 环境变量

| 变量 | 作用 | 优先级 |
|------|------|--------|
| `GOWAILS_GITHUB_TOKEN` | 私有仓库 / 提升 API 限额所需的 PAT | 高于 YAML 的 `github_token` |
| `WAILS_SERVER_HOST` | server 模式监听地址 | 高于 YAML 的 `server.host` |
| `WAILS_SERVER_PORT` | server 模式监听端口 | 高于 YAML 的 `server.port` |
| `GOWS_VERSION` | **构建期**版本号，由 `scripts/build.sh --version` 下发 | 高于 `git describe` |
| `GOWAILS_UPDATE_PUBLIC_KEY` | **构建期**验签公钥（`--sign-key` 时设置） | 植入二进制后运行时不可覆盖 |

---

## 🔒 数据存放与隐私

所有数据都落在**用户数据目录**，不会写进安装目录（桌面应用可能被装到 `Program Files`
这类只读位置），也不会发往任何服务器：

```
{用户数据目录}/webcurl-desktop/data/
  data.json     历史 / 接口集合 / 环境变量
  mock.json     Mock 配置（mock.config_path 为空时）
```

- Windows：`%LOCALAPPDATA%\webcurl-desktop\data\`
- macOS：`~/Library/Application Support/webcurl-desktop/data/`
- Linux：`~/.local/share/webcurl-desktop/data/`

托盘菜单里的「打开数据目录」可以直接跳到那里。想备份或迁移「我的接口集合」，
拷走 `webcurl-desktop` 这一个目录即可。

> 完整响应体**不进入 webview**：预览只带前 1 MiB，界面上的「另存为」由后端把完整内容
> 直接写盘。因此响应即使是 200 MiB 的二进制文件也不会把界面撑爆。

写入采用「临时文件 + rename」的原子替换；数据文件损坏时会被改名成 `.corrupt-*` 留档，
而不是静默丢弃。文件版本高于当前程序时拒绝加载，避免旧版本把新格式读坏。

---

## 🧱 项目结构

```
main.go                     定位配置 → 初始化日志 → 装配并运行应用
internal/
  app/                      装配与生命周期（唯一的 Wails 拥有者）
    app.go                  App、New、依赖装配、启动序列
    window.go / tray.go     主窗口、系统托盘
    menu*.go                应用菜单与多语言重建
    mock.go                 Mock 启动/停止接线、mock:* 事件、文件对话框
    shell.go / system.go    托盘与菜单触发的动作、运行环境信息
    autostart.go / update.go
  binding/                  前端契约层：只暴露该给前端的方法
  service/                  业务逻辑（不 import wails）
    request.go              发送、取消、响应体另存
    workspace.go            集合 / 历史 / 环境 / 导入导出
    mock.go / config.go     引擎启停与热加载、配置读写
    system.go / update.go
  httpclient/               请求执行引擎（纯 net/http，无 wails 依赖）
  store/                    历史 / 集合 / 环境的本地持久化（原子写盘）
  mock/                     Mock 引擎：监听、路由匹配、响应生成、日志回调
  config/                   配置定义、加载、合并、持久化
  updater/                  检查 / 下载 / 校验 / 解压 / 交接助手
  logger/                   zerolog 初始化与 slog 桥接
  version/ buildinfo/ assets/
cmd/
  updater/                  独立的更新助手（仅标准库，可交叉编译到任意平台）
  versionldflags/ wininfo/  构建期工具：版本与公钥 → -ldflags / Windows 版本资源
frontend/
  src/
    views/workbench/        工作台页面与组件（标签栏、请求行、六个配置面板、响应区…）
    views/mock/             Mock 控制台
    views/setting/ about/
    layout/                 顶栏 / 图标侧栏 / 状态栏
    store/modules/          Pinia：workspace / workbench / mock / app / locale
    api/                    绑定调用的统一封装与 TS 类型
    utils/                  curl 解析、变量替换、格式化（纯函数，不依赖 store）
    bindings/               Wails 生成的 JS 绑定（勿手改）
configs/                    config.example.yaml（模板）与 config.yaml（本机，已忽略）
build/                      各平台构建配置、图标、容器与打包脚本
scripts/build.sh            本地构建与发布入口
```

---

## 🏗 架构要点

后端是标准的单向依赖结构，箭头方向即 import 方向：

```
main.go
  └── internal/app                装配与生命周期
        ├── internal/binding      前端契约层：只暴露该给前端的方法
        │     └── internal/service        业务逻辑，不 import wails
        │           ├── internal/httpclient   请求引擎
        │           ├── internal/store        本地持久化
        │           ├── internal/mock         Mock 引擎
        │           ├── internal/config / updater
        └── internal/logger / version / assets
```

**为什么中间要有一层 `binding`。** Wails 会把服务的**所有导出方法**生成 JS 调用，
非导出方法不会。若把 `service` 直接交给 Wails，就等于把服务未来每一个导出方法都开放给
webview。`binding` 是一层显式的契约：`service` 只写业务，`binding` 决定哪些方法可见，
代价是多一层转发，收益是「暴露面」始终是一个可以一眼读完的文件列表。

**依赖注入而非包级状态。** 分层要求依赖单向（`app → service`），但 `service` 有时需要
回调运行中的应用（切换开机自启、触发更新检查、读取运行环境）。若直接 import `app`
就会形成循环，因此由 `app` 在装配时把**方法值**注入进去，回调只存在结构体字段里 ——
没有「必须先 `New` 再 `SetXxx`」的隐式初始化，单元测试也能直接构造带桩回调的服务。

**请求失败不是异常。** 发送接口返回的 `SendResult` 里有一个 `Error` 字段：它非空表示
「请求没能完成」（DNS 解析失败、证书错误……），要展示给用户看；而「调用绑定失败」
（Promise reject）是程序缺陷。两者语义不同，混在一起会让界面无从区分。

**桌面 / server 两种形态。** 同一份装配代码靠 `application.System.IsServer()` 分流，
它是**编译期常量**，因此桌面构建里不含 HTTP 服务端代码，server 构建里不含窗口 / 托盘代码。

**界面骨架的分工。** 有视图归属的位置交给前端路由，没有视图归属的动作交给原生菜单：
剪贴板角色、缩放、全屏、退出由原生菜单承担（webview 里的剪贴板快捷键由菜单角色分发，
缺了它在 macOS 上 `Cmd+C` 根本不工作），页面切换与高亮交给 Vue Router。两侧通过事件桥
连接，事件名是跨语言契约 —— 改名只改一边不会有编译错误、只会在运行时静默失效，
因此有单元测试专门去前端源码里核对这些名字。

---

## 🧪 技术栈

| 层 | 选型 |
|----|------|
| 桌面框架 | Wails v3（beta.28） |
| 后端 | Go 1.26，标准库 `net/http` 自建请求引擎；`zerolog` + `lumberjack` 日志 |
| 前端 | Vue 3 + TypeScript + Vite、Element Plus、Pinia、vue-router（hash）、vue-i18n |
| Mock | Go 标准库 HTTP server + `coder/websocket` |
| 更新 | GitHub Releases / 自定义 HTTP API，Ed25519 验签 |

第三方依赖刻意压到最低：请求引擎只用标准库（自己处理 multipart 与文件流），
curl 解析在前端用几十行 tokenizer 完成，分栏容器也是自写的几十行组件 ——
不引入 splitpanes 之类的库，是为了不必为改一条分隔线颜色去翻一整套文档。
Mock 配置里的接口集合是 **JSON**，与 `mock.json` 的格式一致。

---

## ❓ 常见问题

**配置改了没生效？**
日志第一行会打印实际加载的 `config` 路径，先确认改的是那一份。另外 `storage.dir` 与
`mock.config_path` 是在启动时确定的，改完要重启应用。

**`go build ./...` 直接失败？**
这是预期的：`build/ios` 下有一个没有 `func main` 的 `package main`（带 `//go:build !ios`
标签），它是 `wails3 update build-assets` 生成的、供 iOS 构建 overlay 使用的模板资源。
**不要删它，也不要补 `main`。** 用 `task verify`（根包只做 `vet`）。

**Windows 上 `go build .` 第二次就失败？**
Wails 工具链会先生成 `wails_windows_amd64.syso` 到仓库根目录、链接后自行删除；构建中途
被打断就会残留并导致后续链接失败。真实二进制请用 `task build`。

**Mock 启动后日志里什么都没写？**
这是预期行为 —— 加日志会让「配置里没开 Mock」和「开了但没监听」两种情况在日志里
长得一样。空配置（`mock.json` 里没有任何 `listen`）也不算故障：它只是首次运行的正常
状态，由 Mock 页面提示「还没有配置监听」，而不是弹一堆红字。

**下载大文件会不会卡住界面？**
不会。响应预览只带前 1 MiB，完整内容留在后端缓存里，「另存为」由后端直接写盘。

---

## 🤝 开发约定

- **注释使用中文**，导出符号必须有文档注释且以符号名开头（`// UpdateConfig ...`）。
- 注释解释「为什么」，而不是复述「做了什么」；非直觉的分支尤其要写清原因。
- 不使用包级可变状态承载依赖，改为构造器参数或方法值注入。
- 任何被忽略的错误都显式写 `_ =`，并在注释里说明为何可以忽略。
- 提交前跑 `task verify`，`gofmt` 与 `go vet` 必须干净。
- Go 文件名反映**主题**而非层：`service/config.go` 与 `binding/config.go` 一一对应，
  避免两个包各有一个 `configservice.go`。单文件超过约 300 行且关注点混杂时按职责拆分。
- 平台相关文件用 Go 构建约束后缀（`process_windows.go` / `process_other.go`），
  互斥的构建标签成对出现（`menu_dev.go` / `menu_production.go`）。
- 前端样式骨架用 `@use` 共享而不复制：六个配置面板的边距与底栏高度必须完全一致，
  否则切换页签时内容会整体抖一下。

---

## 🙏 致谢

- 能力与交互源自网页版 **WebCurl**（HTTP 请求转发、调试与 Mock 工具），
  本仓库是它的桌面化重写。
- 感谢 [Wails](https://wails.io)、[Vue](https://vuejs.org)、
  [Element Plus](https://element-plus.org) 等开源项目。

---

## 📄 许可证

[MIT](LICENSE) © 2026 aguoxing
