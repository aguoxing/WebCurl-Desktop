/**
 * 前端侧的请求/响应/工作区类型定义。
 *
 * 这些结构与 internal/httpclient、internal/store 里的 Go 结构一一对应：
 * Wails 绑定把 Go 的值按 JSON 标签序列化过来，字段名以 JSON 标签为准
 * （因此是 camelCase，而不是 Go 的导出名）。
 *
 * 这里手写而不是从 bindings 里的 models.js 推导：那些模型类为了兼容 Go 的零值
 * 语义，把每个字段都做成可选并带默认值，直接拿来用会让"这个字段到底有没有值"
 * 变得不可判断——而界面上大量逻辑恰恰依赖这个判断（比如"没填超时就显示占位符"）。
 */

/** 请求体类型，取值与 Go 侧的 BodyKind 保持一致。 */
export type BodyKind =
  'none' | 'json' | 'xml' | 'text' | 'html' | 'x-www-form-urlencoded' | 'form-data' | 'binary'

/** 认证方式。 */
export type AuthKind = 'none' | 'basic' | 'bearer' | 'api-key'

/** 一个可启用的键值对。界面上的"关掉但保留"就是 enabled = false。 */
export interface KV {
  name: string
  value: string
  enabled: boolean
}

/** form-data 里的一个文件字段。 */
export interface FileField {
  fieldName: string
  path: string
  contentType?: string
  enabled: boolean
}

/** 认证配置。 */
export interface Auth {
  kind: AuthKind
  username?: string
  password?: string
  token?: string
  apiKeyName?: string
  apiKeyValue?: string
  paramIn?: 'header' | 'query'
}

/** 一次待发送的请求。字段与界面各面板一一对应。 */
export interface RequestSpec {
  method: string
  url: string
  params?: KV[]
  headers?: KV[]
  cookies?: KV[]
  bodyKind?: BodyKind
  body?: string
  form?: KV[]
  files?: FileField[]
  binaryPath?: string
  auth?: Auth
  timeoutSec?: number
  retryCount?: number
  retryDelaySec?: number
  /** 跳过 TLS 证书校验。默认 false（即校验）。 */
  skipSslVerify?: boolean
  /** 不跟随 3XX。默认 false（即跟随）。 */
  noFollowRedirect?: boolean
  proxyUrl?: string
  acceptEncoding?: string
}

/** 一次请求的结果。Error 非空表示请求没能完成。 */
export interface SendResult {
  id: string
  error?: string
  status: number
  statusText: string
  proto: string
  headers: KV[]
  cookies: KV[]
  isText: boolean
  text?: string
  base64?: string
  size: number
  previewTruncated: boolean
  truncated: boolean
  durationMs: number
  attempts: number
  finalUrl: string
  redirects: number
  suggestedFileName: string
}

/** 一条请求历史。 */
export interface HistoryEntry {
  id: string
  at: string
  request: RequestSpec
  status: number
  durationMs: number
  size: number
  error?: string
}

/** 集合里的一个接口。 */
export interface CollectionItem {
  id: string
  name: string
  request: RequestSpec
}

/** 一组接口的集合。 */
export interface Collection {
  id: string
  name: string
  createdAt: string
  updatedAt: string
  items: CollectionItem[]
}

/** 一个环境变量。 */
export interface Variable {
  name: string
  value: string
  enabled: boolean
}

/** 一组变量。 */
export interface Environment {
  id: string
  name: string
  variables: Variable[]
}

/** 整个工作区数据文档。 */
export interface WorkspaceData {
  version: number
  history: HistoryEntry[]
  collections: Collection[]
  environments: Environment[]
  activeEnv: string
  savedAt: string
}

/** 数据文件位置。 */
export interface DataPath {
  dir: string
  file: string
}

/** Mock 引擎里的单个监听。 */
export interface MockListener {
  host: string
  port: number
  protocols?: string[]
}

/** Mock 引擎状态。 */
export interface MockStatus {
  running: boolean
  configPath: string
  listeners: MockListener[]
  lastError?: string
}

/** 一条 Mock 请求日志。 */
export interface MockRequestLog {
  time: string
  method: string
  url: string
  headers?: Record<string, string>
  body?: string
  status?: number
}

/** 请求调试的默认参数（与 Go 的 config.RequestConfig 对应）。 */
export interface RequestDefaultsConfig {
  defaultTimeoutSec: number
  defaultRetryCount: number
  defaultRetryDelaySec: number
  followRedirect: number
  verifySSL: number
  historyLimit: number
}

/** 工作台里的一个标签页。 */
export interface RequestTab {
  id: string
  /** 标签标题，默认由方法 + URL 推导，用户保存到集合后取集合里的名字。 */
  title: string
  /** 当前编辑中的请求。 */
  request: RequestSpec
  /** 最近一次发送的结果，未发送过为 null。 */
  response: SendResult | null
  /** 正在发送中。 */
  sending: boolean
  /** 该标签是否已被改动而未保存。 */
  dirty: boolean
  /** 若来自集合，记录来源以便"保存"回写。 */
  source?: { collectionId: string; itemId: string }
}
