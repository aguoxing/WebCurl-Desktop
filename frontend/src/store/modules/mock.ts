import { defineStore } from 'pinia'
import { mockApi } from '@/api'
import type { MockRequestLog, MockStatus } from '@/api/types'

/** 界面里保留的日志条数上限。再多就没人会往回翻了，只会让滚动变卡。 */
const MAX_LOGS = 500

interface MockState {
  status: MockStatus
  config: string
  configPath: string
  logs: MockRequestLog[]
  loading: boolean
  saving: boolean
}

export const useMockStore = defineStore('mock', {
  state: (): MockState => ({
    status: {
      running: false,
      configPath: '',
      listeners: [],
      lastError: ''
    },
    config: '{}',
    configPath: '',
    logs: [],
    loading: false,
    saving: false
  }),

  getters: {
    /** 引擎是否在跑。 */
    running(state): boolean {
      return state.status.running
    },

    /** 第一个监听的可访问地址，用于界面提示"请求该发到哪"。 */
    primaryAddress(state): string {
      const first = state.status.listeners?.[0]
      if (!first) return ''
      const host =
        !first.host || first.host === '0.0.0.0' || first.host === '::' ? '127.0.0.1' : first.host
      return `${host}:${first.port}`
    }
  },

  actions: {
    /** 首次进入 Mock 页面时加载状态、配置与历史日志。 */
    async load() {
      this.loading = true
      try {
        await Promise.all([this.refreshStatus(), this.loadConfig(), this.loadLogs()])
      } finally {
        this.loading = false
      }
    },

    async refreshStatus() {
      this.status = await mockApi.status()
      this.configPath = this.status.configPath || (await mockApi.configPath())
    },

    async loadConfig() {
      this.config = await mockApi.getConfig()
    },

    async loadLogs() {
      this.logs = await mockApi.recentLogs(MAX_LOGS)
    },

    /** 追加一条实时日志（由 Go 侧事件触发）。 */
    appendLog(entry: MockRequestLog) {
      this.logs.push(entry)
      if (this.logs.length > MAX_LOGS) {
        this.logs.splice(0, this.logs.length - MAX_LOGS)
      }
    },

    async clearLogs() {
      await mockApi.clearLogs()
      this.logs = []
    },

    async start() {
      await mockApi.start()
      await this.refreshStatus()
    },

    async stop() {
      await mockApi.stop()
      await this.refreshStatus()
    },

    async restart() {
      await mockApi.restart()
      await this.refreshStatus()
    },

    /** 保存配置并热加载。 */
    async saveConfig(config: string) {
      this.saving = true
      try {
        await mockApi.saveConfig(config)
        this.config = config
        await this.refreshStatus()
      } finally {
        this.saving = false
      }
    },

    /** 只热加载，不写盘（用当前编辑框里的内容试一次）。 */
    async reload(config: string) {
      await mockApi.reload(config)
      await this.refreshStatus()
    }
  }
})
