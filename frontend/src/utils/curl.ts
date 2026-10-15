/**
 * curl 命令与请求结构之间的互转。
 *
 * 放在前端而不是后端：这是一段纯字符串处理，放在这里可以即时反馈（用户粘贴完
 * 立刻在界面上看到解析结果并直接修改），而往返一次后端只会让交互变迟钝。
 *
 * 支持的是"日常能从浏览器 DevTools 复制出来"的那部分 curl 语法，覆盖：
 *   -X/--request、-H/--header、-d/--data/--data-raw/--data-binary/--data-urlencode、
 *   -F/--form、-u/--user、-b/--cookie、--compressed、--insecure/-k、-L/--location，
 *   以及换行续行符 \" 和单/双引号包裹。
 * 明确不做的是：多 URL、-o 输出、代理链、cert 等与"调试这个接口"无关的开关。
 */
import type { BodyKind, KV, RequestSpec } from '@/api/types'

/** 把 curl 命令解析成请求结构。解析失败时抛错，错误信息面向用户。 */
export function parseCurl(command: string): RequestSpec {
  const tokens = tokenize(command)
  if (tokens.length === 0) {
    throw new Error('命令为空')
  }
  if (tokens[0].toLowerCase() !== 'curl') {
    throw new Error('不是 curl 命令（应以 curl 开头）')
  }

  const req: RequestSpec = {
    method: '',
    url: '',
    params: [],
    headers: [],
    cookies: [],
    bodyKind: 'none',
    body: '',
    form: [],
    files: [],
    auth: { kind: 'none' },
    timeoutSec: 0,
    retryCount: 0,
    retryDelaySec: 0,
    skipSslVerify: false,
    noFollowRedirect: false
  }

  const dataParts: string[] = []
  const formParts: { name: string; value: string }[] = []
  const fileParts: { fieldName: string; path: string }[] = []
  let urlEncodeParts: string[] = []

  for (let i = 1; i < tokens.length; i += 1) {
    const tok = tokens[i]
    switch (tok) {
      case '-X':
      case '--request':
        req.method = (tokens[++i] ?? '').toUpperCase()
        break

      case '-H':
      case '--header': {
        const raw = tokens[++i] ?? ''
        const idx = raw.indexOf(':')
        if (idx > 0) {
          const name = raw.slice(0, idx).trim()
          const value = raw.slice(idx + 1).trim()
          // curl 里显式给的 Content-Type 由 body 类型推导，避免出现两份。
          if (name.toLowerCase() !== 'content-type') {
            req.headers?.push({ name, value, enabled: true })
          }
        }
        break
      }

      case '-d':
      case '--data':
      case '--data-raw':
      case '--data-binary':
        dataParts.push(tokens[++i] ?? '')
        break

      case '--data-urlencode':
        urlEncodeParts.push(tokens[++i] ?? '')
        break

      case '-F':
      case '--form': {
        const raw = tokens[++i] ?? ''
        const idx = raw.indexOf('=')
        if (idx > 0) {
          const name = raw.slice(0, idx)
          const value = raw.slice(idx + 1)
          if (value.startsWith('@')) {
            fileParts.push({ fieldName: name, path: value.slice(1) })
          } else {
            formParts.push({ name, value })
          }
        }
        break
      }

      case '-u':
      case '--user': {
        const raw = tokens[++i] ?? ''
        const idx = raw.indexOf(':')
        req.auth = {
          kind: 'basic',
          username: idx >= 0 ? raw.slice(0, idx) : raw,
          password: idx >= 0 ? raw.slice(idx + 1) : ''
        }
        break
      }

      case '-b':
      case '--cookie': {
        const raw = tokens[++i] ?? ''
        for (const pair of raw.split(';')) {
          const idx = pair.indexOf('=')
          if (idx > 0) {
            req.cookies?.push({
              name: pair.slice(0, idx).trim(),
              value: pair.slice(idx + 1).trim(),
              enabled: true
            })
          }
        }
        break
      }

      case '--compressed':
        // 上游语义就是"声明接受压缩"；这里落到显式的 Accept-Encoding。
        req.acceptEncoding = 'gzip, deflate, br'
        break

      case '-k':
      case '--insecure':
        req.skipSslVerify = true
        break

      case '-L':
      case '--location':
        req.noFollowRedirect = false
        break

      case '-G':
      case '--get':
        // -G 表示把 -d 的内容拼到查询串上；转成参数即可。
        if (dataParts.length) {
          for (const kv of parseQueryPairs(dataParts.join('&'))) {
            req.params?.push({ ...kv, enabled: true })
          }
          dataParts.length = 0
        }
        break

      case '-s':
      case '--silent':
      case '-i':
      case '--include':
      case '-v':
      case '--verbose':
      case '--no-buffer':
      case '-N':
        // 只影响 curl 自身的输出行为，与本应用展示无关，直接忽略。
        break

      default:
        if (tok.startsWith('-')) {
          // 未知开关：保守地跳过它的可能取值，避免把值当成 URL。
          break
        }
        if (!req.url) {
          req.url = tok
        }
        break
    }
  }

  if (!req.url) {
    throw new Error('命令里没有找到请求地址')
  }

  // 从 URL 上拆出查询参数，让它们出现在 Params 面板里而不是被藏在 URL 中。
  const split = splitURLAndParams(req.url)
  req.url = split.url
  for (const kv of split.params) {
    req.params?.push({ ...kv, enabled: true })
  }

  if (urlEncodeParts.length) {
    dataParts.push(urlEncodeParts.join('&'))
  }

  if (fileParts.length || formParts.length) {
    req.bodyKind = 'form-data'
    req.form = formParts.map((p) => ({ ...p, enabled: true })) as KV[]
    req.files = fileParts.map((p) => ({ ...p, enabled: true }))
  } else if (dataParts.length) {
    const joined = dataParts.join('&')
    const contentType = pickContentType(req.headers)
    if (contentType.includes('application/x-www-form-urlencoded')) {
      req.bodyKind = 'x-www-form-urlencoded'
      req.form = parseQueryPairs(joined).map((p) => ({ ...p, enabled: true }))
    } else {
      req.bodyKind = guessBodyKind(contentType, joined)
      req.body = joined
    }
  }

  if (!req.method) {
    req.method = req.bodyKind === 'none' ? 'GET' : 'POST'
  }
  return req
}

