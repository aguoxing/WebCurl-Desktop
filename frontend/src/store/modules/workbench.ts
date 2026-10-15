import { defineStore } from 'pinia'
import { requestApi } from '@/api'
import type { HistoryEntry, RequestSpec, RequestTab, SendResult } from '@/api/types'
import { shortId, shortUrl } from '@/utils/format'
import { applyVariables } from '@/utils/variables'
import { emptyRequest, useWorkspaceStore } from './workspace'

interface WorkbenchState {
  tabs: RequestTab[]
  activeId: string
  /** 左侧面板当前显示的是集合还是历史。 */
  panel: 'collections' | 'history'
  /**
   * 每个标签页最近一次发送所用的请求 ID。
   *
   * 单独存而不是塞进 request 里：request 会被原样发给后端，多带一个后端不认识的
   * 字段会让"界面看到的内容"和"实际发出的内容"出现差异——而这个应用的全部价值
   * 就在于这两者一致。
   */
  requestIds: Record<string, string>
}

/** 标签标题：优先取用户/集合给的名字，否则由方法 + 路径推导。 */
function deriveTitle(req: RequestSpec): string {
  if (!req.url) return '新建请求'
  return shortUrl(req.url)
}

export const useWorkbenchStore = defineStore('workbench', {
  state: (): WorkbenchState => ({
    tabs: [],
    activeId: '',
    panel: 'collections',
    requestIds: {}
  }),

  getters: {
    activeTab(state): RequestTab | null {
      return state.tabs.find((t) => t.id === state.activeId) ?? null
    },

    /** 发送中的标签数量，用于顶栏的整体忙碌提示。 */
    sendingCount(state): number {
      return state.tabs.filter((t) => t.sending).length
    }
  },

  actions: {
    /** 保证至少有一个标签页。首次进入工作台时调用。 */
    ensureTab() {
      if (this.tabs.length === 0) {
        this.newTab()
      }
    },

    /** 新建一个空标签页并激活。 */
    newTab(spec?: RequestSpec): RequestTab {
      const workspace = useWorkspaceStore()
      const tab: RequestTab = {
        id: shortId(),
        title: spec ? deriveTitle(spec) : '新建请求',
        request: spec ? cloneSpec(spec) : workspace.newRequest(),
        response: null,
        sending: false,
        dirty: false
      }
      this.tabs.push(tab)
      this.activeId = tab.id
      return tab
    },

    /**
     * 在标签页里打开一份请求。
     *
     * 已经打开过同一个集合条目时直接切过去，而不是再开一个：反复点同一个接口
     * 会堆出一排同名标签，用户还得自己分辨哪个改了。
     */
    openSpec(spec: RequestSpec, source?: RequestTab['source'], title?: string) {
      if (source) {
        const existed = this.tabs.find(
          (t) =>
            t.source?.collectionId === source.collectionId && t.source?.itemId === source.itemId
        )
        if (existed) {
          this.activeId = existed.id
          return existed
        }
      }

      const tab: RequestTab = {
        id: shortId(),
        title: title || deriveTitle(spec),
        request: cloneSpec(spec),
        response: null,
        sending: false,
        dirty: false,
        source
      }
      this.tabs.push(tab)
      this.activeId = tab.id
      return tab
    },

    closeTab(id: string) {
      const idx = this.tabs.findIndex((t) => t.id === id)
      if (idx < 0) return
      this.tabs.splice(idx, 1)

      if (this.activeId === id) {
        // 关掉当前标签后激活相邻的一个：优先右边（用户的下一个心理目标），
        // 没有右边就取左边。
        const next = this.tabs[idx] ?? this.tabs[idx - 1]
        this.activeId = next?.id ?? ''
      }
      if (this.tabs.length === 0) {
        this.newTab()
      }
    },

    closeOtherTabs(id: string) {
      this.tabs = this.tabs.filter((t) => t.id === id)
      this.activeId = id
    },

    setActive(id: string) {
      this.activeId = id
    },

    /** 修改当前标签的请求内容。 */
    patchRequest(patch: Partial<RequestSpec>) {
      const tab = this.activeTab
      if (!tab) return
      tab.request = { ...tab.request, ...patch }
      tab.dirty = true
      if (!tab.source) {
        tab.title = deriveTitle(tab.request)
      }
    },

    /** 整体替换请求（如从 curl 导入、加载历史）。 */
    replaceRequest(spec: RequestSpec, title?: string) {
      const tab = this.activeTab
      if (!tab) return
      tab.request = cloneSpec(spec)
      tab.dirty = true
      tab.title = title || deriveTitle(tab.request)
    },

    /** 保存到集合后回写来源与标题，使"另存为"变成"保存"。 */
    markSaved(collectionId: string, itemId: string, title: string) {
      const tab = this.activeTab
      if (!tab) return
      tab.source = { collectionId, itemId }
      tab.title = title
      tab.dirty = false
    },

    /**
     * 发送当前标签的请求。
     *
     * 变量替换在发送前一刻做：地址栏里保留 {{}} 原文，用户切换环境后再发就能
     * 打到另一个服务，而不用把请求改两遍。
     */
    async send(tabId?: string): Promise<SendResult | null> {
      const tab = this.tabs.find((t) => t.id === (tabId ?? this.activeId))
      if (!tab) return null
      if (tab.sending) return null

      const workspace = useWorkspaceStore()
      const resolved = applyVariables(tab.request, workspace.variables)

      // 请求 ID 每次都换：后端用它关联取消与"另存为"的响应体缓存，
      // 复用 ID 会让上一次的响应体被覆盖（缓存按 ID 去重）。
      const requestId = `${tab.id}-${shortId()}`
      tab.sending = true
      this.requestIds[tab.id] = requestId

      try {
        const result = await requestApi.send(requestId, resolved)
        tab.response = result
        this.recordHistory(tab, result)
        return result
      } catch (error) {
        // 到这里说明是"调用本身失败"（如后端未就绪），与请求失败（result.error）不同。
        const message = error instanceof Error ? error.message : String(error)
        tab.response = {
          id: requestId,
          error: message,
          status: 0,
          statusText: '',
          proto: '',
          headers: [],
          cookies: [],
          isText: true,
          text: '',
          size: 0,
          previewTruncated: false,
          truncated: false,
          durationMs: 0,
          attempts: 0,
          finalUrl: resolved.url,
          redirects: 0,
          suggestedFileName: 'response'
        }
        this.recordHistory(tab, tab.response)
        return tab.response
      } finally {
        tab.sending = false
      }
    },

    async cancelSend(tabId?: string) {
      const id = tabId ?? this.activeId
      const requestId = this.requestIds[id]
      if (!requestId) return
      await requestApi.cancel(requestId)
    },

    /**
     * 写一条历史。
     *
     * 这里刻意不 await：历史写盘失败不该影响"请求已经发出去并拿到结果"这件事，
     * 而等待它会让响应面板的渲染被磁盘 IO 拖住。
     */
    recordHistory(tab: RequestTab, result: SendResult) {
      const workspace = useWorkspaceStore()
      const entry: HistoryEntry = {
        id: shortId(),
        at: new Date().toISOString(),
        request: cloneSpec(tab.request),
        status: result.status,
        durationMs: result.durationMs,
        size: result.size,
        error: result.error ?? ''
      }
      workspace.addHistory(entry).catch((error) => console.error('[workbench] 写入历史失败', error))
    },

    /** 把一条历史加载到新标签页。 */
    openHistory(entry: HistoryEntry) {
      this.openSpec(entry.request, undefined, deriveTitle(entry.request))
    }
  }
})

/** 深拷贝一份请求，避免标签页之间、以及请求与历史之间共享引用。 */
function cloneSpec(spec: RequestSpec): RequestSpec {
  return JSON.parse(JSON.stringify({ ...emptyRequest(), ...spec })) as RequestSpec
}
