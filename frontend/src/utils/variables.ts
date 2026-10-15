/**
 * 变量替换：把请求里的 {{变量名}} 换成当前环境下的值。
 *
 * 替换发生在发送前的一刻（而不是编辑时就地改写）：用户在地址栏里看到的始终是
 * 带 {{}} 的原始文本，否则切一次环境就要把请求内容改一遍，没法来回对比。
 */
import type { RequestSpec, Variable } from '@/api/types'

/** 变量引用语法：{{name}}，名字两侧允许空格。 */
const VAR_PATTERN = /\{\{\s*([^}\s]+)\s*\}\}/g

/** 用变量表替换一段文本。 */
export function substitute(text: string, vars: Map<string, string>): string {
  if (!text) return text
  return text.replace(VAR_PATTERN, (match, name: string) => {
    const value = vars.get(name)
    return value === undefined ? match : value
  })
}

/** 找出文本里引用到的变量名（去重），用于提示"哪些变量没定义"。 */
export function referencedVars(text: string): string[] {
  const out = new Set<string>()
  for (const m of text.matchAll(VAR_PATTERN)) {
    out.add(m[1])
  }
  return [...out]
}

/** 由变量列表构造查询表（跳过未启用的项）。 */
export function toVarMap(variables: Variable[] | undefined): Map<string, string> {
  const map = new Map<string, string>()
  for (const v of variables ?? []) {
    if (v.enabled && v.name) {
      map.set(v.name, v.value)
    }
  }
  return map
}

/** 对整份请求做变量替换，返回新对象（不改原对象）。 */
export function applyVariables(req: RequestSpec, vars: Map<string, string>): RequestSpec {
  if (vars.size === 0) return req
  const sub = (s: string) => substitute(s, vars)

  return {
    ...req,
    url: sub(req.url),
    headers: (req.headers ?? []).map((h) => ({ ...h, name: sub(h.name), value: sub(h.value) })),
    params: (req.params ?? []).map((p) => ({ ...p, name: sub(p.name), value: sub(p.value) })),
    cookies: (req.cookies ?? []).map((c) => ({ ...c, name: sub(c.name), value: sub(c.value) })),
    form: (req.form ?? []).map((f) => ({ ...f, name: sub(f.name), value: sub(f.value) })),
    files: (req.files ?? []).map((f) => ({ ...f, path: sub(f.path) })),
    binaryPath: req.binaryPath ? sub(req.binaryPath) : req.binaryPath,
    body: req.body ? sub(req.body) : req.body,
    auth: req.auth
      ? {
          ...req.auth,
          username: req.auth.username ? sub(req.auth.username) : req.auth.username,
          password: req.auth.password ? sub(req.auth.password) : req.auth.password,
          token: req.auth.token ? sub(req.auth.token) : req.auth.token,
          apiKeyValue: req.auth.apiKeyValue ? sub(req.auth.apiKeyValue) : req.auth.apiKeyValue
        }
      : req.auth
  }
}

/** 收集一份请求里所有引用到的变量名，用于"未定义变量"提示。 */
export function collectRefs(req: RequestSpec): string[] {
  const texts: string[] = [req.url]
  for (const list of [req.headers, req.params, req.cookies, req.form]) {
    for (const kv of list ?? []) {
      texts.push(kv.name, kv.value)
    }
  }
  for (const f of req.files ?? []) {
    texts.push(f.path)
  }
  if (req.binaryPath) texts.push(req.binaryPath)
  if (req.body) texts.push(req.body)
  if (req.auth) {
    texts.push(
      req.auth.username ?? '',
      req.auth.password ?? '',
      req.auth.token ?? '',
      req.auth.apiKeyValue ?? ''
    )
  }
  return [...new Set(texts.flatMap(referencedVars))]
}
