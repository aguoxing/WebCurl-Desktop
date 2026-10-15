/** 展示层的格式化工具。 */

/** 把字节数格式化成人类可读的形式。 */
export function formatBytes(bytes: number): string {
  if (!bytes || bytes < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = bytes
  let i = 0
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i += 1
  }
  // 小于 10 时保留一位小数（1.5 KB 比 2 KB 有信息量），否则取整。
  const text = value < 10 && i > 0 ? value.toFixed(1) : Math.round(value).toString()
  return `${text} ${units[i]}`
}

/** 把毫秒格式化成人类可读的耗时。 */
export function formatDuration(ms: number): string {
  if (!ms || ms < 0) return '0 ms'
  if (ms < 1000) return `${Math.round(ms)} ms`
  return `${(ms / 1000).toFixed(2)} s`
}

/** 把时间戳格式化成 HH:mm:ss。 */
export function formatClock(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

/** 把时间戳格式化成 YYYY-MM-DD HH:mm:ss。 */
export function formatDateTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
    `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  )
}

/** 尝试把文本格式化成缩进 JSON；失败时原样返回。 */
export function prettyJSON(text: string): string {
  const trimmed = text.trim()
  if (!trimmed) return text
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2)
  } catch {
    return text
  }
}

/** 判断文本是否是（可解析的）JSON。 */
export function isJSON(text: string): boolean {
  const trimmed = text.trim()
  if (!trimmed) return false
  if (!/^[[{]/.test(trimmed)) return false
  try {
    JSON.parse(trimmed)
    return true
  } catch {
    return false
  }
}

/** 按状态码给出语义化的颜色类型，供 Element Plus 的 tag 使用。 */
export function statusTagType(status: number): 'success' | 'warning' | 'danger' | 'info' {
  if (status === 0) return 'info'
  if (status < 300) return 'success'
  if (status < 400) return 'warning'
  if (status < 500) return 'warning'
  return 'danger'
}

/** 状态码的文字说明，用于在不熟悉的码出现时给出提示。 */
export function statusText(status: number): string {
  if (status === 0) return '未发送'
  const map: Record<number, string> = {
    200: 'OK',
    201: 'Created',
    204: 'No Content',
    301: 'Moved Permanently',
    302: 'Found',
    304: 'Not Modified',
    307: 'Temporary Redirect',
    308: 'Permanent Redirect',
    400: 'Bad Request',
    401: 'Unauthorized',
    403: 'Forbidden',
    404: 'Not Found',
    405: 'Method Not Allowed',
    408: 'Request Timeout',
    409: 'Conflict',
    422: 'Unprocessable Entity',
    429: 'Too Many Requests',
    500: 'Internal Server Error',
    502: 'Bad Gateway',
    503: 'Service Unavailable',
    504: 'Gateway Timeout'
  }
  return map[status] ?? ''
}

/** 从 URL 里取出路径与查询串，用于在标签页上做简短标题。 */
export function shortUrl(url: string): string {
  if (!url) return '未命名请求'
  try {
    const u = new URL(url)
    return (u.pathname || '/') + (u.search || '')
  } catch {
    return url
  }
}

/** 生成一个足够唯一的短 ID，用于标签页与请求标识。 */
export function shortId(): string {
  const bytes = new Uint8Array(8)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
}

/** 把 Base64 解码成字节数组，供"预览图片/下载"使用。 */
export function base64ToBytes(base64: string): Uint8Array {
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

/** 把 Base64 转成可直接放进 img src 的 data URL。 */
export function base64ToDataUrl(base64: string, contentType?: string): string {
  const type = contentType || 'application/octet-stream'
  return `data:${type};base64,${base64}`
}

/** 请求方法下拉里的可选项，顺序按使用频率排。 */
export const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'] as const

/**
 * 方法对应的标识色。
 *
 * 沿用 Swagger 那一套配色：调试工具里方法的颜色已经形成了肌肉记忆，
 * 换一套配色虽然"更好看"，但会让用户每次都要重新确认自己点的是哪个方法。
 * 颜色本身偏亮，浅色主题下直接当文字色对比度不足，所以实际使用时都配
 * hexAlpha 出来的浅底（见 methodChipStyle）。
 */
const METHOD_COLORS: Record<string, string> = {
  GET: '#2f80ed',
  POST: '#27ae60',
  PUT: '#e08b1a',
  PATCH: '#12b5b0',
  DELETE: '#e74c3c',
  HEAD: '#8e5cf7',
  OPTIONS: '#2b6cb0'
}

/** 取方法标识色，未知方法给一个中性灰。 */
export function methodColor(method: string): string {
  return METHOD_COLORS[(method || '').toUpperCase()] ?? '#8a8f99'
}

/** 把 #rrggbb 加上透明度，得到 rgba() 字符串。 */
export function hexAlpha(hex: string, alpha: number): string {
  let s = hex.replace('#', '')
  if (s.length === 3) {
    s = s
      .split('')
      .map((c) => c + c)
      .join('')
  }
  const r = parseInt(s.slice(0, 2), 16)
  const g = parseInt(s.slice(2, 4), 16)
  const b = parseInt(s.slice(4, 6), 16)
  if (Number.isNaN(r) || Number.isNaN(g) || Number.isNaN(b)) return hex
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

/** 方法标签的底色文字色组合。 */
export function methodChipStyle(method: string): Record<string, string> {
  const color = methodColor(method)
  return { color, backgroundColor: hexAlpha(color, 0.12) }
}

/** 在响应头列表里找某个头的值（不区分大小写）。 */
export function headerValue(
  headers: { name: string; value: string }[] | undefined,
  name: string
): string {
  if (!headers) return ''
  const found = headers.find((h) => h.name.toLowerCase() === name.toLowerCase())
  return found?.value ?? ''
}