/** 把请求结构拼成可直接粘贴到终端的 curl 命令。 */
export function buildCurl(req: RequestSpec, pretty = true): string {
  const parts: string[] = []
  const method = (req.method || 'GET').toUpperCase()
  if (method !== 'GET') {
    parts.push(`-X ${method}`)
  }

  const fullUrl = composeURL(req)
  parts.push(quote(fullUrl, pretty))

  for (const h of req.headers ?? []) {
    if (!h.enabled || !h.name) continue
    parts.push(`-H ${quote(`${h.name}: ${h.value}`, pretty)}`)
  }

  const auth = req.auth
  if (auth?.kind === 'basic' && auth.username) {
    parts.push(`-u ${quote(`${auth.username}:${auth.password ?? ''}`, pretty)}`)
  }
  if (auth?.kind === 'bearer' && auth.token) {
    parts.push(`-H ${quote(`Authorization: Bearer ${auth.token}`, pretty)}`)
  }
  if (auth?.kind === 'api-key' && auth.apiKeyName && auth.paramIn === 'header') {
    parts.push(`-H ${quote(`${auth.apiKeyName}: ${auth.apiKeyValue ?? ''}`, pretty)}`)
  }

  const cookies = (req.cookies ?? []).filter((c) => c.enabled && c.name)
  if (cookies.length) {
    parts.push(`-b ${quote(cookies.map((c) => `${c.name}=${c.value}`).join('; '), pretty)}`)
  }

  switch (req.bodyKind) {
    case 'form-data':
      for (const f of req.form ?? []) {
        if (f.enabled && f.name) parts.push(`-F ${quote(`${f.name}=${f.value}`, pretty)}`)
      }
      for (const f of req.files ?? []) {
        if (f.enabled && f.path) {
          parts.push(`-F ${quote(`${f.fieldName || 'file'}=@${f.path}`, pretty)}`)
        }
      }
      break

    case 'x-www-form-urlencoded': {
      const body = (req.form ?? [])
        .filter((f) => f.enabled && f.name)
        .map((f) => `${encodeURIComponent(f.name)}=${encodeURIComponent(f.value)}`)
        .join('&')
      if (body) parts.push(`--data ${quote(body, pretty)}`)
      break
    }

    case 'json':
    case 'xml':
    case 'text':
    case 'html':
      if (req.body) parts.push(`--data-raw ${quote(req.body, pretty)}`)
      break

    case 'binary':
      if (req.binaryPath) parts.push(`--data-binary ${quote(`@${req.binaryPath}`, pretty)}`)
      break

    default:
      break
  }

  if (req.skipSslVerify) parts.push('-k')
  if (req.noFollowRedirect) parts.push('--max-redirs 0')

  return joinForShell(parts, pretty)
}

// --- 内部工具 ---

