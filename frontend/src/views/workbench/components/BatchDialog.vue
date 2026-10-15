<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { CaretRight, CircleClose } from '@element-plus/icons-vue'
import { requestApi } from '@/api'
import type { Collection, CollectionItem, SendResult } from '@/api/types'
import { formatDuration, methodChipStyle, statusTagType, formatBytes } from '@/utils/format'
import { applyVariables } from '@/utils/variables'
import { useWorkspaceStore } from '@/store/modules/workspace'
import { useMessage } from '@/hooks/web/useMessage'

/**
 * 集合批量运行。
 *
 * 串行而不是并发：调试场景里接口之间往往有依赖（先建单再查单），并发跑出来的
 * 结果顺序是不可解释的；而且串行才能把"跑到哪一条了"直观地展示出来。
 */
const visible = defineModel<boolean>({ required: true })

const props = defineProps<{ collection: Collection | null }>()
const emit = defineEmits<{ open: [item: CollectionItem] }>()

const workspace = useWorkspaceStore()
const { success } = useMessage()

type RowState = 'pending' | 'running' | 'done' | 'failed' | 'skipped'

interface BatchRow {
  item: CollectionItem
  state: RowState
  result: SendResult | null
  error: string
}

const rows = ref<BatchRow[]>([])
const running = ref(false)
const stopped = ref(false)
/** 已跑完的数量，用于进度显示（不含跳过）。 */
const finished = computed(
  () => rows.value.filter((r) => r.state === 'done' || r.state === 'failed').length
)

const summary = computed(() => {
  const ok = rows.value.filter((r) => r.state === 'done').length
  const bad = rows.value.filter((r) => r.state === 'failed').length
  return { ok, bad, total: rows.value.length }
})

watch(visible, (open) => {
  if (!open) return
  rows.value = (props.collection?.items ?? []).map((item) => ({
    item,
    state: 'pending',
    result: null,
    error: ''
  }))
  running.value = false
  stopped.value = false
})

async function start() {
  if (!props.collection || running.value) return
  running.value = true
  stopped.value = false

  for (let i = 0; i < rows.value.length; i += 1) {
    const row = rows.value[i]
    if (stopped.value) {
      row.state = 'skipped'
      continue
    }
    row.state = 'running'
    // 与单个发送保持一致：变量在发送前一刻替换，批量运行才能随环境切换换目标。
    const resolved = applyVariables(row.item.request, workspace.variables)
    const requestId = `batch-${row.item.id}-${i}-${Date.now()}`
    try {
      const result = await requestApi.send(requestId, resolved)
      row.result = result
      row.state = result.error ? 'failed' : 'done'
      row.error = result.error ?? ''
      // 批量运行同样进历史：跑完一批之后想回看某一条的响应，历史是唯一入口。
      workspace
        .addHistory({
          id: `${row.item.id}-${Date.now()}`,
          at: new Date().toISOString(),
          request: JSON.parse(JSON.stringify(row.item.request)),
          status: result.status,
          durationMs: result.durationMs,
          size: result.size,
          error: result.error ?? ''
        })
        .catch((err) => console.error('[batch] 写入历史失败', err))
    } catch (err) {
      row.state = 'failed'
      row.error = err instanceof Error ? err.message : String(err)
    }
  }

  running.value = false
  const { ok, bad } = summary.value
  success(`批量运行结束：成功 ${ok} 条${bad ? `，失败 ${bad} 条` : ''}`)
}

function stop() {
  stopped.value = true
}

async function retryFailed() {
  for (const row of rows.value) {
    if (row.state === 'failed' || row.state === 'skipped') {
      row.state = 'pending'
      row.result = null
      row.error = ''
    }
  }
  await start()
}
</script>

