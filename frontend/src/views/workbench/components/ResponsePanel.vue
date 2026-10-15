<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { CopyDocument, Download, MagicStick, WarningFilled } from '@element-plus/icons-vue'
import { requestApi } from '@/api'
import type { RequestSpec, SendResult } from '@/api/types'
import {
  base64ToDataUrl,
  formatBytes,
  formatDuration,
  headerValue,
  isJSON,
  prettyJSON,
  statusTagType,
  statusText
} from '@/utils/format'
import { useMessage } from '@/hooks/web/useMessage'

/**
 * 响应面板。
 *
 * 空态、错误态、正常态三者在同一个位置切换而不是各占一个区域：
 * 响应区的高度是固定的（用户拖过分隔条），内容换形态时若位置也变，
 * 视线每次都要重新找落点。
 */
const props = defineProps<{
  response: SendResult | null
  sending: boolean
  /** 最近一次发送使用的请求 ID，供"另存为"从后端取完整响应体。 */
  requestId: string
  request: RequestSpec
}>()

const { success, error: toastError } = useMessage()

const activeTab = ref<'body' | 'headers' | 'cookies'>('body')
/** 是否美化显示。JSON 默认美化——接口调试里 95% 的响应是 JSON。 */
const pretty = ref(true)
const saving = ref(false)

/** 换了一个响应就回到默认视图：上一条调成"Headers"不该影响新的一条。 */
watch(
  () => props.response?.id,
  () => {
    activeTab.value = 'body'
    pretty.value = true
  }
)

const hasResponse = computed(() => Boolean(props.response))
const failed = computed(() => Boolean(props.response?.error))

const contentType = computed(() => headerValue(props.response?.headers, 'content-type'))

const isImage = computed(
  () => props.response?.isText === false && /^image\//i.test(contentType.value)
)

const imageSrc = computed(() =>
  isImage.value && props.response?.base64
    ? base64ToDataUrl(props.response.base64, contentType.value)
    : ''
)

/** 文本的最终展示内容。二进制响应没有 text，这里只服务文本分支。 */
const displayBody = computed(() => {
  const text = props.response?.text ?? ''
  if (!pretty.value) return text
  return prettyJSON(text)
})

const canFormat = computed(() => isJSON(props.response?.text ?? ''))

const headerCount = computed(() => props.response?.headers?.length ?? 0)
const cookieCount = computed(() => props.response?.cookies?.length ?? 0)

/** 最终 URL 与请求 URL 不同说明发生了重定向，值得在概览里点出来。 */
const redirected = computed(
  () => Boolean(props.response?.finalUrl) && props.response.finalUrl !== (props.request.url ?? '')
)

async function copyBody() {
  const text = props.response?.text ?? ''
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    success('响应体已复制')
  } catch (err) {
    console.error('[response] 复制失败', err)
    toastError('复制失败，剪贴板不可用')
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    success('已复制')
  } catch (err) {
    console.error('[response] 复制失败', err)
    toastError('复制失败，剪贴板不可用')
  }
}

/** 另存为完整响应体：预览可能被截断，真正落盘的是后端缓存的完整内容。 */
async function saveBody() {
  const res = props.response
  if (!res) return
  saving.value = true
  try {
    const path = await requestApi.saveBody(props.requestId, res.suggestedFileName || 'response')
    if (path) {
      success(`已保存到 ${path}`)
    }
  } catch (err) {
    console.error('[response] 保存失败', err)
    toastError(err instanceof Error ? err.message : String(err))
  } finally {
    saving.value = false
  }
}

function headersToText(): string {
  return (props.response?.headers ?? []).map((h) => `${h.name}: ${h.value}`).join('\n')
}

function cookiesToText(): string {
  return (props.response?.cookies ?? []).map((c) => `${c.name}=${c.value}`).join('\n')
}
</script>

