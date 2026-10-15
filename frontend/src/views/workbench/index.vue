<script lang="ts" setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Collection, Clock, Grid } from '@element-plus/icons-vue'
import type { CollectionItem, HistoryEntry, RequestSpec } from '@/api/types'
import { useWorkspaceStore } from '@/store/modules/workspace'
import { useWorkbenchStore } from '@/store/modules/workbench'
import { useMessage } from '@/hooks/web/useMessage'
import SplitPane from './components/SplitPane.vue'
import RequestTabs from './components/RequestTabs.vue'
import RequestLine from './components/RequestLine.vue'
import ParamsPanel from './components/ParamsPanel.vue'
import HeadersPanel from './components/HeadersPanel.vue'
import CookiesPanel from './components/CookiesPanel.vue'
import BodyPanel from './components/BodyPanel.vue'
import AuthPanel from './components/AuthPanel.vue'
import OptionsPanel from './components/OptionsPanel.vue'
import ResponsePanel from './components/ResponsePanel.vue'
import CollectionsPanel from './components/CollectionsPanel.vue'
import HistoryPanel from './components/HistoryPanel.vue'
import SaveDialog from './components/SaveDialog.vue'
import CurlDialog from './components/CurlDialog.vue'
import EnvironmentsDialog from './components/EnvironmentsDialog.vue'
import BatchDialog from './components/BatchDialog.vue'

// 组件名必须与路由 name 一致：KeepAlive 的 include 匹配组件名，
// 而 SFC 默认推断出的名字是文件名（index）。约定见 router/routes.ts。
defineOptions({ name: 'Workbench' })

const workspace = useWorkspaceStore()
const workbench = useWorkbenchStore()
const { success, error: toastError } = useMessage()

/** 请求配置区当前选中的页签。 */
const requestTab = ref<'params' | 'headers' | 'cookies' | 'body' | 'auth' | 'options'>('params')

const saveVisible = ref(false)
const curlVisible = ref(false)
const envVisible = ref(false)
const batchVisible = ref(false)
/** 批量运行的目标集合，由左侧集合行的"运行"入口设置。 */
const batchTarget = ref<import('@/api/types').Collection | null>(null)

const activeTab = computed(() => workbench.activeTab)

const paramsCount = computed(
  () => (activeTab.value?.request.params ?? []).filter((p) => p.enabled && p.name).length
)
const headersCount = computed(
  () => (activeTab.value?.request.headers ?? []).filter((h) => h.enabled && h.name).length
)
const cookiesCount = computed(
  () => (activeTab.value?.request.cookies ?? []).filter((c) => c.enabled && c.name).length
)

const bodyKind = computed(() => activeTab.value?.request.bodyKind ?? 'none')

const requestId = computed(() =>
  activeTab.value ? (workbench.requestIds[activeTab.value.id] ?? '') : ''
)

// --- 动作 ---

function patchRequest(patch: Partial<RequestSpec>) {
  workbench.patchRequest(patch)
}

async function send() {
  const tab = activeTab.value
  if (!tab) return
  if (!tab.request.url.trim()) {
    toastError('请先填写请求地址')
    return
  }
  const result = await workbench.send(tab.id)
  if (result?.error) {
    toastError(result.error)
  }
}

function cancel() {
  workbench.cancelSend()
}

function openFromCollection(
  spec: RequestSpec,
  source: { collectionId: string; itemId: string } | undefined,
  title: string
) {
  workbench.openSpec(spec, source, title)
}

function openFromHistory(entry: HistoryEntry) {
  workbench.openHistory(entry)
}

/** 从批量运行对话框点某一条，直接在工作台里打开它。 */
function openBatchItem(item: CollectionItem) {
  const collection = batchTarget.value
  workbench.openSpec(
    item.request,
    collection ? { collectionId: collection.id, itemId: item.id } : undefined,
    item.name
  )
  batchVisible.value = false
}

function openBatch(collection: import('@/api/types').Collection) {
  batchTarget.value = collection
  batchVisible.value = true
}

function onCurlImported(spec: RequestSpec) {
  workbench.replaceRequest(spec)
}

function onSaved(collectionId: string, itemId: string, name: string) {
  workbench.markSaved(collectionId, itemId, name)
}

