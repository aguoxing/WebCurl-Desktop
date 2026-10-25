/** 常用请求头字典：名称联想的候选，也是"常用请求头"一键插入的来源。 */

/**
 * 一条常用请求头。
 *
 * 同一个头可能出现多条（Content-Type 的 json 与表单编码都很常用），
 * 名称联想会按名称去重，一键插入则逐条保留。
 */
export interface CommonHeaderOption {
  /** 头名称，按标准大小写书写。 */
  name: string
  /** 选中该名称后填入的推荐值；空串表示只补名称，具体值由用户填写。 */
  value: string
  /** 一句话说明，在列表里帮用户分辨用途。 */
  note: string
}

/**
 * 字典顺序即展示顺序：越常用越靠前，联想的首屏才有意义。
 */
export const COMMON_HEADER_OPTIONS: CommonHeaderOption[] = [
  { name: 'Content-Type', value: 'application/json', note: '请求体是 JSON' },
  {
    name: 'Content-Type',
    value: 'application/x-www-form-urlencoded',
    note: '请求体是表单'
  },
  { name: 'Accept', value: 'application/json', note: '期望返回 JSON' },
  { name: 'Accept', value: '*/*', note: '接受任意格式' },
  { name: 'Authorization', value: 'Bearer ', note: '鉴权令牌' },
  { name: 'Accept-Encoding', value: 'gzip, deflate, br', note: '可接受的压缩方式' },
  { name: 'Accept-Language', value: 'zh-CN,zh;q=0.9', note: '语言偏好' },
  { name: 'User-Agent', value: 'WebCurl/1.0', note: '客户端标识' },
  { name: 'Cache-Control', value: 'no-cache', note: '缓存策略' },
  { name: 'Range', value: 'bytes=0-1023', note: '分段下载' },
  { name: 'Cookie', value: '', note: '会话 Cookie' },
  { name: 'Referer', value: '', note: '来源页面' },
  { name: 'Origin', value: '', note: '跨域来源' },
  { name: 'If-None-Match', value: '', note: '条件请求，值为 ETag' },
  { name: 'If-Modified-Since', value: '', note: '条件请求，值为时间' },
  { name: 'X-Request-Id', value: '', note: '链路追踪 ID' },
  { name: 'X-Api-Key', value: '', note: '自定义鉴权' },
  { name: 'Content-Length', value: '', note: '由发送端计算，通常不用手写' }
]

/**
 * 名称联想用的去重清单：同名头只保留第一次出现的推荐值。
 *
 * 联想列表里出现两条一模一样的 Content-Type 只会让人犹豫该点哪条，
 * 而一键插入那边保留两条正是因为值不同。
 */
export const COMMON_HEADER_NAMES: CommonHeaderOption[] = COMMON_HEADER_OPTIONS.reduce<
  CommonHeaderOption[]
>((list, option) => {
  if (!list.some((item) => item.name.toLowerCase() === option.name.toLowerCase())) {
    list.push(option)
  }
  return list
}, [])
