import { defineStore } from 'pinia'
import { configApi, workspaceApi } from '@/api'
import type {
  Collection,
  CollectionItem,
  DataPath,
  Environment,
  HistoryEntry,
  RequestSpec,
  WorkspaceData
} from '@/api/types'
import { toVarMap } from '@/utils/variables'
import { shortId } from '@/utils/format'

/** 新建请求时使用的默认参数。 */
export interface RequestDefaults {
  timeoutSec: number
  retryCount: number
  retryDelaySec: number
  skipSslVerify: boolean
  noFollowRedirect: boolean
}

interface WorkspaceState {
  collections: Collection[]
  history: HistoryEntry[]
  environments: Environment[]
  activeEnv: string
  dataPath: DataPath
  defaults: RequestDefaults
  loaded: boolean
  loading: boolean
}

/**
 * 按配置里的默认值构造一份新请求。
 *
 * 默认值来自后端配置（设置页可改），而不是前端写死：两处各写一份必然会漂移，
 * 而"我明明在设置里关掉了跟随重定向，新建的请求却还是跟着跳"是极难自查的问题。
 */
export function emptyRequest(defaults?: Partial<RequestDefaults>): RequestSpec {
  return {
    method: 'GET',
    url: '',
    params: [],
    headers: [],
    cookies: [],
    bodyKind: 'none',
    body: '',
    form: [],
    files: [],
    binaryPath: '',
    auth: { kind: 'none' },
    timeoutSec: defaults?.timeoutSec ?? 30,
    retryCount: defaults?.retryCount ?? 0,
    retryDelaySec: defaults?.retryDelaySec ?? 1,
    skipSslVerify: defaults?.skipSslVerify ?? false,
    noFollowRedirect: defaults?.noFollowRedirect ?? false,
    proxyUrl: '',
    acceptEncoding: ''
  }
}