/**
 * 把命令拆成 token，处理引号与续行。
 *
 * 自己写而不是用 split(' ')：命令里的 JSON 体几乎必然带空格与引号，
 * 按空格切会把一个 -d '{"a": 1}' 拆成三段，解析结果全错。
 */
function tokenize(command: string): string[] {
  const tokens: string[] = []
  let current = ''
  let quoteChar: '"' | "'" | null = null
  let started = false

  for (let i = 0; i < command.length; i += 1) {
    const ch = command[i]

    if (quoteChar) {
      if (ch === quoteChar) {
        quoteChar = null
      } else if (ch === '\\' && quoteChar === '"' && i + 1 < command.length) {
        // 双引号内的反斜杠转义（curl 常见于 \" 与 \\）。
        i += 1
        current += command[i]
      } else {
        current += ch
      }
      continue
    }

    if (ch === "'" || ch === '"') {
      quoteChar = ch as '"' | "'"
      started = true
      continue
    }

    if (ch === '\\' && i + 1 < command.length && command[i + 1] === '\n') {
      i += 1 // 续行
      continue
    }
    if (ch === '\\' && i + 1 < command.length) {
      // 行尾续行符之外的反斜杠：保留原字符（Windows 路径里很常见）。
      i += 1
      current += command[i]
      started = true
      continue
    }

    if (/\s/.test(ch)) {
      if (started || current) {
        tokens.push(current)
        current = ''
        started = false
      }
      continue
    }

    current += ch
    started = true
  }

  if (current || started) {
    tokens.push(current)
  }
  return tokens.filter((t) => t !== '')
}

/** 拆出 URL 上的查询参数。 */
function splitURLAndParams(raw: string): { url: string; params: KV[] } {
  const idx = raw.indexOf('?')
  if (idx < 0) return { url: raw, params: [] }
  return {
    url: raw.slice(0, idx),
    params: parseQueryPairs(raw.slice(idx + 1))
  }
}

/** 把 a=1&b=2 解析为键值对（不做解码，保持用户输入原样）。 */
function parseQueryPairs(raw: string): KV[] {
  if (!raw) return []
  return raw
    .split('&')
    .filter((p) => p !== '')
    .map((pair) => {
      const idx = pair.indexOf('=')
      return {
        name: idx >= 0 ? pair.slice(0, idx) : pair,
        value: idx >= 0 ? pair.slice(idx + 1) : '',
        enabled: true
      }
    })
}

/** 读取请求头里的 Content-Type。 */
function pickContentType(headers: KV[] | undefined): string {
  const found = (headers ?? []).find((h) => h.enabled && h.name.toLowerCase() === 'content-type')
  return found?.value.toLowerCase() ?? ''
}

/** 依据 Content-Type 与内容猜请求体类型。 */
function guessBodyKind(contentType: string, body: string): BodyKind {
  if (contentType.includes('json')) return 'json'
  if (contentType.includes('xml')) return 'xml'
  if (contentType.includes('html')) return 'html'
  if (contentType.includes('text/plain')) return 'text'

  const trimmed = body.trim()
  if (/^[[{]/.test(trimmed)) {
    try {
      JSON.parse(trimmed)
      return 'json'
    } catch {
      /* 不是合法 JSON，按文本处理 */
    }
  }
  if (trimmed.startsWith('<')) return 'xml'
  return 'text'
}

/** 把请求结构拼成完整 URL（含查询参数与 api-key 的 query 形态）。 */
export function composeURL(req: RequestSpec): string {
  const base = (req.url || '').trim()
  const pairs: KV[] = (req.params ?? []).filter((p) => p.enabled && p.name)
  const extra = [...pairs]
  if (req.auth?.kind === 'api-key' && req.auth.paramIn === 'query' && req.auth.apiKeyName) {
    extra.push({
      name: req.auth.apiKeyName,
      value: req.auth.apiKeyValue ?? '',
      enabled: true
    })
  }
  if (!extra.length) return base

  const query = extra
    .map((p) => `${encodeURIComponent(p.name)}=${encodeURIComponent(p.value)}`)
    .join('&')
  return base.includes('?') ? `${base}&${query}` : `${base}?${query}`
}

/** 按 shell 习惯给参数加引号。 */
function quote(value: string, pretty: boolean): string {
  if (!pretty) return value
  if (/^[A-Za-z0-9_./:=@-]+$/.test(value)) return value
  return `'${value.replace(/'/g, `'\\''`)}'`
}

/** 拼接多行命令，续行符让长命令仍然可读。 */
function joinForShell(parts: string[], pretty: boolean): string {
  if (!pretty || parts.length === 1) {
    return `curl ${parts.join(' ')}`
  }
  return `curl ${parts.join(' \\\n  ')}`
}