<template>
  <div class="response-panel">
    <!-- 概览条 -->
    <div class="resp-bar">
      <template v-if="hasResponse">
        <ElTag
          v-if="!failed"
          size="small"
          effect="dark"
          :type="statusTagType(props.response!.status)"
          class="status-tag"
        >
          {{ props.response!.status }}
          {{ props.response!.statusText || statusText(props.response!.status) }}
        </ElTag>
        <ElTag v-else size="small" effect="dark" type="danger" class="status-tag">请求失败</ElTag>

        <span class="metric">{{ formatDuration(props.response!.durationMs) }}</span>
        <span class="metric">{{ formatBytes(props.response!.size) }}</span>
        <ElTooltip
          v-if="props.response!.attempts > 1"
          :content="`共发出 ${props.response!.attempts} 次（含重试）`"
        >
          <span class="metric is-warn">重试 {{ props.response!.attempts - 1 }} 次</span>
        </ElTooltip>
        <ElTooltip
          v-if="props.response!.redirects > 0"
          :content="`最终地址：${props.response!.finalUrl}`"
        >
          <span class="metric">重定向 {{ props.response!.redirects }} 次</span>
        </ElTooltip>
        <span v-if="props.response!.proto" class="metric is-dim">{{ props.response!.proto }}</span>
      </template>
      <span v-else class="metric is-dim">
        {{ sending ? '请求已发出，等待响应…' : '尚未发送请求' }}
      </span>

      <div class="spacer" />

      <template v-if="hasResponse && !failed">
        <ElButton
          v-if="canFormat && activeTab === 'body'"
          link
          size="small"
          :icon="MagicStick"
          @click="pretty = !pretty"
        >
          {{ pretty ? '原始' : '美化' }}
        </ElButton>
        <ElButton link size="small" :icon="CopyDocument" @click="copyBody">复制</ElButton>
        <ElButton link size="small" :icon="Download" :loading="saving" @click="saveBody">
          另存为
        </ElButton>
      </template>
    </div>

    <!-- 空态 -->
    <div v-if="!hasResponse" class="placeholder">
      <div class="placeholder-title">{{ sending ? '发送中…' : '还没有响应' }}</div>
      <div class="placeholder-sub"> 填写请求地址后点「发送」，或按 Ctrl + Enter。 </div>
    </div>

    <!-- 错误态 -->
    <div v-else-if="failed" class="failure">
      <ElIcon class="failure-icon"><WarningFilled /></ElIcon>
      <div class="failure-title">请求未能完成</div>
      <div class="failure-msg">{{ props.response!.error }}</div>
      <div class="failure-url">{{ props.request.url }}</div>
    </div>

    <!-- 正常态 -->
    <template v-else>
      <ElTabs v-model="activeTab" class="resp-tabs">
        <ElTabPane name="body">
          <template #label>
            <span>响应体</span>
          </template>

          <div class="tab-body">
            <img v-if="isImage" class="image-preview" :src="imageSrc" alt="响应图片" />

            <pre v-else-if="props.response!.isText" class="code">{{
              displayBody || '（空响应体）'
            }}</pre>

            <div v-else class="binary">
              <div class="binary-title">二进制响应</div>
              <div class="binary-meta">
                类型：{{ contentType || '未知' }} · 大小：{{ formatBytes(props.response!.size) }}
              </div>
              <ElButton size="small" :icon="Download" :loading="saving" @click="saveBody">
                另存为文件
              </ElButton>
            </div>

            <div v-if="props.response!.previewTruncated" class="truncated-tip">
              内容较大，这里只预览了前一部分；「另存为」拿到的是完整响应体。
            </div>
            <div v-if="redirected" class="truncated-tip">
              实际请求地址已被重定向为：{{ props.response!.finalUrl }}
            </div>
          </div>
        </ElTabPane>

        <ElTabPane name="headers">
          <template #label>
            <span
              >响应头<span v-if="headerCount" class="tab-count">{{ headerCount }}</span></span
            >
          </template>

          <div class="tab-body">
            <div v-if="activeTab === 'headers'" class="kv-toolbar">
              <ElButton link size="small" :icon="CopyDocument" @click="copyText(headersToText())">
                复制全部
              </ElButton>
            </div>
            <div v-if="!headerCount" class="empty-hint">没有响应头</div>
            <div v-for="h in props.response!.headers" :key="h.name" class="resp-kv">
              <span class="resp-kv-name">{{ h.name }}</span>
              <span class="resp-kv-value">{{ h.value }}</span>
            </div>
          </div>
        </ElTabPane>

        <ElTabPane name="cookies">
          <template #label>
            <span
              >Cookie<span v-if="cookieCount" class="tab-count">{{ cookieCount }}</span></span
            >
          </template>

          <div class="tab-body">
            <div v-if="activeTab === 'cookies'" class="kv-toolbar">
              <ElButton link size="small" :icon="CopyDocument" @click="copyText(cookiesToText())">
                复制全部
              </ElButton>
            </div>
            <div v-if="!cookieCount" class="empty-hint">响应里没有 Set-Cookie</div>
            <div v-for="c in props.response!.cookies" :key="c.name" class="resp-kv">
              <span class="resp-kv-name">{{ c.name }}</span>
              <span class="resp-kv-value">{{ c.value }}</span>
            </div>
          </div>
        </ElTabPane>
      </ElTabs>
    </template>
  </div>