export const useWorkspaceStore = defineStore('workspace', {
  state: (): WorkspaceState => ({
    collections: [],
    history: [],
    environments: [],
    activeEnv: '',
    dataPath: { dir: '', file: '' },
    defaults: {
      timeoutSec: 30,
      retryCount: 0,
      retryDelaySec: 1,
      skipSslVerify: false,
      noFollowRedirect: false
    },
    loaded: false,
    loading: false
  }),

  getters: {
    /** 当前激活的环境。 */
    activeEnvironment(state): Environment | null {
      return state.environments.find((e) => e.id === state.activeEnv) ?? null
    },

    /** 当前生效的变量表（供 {{}} 替换使用）。 */
    variables(): Map<string, string> {
      return toVarMap(this.activeEnvironment?.variables)
    },

    /** 当前环境下已定义的变量名集合。 */
    definedVarNames(): Set<string> {
      return new Set(
        (this.activeEnvironment?.variables ?? [])
          .filter((v) => v.enabled && v.name)
          .map((v) => v.name)
      )
    }
  },

  actions: {
    /** 首次进入工作台时加载全部数据与默认参数。 */
    async load() {
      if (this.loaded || this.loading) return
      this.loading = true
      try {
        await Promise.all([this.refresh(), this.loadDefaults()])
        this.loaded = true
      } finally {
        this.loading = false
      }
    },

    async loadDefaults() {
      try {
        const cfg = await configApi.getRequest()
        this.defaults = {
          timeoutSec: cfg.defaultTimeoutSec ?? 30,
          retryCount: cfg.defaultRetryCount ?? 0,
          retryDelaySec: cfg.defaultRetryDelaySec ?? 1,
          skipSslVerify: cfg.verifySSL === 0,
          noFollowRedirect: cfg.followRedirect === 0
        }
      } catch (error) {
        console.error('[workspace] 读取默认参数失败，沿用内置值', error)
      }
    },

    /** 从后端重新拉取完整快照。 */
    async refresh() {
      const [snapshot, path] = await Promise.all([workspaceApi.snapshot(), workspaceApi.path()])
      this.applySnapshot(snapshot)
      this.dataPath = path
    },

    applySnapshot(data: WorkspaceData) {
      this.collections = data.collections ?? []
      this.history = data.history ?? []
      this.environments = data.environments ?? []
      this.activeEnv = data.activeEnv ?? ''
    },

    /** 新建一份请求（使用当前默认参数）。 */
    newRequest(): RequestSpec {
      return emptyRequest(this.defaults)
    },

    // --- 历史 ---

    /**
     * 记录一条历史。
     *
     * 状态与耗时由调用方在发送完成后补齐：先落一条"在发"的记录会让历史里出现
     * 一串没有结果的条目，反而干扰阅读。
     */
    async addHistory(entry: HistoryEntry): Promise<HistoryEntry[]> {
      this.history = await workspaceApi.addHistory(entry)
      return this.history
    },

    async deleteHistory(id: string) {
      this.history = await workspaceApi.deleteHistory(id)
    },

    async clearHistory() {
      await workspaceApi.clearHistory()
      this.history = []
    },

    // --- 集合 ---

    async saveCollection(collection: Collection) {
      this.collections = await workspaceApi.saveCollection(collection)
      return this.collections
    },

    /** 新建一个空集合。 */
    createCollection(name: string): Collection {
      const collection: Collection = {
        id: shortId(),
        name,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        items: []
      }
      return collection
    },

    /** 把请求存进集合（集合不存在时先建）。 */
    async addToCollection(collectionId: string, name: string, request: RequestSpec) {
      const collection = this.collections.find((c) => c.id === collectionId)
      if (!collection) throw new Error('集合不存在')

      const item: CollectionItem = {
        id: shortId(),
        name,
        request: JSON.parse(JSON.stringify(request))
      }
      collection.items = [...collection.items, item]
      await this.saveCollection(collection)
      return item
    },

    /** 更新集合里的某个接口。 */
    async updateCollectionItem(collectionId: string, item: CollectionItem) {
      const collection = this.collections.find((c) => c.id === collectionId)
      if (!collection) throw new Error('集合不存在')
      collection.items = collection.items.map((i) => (i.id === item.id ? item : i))
      await this.saveCollection(collection)
    },

    async deleteCollection(id: string) {
      this.collections = await workspaceApi.deleteCollection(id)
    },

    async deleteCollectionItem(collectionId: string, itemId: string) {
      const collection = this.collections.find((c) => c.id === collectionId)
      if (!collection) return
      collection.items = collection.items.filter((i) => i.id !== itemId)
      await this.saveCollection(collection)
    },

    // --- 环境 ---

    async saveEnvironment(env: Environment) {
      this.environments = await workspaceApi.saveEnvironment(env)
      return this.environments
    },

    async deleteEnvironment(id: string) {
      this.environments = await workspaceApi.deleteEnvironment(id)
      if (this.activeEnv === id) this.activeEnv = ''
    },

    async setActiveEnvironment(id: string) {
      await workspaceApi.setActiveEnvironment(id)
      this.activeEnv = id
    },

    // --- 导入导出 ---

    async exportToFile(): Promise<string> {
      const data = await workspaceApi.exportData()
      const now = new Date()
      const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, '0')}${String(
        now.getDate()
      ).padStart(2, '0')}`
      return workspaceApi.exportToFile(
        `webcurl-workspace-${stamp}.json`,
        JSON.stringify(data, null, 2)
      )
    },

    /** 从文件导入：返回导入的集合数量，便于给出具体反馈。 */
    async importFromFile(): Promise<{ collections: number; history: number } | null> {
      const text = await workspaceApi.importFromFile()
      if (!text) return null

      let data: WorkspaceData
      try {
        data = JSON.parse(text)
      } catch {
        throw new Error('文件内容不是合法 JSON')
      }
      if (!Array.isArray(data.collections) && !Array.isArray(data.history)) {
        throw new Error('文件里没有可识别的集合或历史')
      }

      await workspaceApi.importData({
        version: data.version ?? 1,
        history: data.history ?? [],
        collections: data.collections ?? [],
        environments: data.environments ?? [],
        activeEnv: data.activeEnv ?? '',
        savedAt: data.savedAt ?? new Date().toISOString()
      })
      await this.refresh()
      return {
        collections: this.collections.length,
        history: this.history.length
      }
    },

    async openDataDir() {
      await workspaceApi.openDataDir()
    }
  }
})
