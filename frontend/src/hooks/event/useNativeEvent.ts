import { onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'

// 事件名是本前端与 Go 侧之间的契约，必须与 internal/app/mock.go、menu.go 中
// 的同名常量逐一对应；改名要两边同时改。
export const NativeEvent = {
  /** Go → JS：原生菜单请求前端跳转。payload 为路由路径，如 "/setting"。 */
  Navigate: 'nav:goto',
  /** JS → Go：把当前界面语言告知 Go 侧，用于重建原生菜单文案。payload 如 "zh-CN"。 */
  Language: 'app:language',
  /** Go → JS：一条 Mock 请求日志。payload 为 MockRequestLog。 */
  MockLog: 'mock:log',
  /** Go → JS：Mock 引擎状态变化（托盘菜单启停、配置加载失败等）。 */
  MockStatus: 'mock:status',
  /** Go → JS：日志已被清空，界面列表也应清掉。 */
  MockLogsCleared: 'mock:logs-cleared'
} as const

/** 原生事件名类型。 */
export type NativeEventName = (typeof NativeEvent)[keyof typeof NativeEvent]

/**
 * 订阅一个来自 Go 侧的事件，并在组件卸载时自动退订。
 *
 * handler 收到的是事件的 data 部分（Wails 的 WailsEvent 包装已被剥掉），
 * 因此调用方不必再关心宿主的事件结构。
 *
 * 在浏览器里单独跑前端（pnpm dev）时没有 Wails 运行时，此时订阅不会生效，
 * 但也不会抛错——原生能力整体降级的语义由调用方自行处理。
 */
export const useNativeEvent = <T = unknown>(name: NativeEventName, handler: (data: T) => void) => {
  let off: (() => void) | null = null

  onMounted(() => {
    off = Events.On(name, (event: any) => handler(event?.data as T))
  })

  onUnmounted(() => {
    off?.()
    off = null
  })
}

/**
 * 向 Go 侧投递一个事件。
 *
 * 刻意不判断"是否在 Wails 宿主里"：运行时是否可用只能靠实际调用得知，
 * 而失败的后果仅仅是原生侧收不到通知，因此这里统一吞掉异常，
 * 让浏览器预览与桌面运行共用同一份调用代码。
 */
export const emitNative = async (name: NativeEventName, data?: unknown) => {
  try {
    await Events.Emit(name, data as any)
  } catch (error) {
    console.debug(`[native] event "${name}" was not delivered:`, error)
  }
}