<template>
  <ElDialog v-model="visible" title="批量运行集合" width="760px" append-to-body>
    <div class="batch">
      <div class="batch-head">
        <span class="batch-name">{{ props.collection?.name }}</span>
        <span class="batch-count">
          共 {{ rows.length }} 条<template v-if="finished"> · 已完成 {{ finished }}</template>
        </span>
        <div class="spacer" />
        <ElTag
          v-if="rows.length && finished === rows.length && !running"
          size="small"
          type="success"
        >
          成功 {{ summary.ok }} / 失败 {{ summary.bad }}
        </ElTag>
      </div>

      <div v-if="!rows.length" class="batch-empty">这个集合里还没有接口。</div>

      <div v-else class="batch-list">
        <div
          v-for="(row, index) in rows"
          :key="row.item.id + index"
          class="batch-row"
          :class="`is-${row.state}`"
          @click="emit('open', row.item)"
        >
          <span class="batch-index">{{ index + 1 }}</span>
          <span class="batch-method" :style="methodChipStyle(row.item.request.method)">
            {{ (row.item.request.method || 'GET').toUpperCase() }}
          </span>
          <span class="batch-title" :title="row.item.request.url">{{ row.item.name }}</span>

          <span class="batch-result">
            <template v-if="row.state === 'pending'">
              <span class="dim">待运行</span>
            </template>
            <template v-else-if="row.state === 'running'">
              <span class="dim is-blue">运行中…</span>
            </template>
            <template v-else-if="row.state === 'skipped'">
              <span class="dim">已跳过</span>
            </template>
            <template v-else-if="row.state === 'failed'">
              <span class="dim is-red">{{ row.error || '请求失败' }}</span>
            </template>
            <template v-else>
              <ElTag size="small" :type="statusTagType(row.result!.status)" effect="plain">
                {{ row.result!.status }}
              </ElTag>
              <span class="dim">{{ formatDuration(row.result!.durationMs) }}</span>
              <span class="dim">{{ formatBytes(row.result!.size) }}</span>
            </template>
          </span>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="footer">
        <ElButton v-if="running" link type="danger" :icon="CircleClose" @click="stop">
          停止（跑完当前这条）
        </ElButton>
        <ElButton v-else-if="summary.bad > 0" link @click="retryFailed"> 只重试失败的 </ElButton>
        <div class="footer-right">
          <ElButton @click="visible = false">关闭</ElButton>
          <ElButton
            type="primary"
            :icon="CaretRight"
            :loading="running"
            :disabled="!rows.length || running"
            @click="start"
          >
            {{ finished ? '重新运行全部' : '开始运行' }}
          </ElButton>
        </div>
      </div>
    </template>
  </ElDialog>
</template>

<style lang="scss" scoped>
.batch {
  display: flex;
  flex-direction: column;
  height: 420px;
}

.batch-head {
  display: flex;
  flex: 0 0 auto;
  gap: 10px;
  align-items: center;
  padding-bottom: 8px;
  font-size: 13px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.batch-name {
  font-weight: 600;
}

.batch-count {
  font-size: 12px;
  color: var(--text-color-secondary);
}

.spacer {
  flex: 1 1 auto;
}

.batch-empty {
  padding: 24px;
  font-size: 12px;
  color: var(--text-color-secondary);
  text-align: center;
}

.batch-list {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
}

.batch-row {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 6px 4px;
  font-size: 12px;
  cursor: pointer;
  border-bottom: 1px solid var(--el-border-color-extra-light);
}

.batch-row:hover {
  background-color: var(--el-fill-color-light);
}

.batch-row.is-running {
  background-color: var(--el-color-primary-light-9);
}

.batch-index {
  flex: 0 0 22px;
  color: var(--text-color-secondary);
  text-align: right;
}

.batch-method {
  flex: 0 0 auto;
  padding: 0 4px;
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 700;
  line-height: 15px;
  border-radius: 3px;
}

.batch-title {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.batch-result {
  display: flex;
  flex: 0 0 auto;
  gap: 8px;
  align-items: center;
}

.dim {
  color: var(--text-color-secondary);
}

.dim.is-blue {
  color: var(--el-color-primary);
}

.dim.is-red {
  max-width: 280px;
  overflow: hidden;
  color: var(--el-color-danger);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.footer {
  display: flex;
  align-items: center;
}

.footer-right {
  display: flex;
  gap: 8px;
  margin-left: auto;
}
</style>
