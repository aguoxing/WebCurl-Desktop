/**
 * 对 Wails 生成绑定的薄封装。
 *
 * 为什么要再包一层：
 *  1. 生成的函数按 Go 的导出名（Send/GetConfig/PickFiles…）散在四五个文件里，
 *     界面代码直接引用会到处出现长路径 import；
 *  2. 后端的错误约定是"返回 string 表示失败、空串表示成功"，把这层翻译
 *     集中在这里，界面就不用每个调用点都写一次 `if (err) throw`。
 */
import * as RequestBinding from '@/bindings/webcurl-desktop/internal/binding/requestservicebinding'
import * as WorkspaceBinding from '@/bindings/webcurl-desktop/internal/binding/workspaceservicebinding'
import * as MockBinding from '@/bindings/webcurl-desktop/internal/binding/mockservicebinding'
import * as ConfigBinding from '@/bindings/webcurl-desktop/internal/binding/configservicebinding'
import type {
  Collection,
  DataPath,
  Environment,
  HistoryEntry,
  MockRequestLog,
  MockStatus,
  RequestSpec,
  SendResult,
  WorkspaceData
} from './types'

/** 后端以"空串表示成功"的方法统一在这里转成 Promise<void>。 */
async function assertOk(call: Promise<string>): Promise<void> {
  const err = await call
  if (err) {
    throw new Error(err)
  }
}

// --- 请求 ---

export const requestApi = {
  /** 发送请求。requestId 用于取消与"另存为"。 */
  send: (requestId: string, req: RequestSpec): Promise<SendResult> =>
    RequestBinding.Send(requestId, req as never) as unknown as Promise<SendResult>,

  cancel: (requestId: string): Promise<void> => RequestBinding.Cancel(requestId),

  /** 另存为完整响应体，返回实际写入路径（取消对话框时为空串）。 */
  saveBody: (requestId: string, suggestedName: string): Promise<string> =>
    RequestBinding.SaveBody(requestId, suggestedName),

  /** 弹出文件选择框，返回路径列表。 */
  pickFiles: (multiple = false): Promise<string[]> => RequestBinding.PickFiles(multiple),

  bodySize: (requestId: string): Promise<number> => RequestBinding.BodySize(requestId)
}

// --- 工作区（历史 / 集合 / 环境） ---

export const workspaceApi = {
  snapshot: (): Promise<WorkspaceData> =>
    WorkspaceBinding.Snapshot() as unknown as Promise<WorkspaceData>,

  path: (): Promise<DataPath> => WorkspaceBinding.Path() as unknown as Promise<DataPath>,

  addHistory: (entry: HistoryEntry): Promise<HistoryEntry[]> =>
    WorkspaceBinding.AddHistory(entry as never) as unknown as Promise<HistoryEntry[]>,

  updateHistory: (entry: HistoryEntry): Promise<HistoryEntry[]> =>
    WorkspaceBinding.UpdateHistory(entry as never) as unknown as Promise<HistoryEntry[]>,

  deleteHistory: (id: string): Promise<HistoryEntry[]> =>
    WorkspaceBinding.DeleteHistory(id) as unknown as Promise<HistoryEntry[]>,

  clearHistory: (): Promise<void> => WorkspaceBinding.ClearHistory(),

  saveCollection: (c: Collection): Promise<Collection[]> =>
    WorkspaceBinding.SaveCollection(c as never) as unknown as Promise<Collection[]>,

  deleteCollection: (id: string): Promise<Collection[]> =>
    WorkspaceBinding.DeleteCollection(id) as unknown as Promise<Collection[]>,

  saveEnvironment: (e: Environment): Promise<Environment[]> =>
    WorkspaceBinding.SaveEnvironment(e as never) as unknown as Promise<Environment[]>,

  deleteEnvironment: (id: string): Promise<Environment[]> =>
    WorkspaceBinding.DeleteEnvironment(id) as unknown as Promise<Environment[]>,

  setActiveEnvironment: (id: string): Promise<void> => WorkspaceBinding.SetActiveEnvironment(id),

  importData: (data: WorkspaceData): Promise<void> => WorkspaceBinding.ImportData(data as never),

  exportData: (): Promise<WorkspaceData> =>
    WorkspaceBinding.ExportData() as unknown as Promise<WorkspaceData>,

  openDataDir: (): Promise<void> => WorkspaceBinding.OpenDataDir(),

  /** 导出文本到用户选定的文件，返回路径（取消时为空串）。 */
  exportToFile: (suggestedName: string, content: string): Promise<string> =>
    WorkspaceBinding.ExportToFile(suggestedName, content),

  /** 让用户选一个文件并读回文本（取消时为空串）。 */
  importFromFile: (): Promise<string> => WorkspaceBinding.ImportFromFile()
}

// --- Mock ---

export const mockApi = {
  status: (): Promise<MockStatus> => MockBinding.Status() as unknown as Promise<MockStatus>,
  start: (): Promise<void> => MockBinding.Start(),
  stop: (): Promise<void> => MockBinding.Stop(),
  restart: (): Promise<void> => MockBinding.Restart(),
  getConfig: (): Promise<string> => MockBinding.GetConfig(),
  saveConfig: (raw: string): Promise<void> => MockBinding.SaveConfig(raw),
  reload: (raw: string): Promise<void> => MockBinding.Reload(raw),
  configPath: (): Promise<string> => MockBinding.ConfigPath(),
  recentLogs: (limit = 200): Promise<MockRequestLog[]> =>
    MockBinding.RecentLogs(limit) as unknown as Promise<MockRequestLog[]>,
  clearLogs: (): Promise<void> => MockBinding.ClearLogs()
}

// --- 配置 ---

export const configApi = {
  getAll: () => ConfigBinding.GetAllConfig(),
  getRequest: () => ConfigBinding.GetRequest(),
  updateRequest: (v: unknown) => assertOk(ConfigBinding.UpdateRequest(v as never)),
  getMock: () => ConfigBinding.GetMock(),
  updateMock: (v: unknown) => assertOk(ConfigBinding.UpdateMock(v as never)),
  getStorage: () => ConfigBinding.GetStorage(),
  updateStorage: (v: unknown) => assertOk(ConfigBinding.UpdateStorage(v as never)),
  updateStartup: (v: unknown) => assertOk(ConfigBinding.UpdateStartup(v as never)),
  updateUpdate: (v: unknown) => assertOk(ConfigBinding.UpdateUpdate(v as never)),
  updateTray: (v: unknown) => assertOk(ConfigBinding.UpdateTray(v as never)),
  getTray: () => ConfigBinding.GetTray(),
  getApp: () => ConfigBinding.GetApp(),
  getUpdate: () => ConfigBinding.GetUpdate()
}
