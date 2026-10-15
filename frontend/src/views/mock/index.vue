<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import {
  Delete,
  Refresh,
  RefreshRight,
  VideoPlay,
  VideoPause,
  Document
} from '@element-plus/icons-vue'
import { NativeEvent, useNativeEvent } from '@/hooks/event/useNativeEvent'
import type { MockRequestLog } from '@/api/types'
import { useMockStore } from '@/store/modules/mock'
import { useMessage } from '@/hooks/web/useMessage'
import { formatClock, formatBytes } from '@/utils/format'
import SplitPane from '@/views/workbench/components/SplitPane.vue'

// 组件名必须与路由 name 一致，详见 router/routes.ts 的约定说明。
defineOptions({ name: 'Mock' })

const mock = useMockStore()
const { success, error: toastError } = useMessage()

/** 编辑中的配置文本，与已保存的内容分开，便于显示"未保存"状态。 */
const draft = ref('')
const activeLogIndex = ref(-1)

const dirty = computed(() => draft.value !== mock.config)

/** 草稿是否是合法 JSON：不合法时禁止保存并给出提示，避免写坏配置文件。 */
const jsonError = computed(() => {
  const text = draft.value.trim()
  if (!text) return '配置不能为空'
  try {
    JSON.parse(text)
    return ''
  } catch (err) {
    return err instanceof Error ? err.message : 'JSON 解析失败'
  }
})

const canSave = computed(() => dirty.value && !jsonError.value)

const activeLog = computed<MockRequestLog | null>(() => mock.logs[activeLogIndex.value] ?? null)

const listenerText = computed(() =>
  mock.status.listeners?.length
    ? mock.status.listeners
        .map(
          (l) =>
            `${l.host || '0.0.0.0'}:${l.port}${l.protocols?.length ? ` [${l.protocols.join(',')}]` : ''}`
        )
        .join('  ·  ')
    : '—'
)

/** 复制一条日志的请求头，便于拿到 Mock 请求里的鉴权信息等。 */
async function copyHeaders(log: MockRequestLog) {
  const text = Object.entries(log.headers ?? {})
    .map(([name, value]) => `${name}: ${value}`)
    .join('\n')
  try {
    await navigator.clipboard.writeText(text)
    success('请求头已复制')
  } catch (err) {
    console.error('[mock] 复制失败', err)
    toastError('复制失败，剪贴板不可用')
  }
}