async function importFile() {
  try {
    const result = await workspace.importFromFile()
    if (!result) return
    success(`导入完成：${result.collections} 个集合、${result.history} 条历史`)
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

// --- 快捷键 ---

/**
 * 全局快捷键。
 *
 * 只在 workbench 页面上生效（组件挂载时才注册），并且对输入框里的
 * Ctrl+Enter 放行给输入框自己处理。
 */
function onKeydown(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey)) return
  const key = event.key.toLowerCase()

  if (key === 't') {
    event.preventDefault()
    workbench.newTab()
    return
  }
  if (key === 's') {
    event.preventDefault()
    saveVisible.value = true
    return
  }
  if (key === 'h') {
    event.preventDefault()
    workbench.panel = 'history'
    return
  }
  if (key === 'i') {
    event.preventDefault()
    curlVisible.value = true
    return
  }
  if (event.key === 'Enter') {
    event.preventDefault()
    send()
  }
}

onMounted(async () => {
  await workspace.load()
  workbench.ensureTab()
  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="workbench">
    <SplitPane mode="horizontal" :initial="0.2" :min-first="190" :min-second="480">
      <template #first>
        <div class="left">
          <div class="left-switch">
            <button
              class="switch-btn"
              :class="{ 'is-active': workbench.panel === 'collections' }"
              type="button"
              @click="workbench.panel = 'collections'"
            >
              <ElIcon><Grid /></ElIcon>
              集合
            </button>
            <button
              class="switch-btn"
              :class="{ 'is-active': workbench.panel === 'history' }"
              type="button"
              @click="workbench.panel = 'history'"
            >
              <ElIcon><Clock /></ElIcon>
              历史
            </button>
          </div>

          <div class="left-body">
            <CollectionsPanel
              v-show="workbench.panel === 'collections'"
              @open="openFromCollection"
            />
            <HistoryPanel v-show="workbench.panel === 'history'" @open="openFromHistory" />
          </div>

          <div class="left-foot">
            <ElButton link size="small" @click="envVisible = true">环境变量</ElButton>
            <ElButton link size="small" @click="importFile">导入</ElButton>
            <div class="spacer" />
            <ElButton
              link
              size="small"
              :disabled="!workspace.collections.length"
              @click="openBatch(workspace.collections[0])"
            >
              批量运行
            </ElButton>
          </div>
        </div>
      </template>

      <template #second>
        <div class="main">
          <RequestTabs
            :tabs="workbench.tabs"
            :active-id="workbench.activeId"
            @select="workbench.setActive"
            @close="workbench.closeTab"
            @close-others="workbench.closeOtherTabs"
            @close-all="workbench.closeTab(workbench.activeId)"
            @create="workbench.newTab()"
          />

          <template v-if="activeTab">
            <RequestLine
              :request="activeTab.request"
              :sending="activeTab.sending"
              :saved-title="activeTab.source ? activeTab.title : ''"
              @update="patchRequest"
              @send="send"
              @cancel="cancel"
              @save="saveVisible = true"
              @curl="curlVisible = true"
              @history="workbench.panel = 'history'"
            />

            <SplitPane
              class="main-split"
              mode="vertical"
              :initial="0.48"
              :min-first="180"
              :min-second="140"
            >
              <template #first>
                <div class="req-config">
                  <ElTabs v-model="requestTab" class="compact-tabs">
                    <ElTabPane name="params">
                      <template #label>
                        <span
                          >Params<span v-if="paramsCount" class="tab-count">{{
                            paramsCount
                          }}</span></span
                        >
                      </template>
                      <ParamsPanel :request="activeTab.request" @update="patchRequest" />
                    </ElTabPane>

                    <ElTabPane name="headers">
                      <template #label>
                        <span
                          >Headers<span v-if="headersCount" class="tab-count">{{
                            headersCount
                          }}</span></span
                        >
                      </template>
                      <HeadersPanel :request="activeTab.request" @update="patchRequest" />
                    </ElTabPane>

                    <ElTabPane name="cookies">
                      <template #label>
                        <span
                          >Cookies<span v-if="cookiesCount" class="tab-count">{{
                            cookiesCount
                          }}</span></span
                        >
                      </template>
                      <CookiesPanel :request="activeTab.request" @update="patchRequest" />
                    </ElTabPane>

                    <ElTabPane name="body">
                      <template #label>
                        <span>Body<span v-if="bodyKind !== 'none'" class="tab-dot" /></span>
                      </template>
                      <BodyPanel :request="activeTab.request" @update="patchRequest" />
                    </ElTabPane>

                    <ElTabPane name="auth">
                      <template #label>
                        <span
                          >Auth<span
                            v-if="(activeTab.request.auth?.kind ?? 'none') !== 'none'"
                            class="tab-dot"
                        /></span>
                      </template>
                      <AuthPanel :request="activeTab.request" @update="patchRequest" />
                    </ElTabPane>

                    <ElTabPane name="options">
                      <template #label><span>选项</span></template>
                      <OptionsPanel :request="activeTab.request" @update="patchRequest" />
                    </ElTabPane>
                  </ElTabs>
                </div>
              </template>

              <template #second>
                <ResponsePanel
                  :response="activeTab.response"
                  :sending="activeTab.sending"
                  :request-id="requestId"
                  :request="activeTab.request"
                />
              </template>
            </SplitPane>
          </template>

          <div v-else class="no-tab">
            <ElButton type="primary" @click="workbench.newTab()">新建请求</ElButton>
          </div>
        </div>
      </template>
    </SplitPane>

    <SaveDialog
      v-if="activeTab"
      v-model="saveVisible"
      :request="activeTab.request"
      :default-name="activeTab.title"
      :source="activeTab.source"
      @saved="onSaved"
    />
    <CurlDialog
      v-if="activeTab"
      v-model="curlVisible"
      :request="activeTab.request"
      @import="onCurlImported"
    />
    <EnvironmentsDialog v-model="envVisible" />
    <BatchDialog v-model="batchVisible" :collection="batchTarget" @open="openBatchItem" />
  </div>
</template>

<style lang="scss" scoped>
.workbench {
  display: flex;
  width: 100%;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

.left {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  overflow: hidden;
  border-right: 1px solid var(--chrome-border);
}

/* 集合 / 历史 用两个并列按钮而不是 segmented 控件：
   这里只有两个互斥选项，分段控件的滑块动画在窄侧栏里反而显得晃眼。 */
.left-switch {
  display: flex;
  flex: 0 0 auto;
  border-bottom: 1px solid var(--chrome-border);
}

.switch-btn {
  display: flex;
  flex: 1 1 0;
  gap: 4px;
  align-items: center;
  justify-content: center;
  height: 32px;
  font-size: 12px;
  color: var(--text-color-secondary);
  cursor: pointer;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  transition:
    color 0.12s ease,
    border-color 0.12s ease;
}

.switch-btn:hover {
  color: var(--text-color);
}

.switch-btn.is-active {
  color: var(--el-color-primary);
  border-bottom-color: var(--el-color-primary);
}

.left-body {
  flex: 1 1 auto;
  min-height: 0;
  overflow: hidden;
}

.left-body > :deep(*) {
  height: 100%;
}

.left-foot {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  padding: 2px 6px;
  border-top: 1px solid var(--chrome-border);
}

.spacer {
  flex: 1 1 auto;
}

.main {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  height: 100%;
  background-color: var(--app-bgc);
}

.main-split {
  flex: 1 1 auto;
  min-height: 0;
}

.req-config {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  border-bottom: 1px solid var(--chrome-border);
}

/* 请求配置区的页签同样压扁，理由见 ResponsePanel 里的说明。 */
.compact-tabs {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;

  :deep(.el-tabs__header) {
    flex: 0 0 auto;
    margin: 0;
    padding: 0 12px;
  }

  :deep(.el-tabs__item) {
    height: 32px;
    font-size: 12px;
    line-height: 32px;
  }

  :deep(.el-tabs__content) {
    flex: 1 1 auto;
    min-height: 0;
    overflow: hidden;
  }

  :deep(.el-tab-pane) {
    height: 100%;
  }
}

.tab-count {
  display: inline-block;
  min-width: 16px;
  height: 15px;
  margin-left: 5px;
  padding: 0 4px;
  font-size: 10px;
  line-height: 15px;
  color: var(--text-color-secondary);
  text-align: center;
  background-color: var(--el-fill-color);
  border-radius: 8px;
}

/* 有内容的页签上点一个小圆点：比数字更轻，但对于"有没有填"这种二元信息足够了。 */
.tab-dot {
  display: inline-block;
  width: 5px;
  height: 5px;
  margin-left: 5px;
  vertical-align: middle;
  background-color: var(--el-color-primary);
  border-radius: 50%;
}

.no-tab {
  display: flex;
  flex: 1 1 auto;
  align-items: center;
  justify-content: center;
}
</style>