</template>

<style lang="scss" scoped>
.response-panel {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
  height: 100%;
  overflow: hidden;
}

.resp-bar {
  display: flex;
  flex: 0 0 auto;
  gap: 10px;
  align-items: center;
  height: 34px;
  padding: 0 12px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.status-tag {
  font-family: var(--font-mono);
}

.metric {
  font-size: 12px;
  color: var(--text-color-secondary);
  white-space: nowrap;
}

.metric.is-dim {
  opacity: 0.8;
}

.metric.is-warn {
  color: var(--el-color-warning);
}

.spacer {
  flex: 1 1 auto;
}

.placeholder,
.failure {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 6px;
  align-items: center;
  justify-content: center;
  padding: 20px;
  text-align: center;
}

.placeholder-title {
  font-size: 14px;
  color: var(--text-color);
}

.placeholder-sub {
  font-size: 12px;
  color: var(--text-color-secondary);
}

.failure-icon {
  font-size: 26px;
  color: var(--el-color-danger);
}

.failure-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--el-color-danger);
}

.failure-msg {
  max-width: 620px;
  font-size: 13px;
  line-height: 1.7;
  color: var(--text-color);
  word-break: break-all;
}

.failure-url {
  max-width: 620px;
  overflow: hidden;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-color-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 标签页头部压扁：默认高度在桌面程序里显得很占地方，
   而响应区本身高度有限，多出来的每一像素都是内容少一行。 */
.resp-tabs {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;

  :deep(.el-tabs__header) {
    flex: 0 0 auto;
    margin: 0;
    padding: 0 12px;
  }

  :deep(.el-tabs__nav-wrap::after) {
    height: 1px;
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

.tab-body {
  height: 100%;
  padding: 8px 12px 12px;
  overflow: auto;
}

.kv-toolbar {
  margin-bottom: 4px;
}

.code {
  margin: 0;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.65;
  color: var(--text-color);
  white-space: pre-wrap;
  word-break: break-word;
}

.image-preview {
  display: block;
  max-width: 100%;
  max-height: 100%;
  margin: 0 auto;
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 4px;
}

.binary {
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: flex-start;
  padding: 20px 0;
}

.binary-title {
  font-size: 14px;
  color: var(--text-color);
}

.binary-meta {
  font-size: 12px;
  color: var(--text-color-secondary);
}

.truncated-tip {
  margin-top: 10px;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--el-color-warning);
  background-color: var(--el-color-warning-light-9);
  border-radius: 4px;
}

.resp-kv {
  display: flex;
  gap: 12px;
  padding: 4px 0;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.6;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}

.resp-kv-name {
  flex: 0 0 240px;
  color: var(--el-color-primary);
  word-break: break-all;
}

.resp-kv-value {
  flex: 1 1 auto;
  min-width: 0;
  color: var(--text-color);
  word-break: break-all;
}

.empty-hint {
  padding: 16px 0;
  font-size: 12px;
  color: var(--text-color-secondary);
}
</style>