async function start() {
  try {
    await mock.start()
    success('Mock 引擎已启动')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

async function stop() {
  try {
    await mock.stop()
    success('Mock 引擎已停止')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

async function restart() {
  try {
    await mock.restart()
    success('Mock 引擎已重启')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

async function save() {
  if (!canSave.value) return
  try {
    await mock.saveConfig(draft.value)
    success('配置已保存并热加载')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

/** 只热加载当前草稿，不写盘——用来试一版配置是否可用。 */
async function tryReload() {
  if (jsonError.value) {
    toastError(jsonError.value)
    return
  }
  try {
    await mock.reload(draft.value)
    success('已按当前草稿热加载')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

function resetDraft() {
  draft.value = mock.config
}

async function clearLogs() {
  await mock.clearLogs()
  activeLogIndex.value = -1
}

function formatBody(body?: string): string {
  if (!body) return ''
  return body
}

// Go 侧的实时事件：引擎被托盘菜单启停、新的请求日志、日志被清空。
useNativeEvent<MockRequestLog>(NativeEvent.MockLog, (log) => {
  mock.appendLog(log)
  // 用户已经在看最新一条时，跟随滚动到新的这一条。
  if (activeLogIndex.value === -1 || activeLogIndex.value === mock.logs.length - 2) {
    activeLogIndex.value = mock.logs.length - 1
  }
})

useNativeEvent(NativeEvent.MockStatus, () => {
  mock.refreshStatus()
})

useNativeEvent(NativeEvent.MockLogsCleared, () => {
  mock.logs = []
  activeLogIndex.value = -1
})

onMounted(async () => {
  await mock.load()
  draft.value = mock.config
})
</script>

<template>
  <div class="mock">
    <div class="mock-bar">
      <ElTag :type="mock.running ? 'success' : 'info'" effect="dark" size="small">
        {{ mock.running ? '运行中' : '已停止' }}
      </ElTag>

      <span class="addr" :title="listenerText">{{ mock.primaryAddress || '未监听' }}</span>
      <span class="dim">{{ listenerText }}</span>

      <div class="spacer" />

      <ElButton v-if="!mock.running" size="small" type="primary" :icon="VideoPlay" @click="start">
        启动
      </ElButton>
      <template v-else>
        <ElButton size="small" :icon="RefreshRight" @click="restart">重启</ElButton>
        <ElButton size="small" :icon="VideoPause" @click="stop">停止</ElButton>
      </template>
    </div>

    <div v-if="mock.status.lastError" class="mock-error">
      {{ mock.status.lastError }}
    </div>

    <SplitPane
      class="mock-body"
      mode="horizontal"
      :initial="0.52"
      :min-first="360"
      :min-second="320"
    >
      <template #first>
        <div class="pane">
          <div class="pane-head">
            <span class="pane-title">配置</span>
            <ElTooltip :content="mock.configPath || '配置路径未知'" placement="bottom">
              <span class="config-path">
                <ElIcon><Document /></ElIcon>
                {{ mock.configPath ? '配置文件' : '配置路径未知' }}
              </span>
            </ElTooltip>
            <span v-if="dirty" class="dirty">未保存</span>
            <div class="spacer" />
            <ElButton link size="small" :disabled="!dirty" @click="resetDraft">撤销修改</ElButton>
            <ElButton link size="small" :disabled="Boolean(jsonError)" @click="tryReload">
              试加载
            </ElButton>
            <ElButton
              size="small"
              type="primary"
              :disabled="!canSave"
              :loading="mock.saving"
              @click="save"
            >
              保存并热加载
            </ElButton>
          </div>

          <div v-if="jsonError && draft.trim()" class="json-error"
            >JSON 格式错误：{{ jsonError }}</div
          >

          <ElInput
            v-model="draft"
            class="config-editor"
            type="textarea"
            resize="none"
            spellcheck="false"
            placeholder='{ "listeners": [ ... ], "routes": [ ... ] }'
          />
        </div>
      </template>

      <template #second>
        <div class="pane">
          <div class="pane-head">
            <span class="pane-title">最近请求</span>
            <span class="dim">{{ mock.logs.length }} 条</span>
            <div class="spacer" />
            <ElButton link size="small" :icon="Refresh" @click="mock.loadLogs()">刷新</ElButton>
            <ElButton
              link
              size="small"
              :icon="Delete"
              :disabled="!mock.logs.length"
              @click="clearLogs"
            >
              清空
            </ElButton>
          </div>

          <div class="log-split">
            <div class="log-list">
              <div v-if="!mock.logs.length" class="log-empty">
                <p>还没有请求进来。</p>
                <p class="dim">
                  {{
                    mock.running
                      ? `把请求打到 ${mock.primaryAddress || '监听地址'} 就会出现在这里`
                      : '先启动引擎'
                  }}
                </p>
              </div>
              <div
                v-for="(log, index) in mock.logs"
                :key="index"
                class="log-row"
                :class="{ 'is-active': index === activeLogIndex }"
                @click="activeLogIndex = index"
              >
                <span class="log-time">{{ formatClock(log.time) }}</span>
                <span class="log-method">{{ log.method }}</span>
                <span class="log-url" :title="log.url">{{ log.url }}</span>
                <ElTag v-if="log.status" size="small" effect="plain" class="log-status">
                  {{ log.status }}
                </ElTag>
              </div>
            </div>

            <div class="log-detail">
              <template v-if="activeLog">
                <div class="detail-head">
                  <span class="log-method">{{ activeLog.method }}</span>
                  <span class="detail-url">{{ activeLog.url }}</span>
                  <ElButton link size="small" @click="copyHeaders(activeLog)">复制请求头</ElButton>
                </div>
                <div class="detail-block">
                  <div class="detail-title">请求头</div>
                  <div
                    v-for="(value, name) in activeLog.headers ?? {}"
                    :key="name"
                    class="detail-kv"
                  >
                    <span class="detail-name">{{ name }}</span>
                    <span class="detail-value">{{ value }}</span>
                  </div>
                  <div v-if="!Object.keys(activeLog.headers ?? {}).length" class="dim"
                    >没有请求头</div
                  >
                </div>
                <div class="detail-block">
                  <div class="detail-title">
                    请求体
                    <span v-if="activeLog.body" class="dim">
                      {{ formatBytes(new Blob([activeLog.body]).size) }}
                    </span>
                  </div>
                  <pre v-if="activeLog.body" class="detail-body">{{
                    formatBody(activeLog.body)
                  }}</pre>
                  <div v-else class="dim">没有请求体</div>
                </div>
              </template>
              <div v-else class="log-empty">
                <p>选择左侧一条记录查看详情</p>
              </div>
            </div>
          </div>
        </div>
      </template>
    </SplitPane>
  </div>
</template>

<style lang="scss" scoped>
.mock {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

.mock-bar {
  display: flex;
  flex: 0 0 auto;
  gap: 10px;
  align-items: center;
  height: 40px;
  padding: 0 12px;
  border-bottom: 1px solid var(--chrome-border);
}

.addr {
  font-family: var(--font-mono);
  font-size: 13px;
  font-weight: 600;
  color: var(--el-color-primary);
}

.dim {
  font-size: 12px;
  color: var(--text-color-secondary);
}

.spacer {
  flex: 1 1 auto;
}

.mock-error {
  flex: 0 0 auto;
  padding: 6px 12px;
  font-size: 12px;
  color: var(--el-color-danger);
  background-color: var(--el-color-danger-light-9);
}

.mock-body {
  flex: 1 1 auto;
  min-height: 0;
}

.pane {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  height: 100%;
}

.pane-head {
  display: flex;
  flex: 0 0 auto;
  gap: 8px;
  align-items: center;
  height: 34px;
  padding: 0 10px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.pane-title {
  font-size: 13px;
  font-weight: 600;
}

.config-path {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  max-width: 220px;
  overflow: hidden;
  font-size: 12px;
  color: var(--text-color-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dirty {
  padding: 0 6px;
  font-size: 11px;
  line-height: 16px;
  color: var(--el-color-warning);
  background-color: var(--el-color-warning-light-9);
  border-radius: 8px;
}

.json-error {
  flex: 0 0 auto;
  padding: 4px 10px;
  font-size: 12px;
  color: var(--el-color-danger);
  background-color: var(--el-color-danger-light-9);
}

.config-editor {
  flex: 1 1 auto;
  min-height: 0;
  border: none;
  border-radius: 0;

  :deep(.el-textarea__inner) {
    height: 100%;
    min-height: 0;
    padding: 10px 12px;
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.65;
    border: none;
    border-radius: 0;
    box-shadow: none;
  }
}

.log-split {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
}

.log-list {
  flex: 1 1 0;
  min-height: 0;
  overflow-y: auto;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.log-row {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 4px 10px;
  font-family: var(--font-mono);
  font-size: 12px;
  cursor: pointer;
}

.log-row:hover {
  background-color: var(--el-fill-color-light);
}

.log-row.is-active {
  background-color: var(--el-color-primary-light-9);
}

.log-time {
  flex: 0 0 auto;
  color: var(--text-color-secondary);
}

.log-method {
  flex: 0 0 auto;
  font-weight: 700;
  color: var(--el-color-primary);
}

.log-url {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.log-status {
  flex: 0 0 auto;
  height: 17px;
  padding: 0 5px;
  font-size: 10px;
  line-height: 15px;
}

.log-empty {
  padding: 20px 12px;
  font-size: 12px;
  line-height: 1.9;
  color: var(--text-color-secondary);
  text-align: center;
}

.log-detail {
  flex: 1 1 0;
  min-height: 0;
  padding: 10px 12px;
  overflow-y: auto;
}

.detail-head {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 10px;
}

.detail-url {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-family: var(--font-mono);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-block + .detail-block {
  margin-top: 14px;
}

.detail-title {
  display: flex;
  gap: 8px;
  align-items: baseline;
  margin-bottom: 6px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-color-secondary);
}

.detail-kv {
  display: flex;
  gap: 10px;
  padding: 2px 0;
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.6;
}

.detail-name {
  flex: 0 0 170px;
  overflow: hidden;
  color: var(--el-color-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-value {
  flex: 1 1 auto;
  min-width: 0;
  word-break: break-all;
}

.detail-body {
  max-height: 220px;
  margin: 0;
  padding: 8px 10px;
  overflow: auto;
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  background-color: var(--el-fill-color-lighter);
  border-radius: 4px;
}
</style>
