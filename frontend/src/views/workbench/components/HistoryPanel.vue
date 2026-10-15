<script lang="ts" setup>
import { computed, ref } from 'vue'
import { Delete, Refresh, Search } from '@element-plus/icons-vue'
import type { HistoryEntry, RequestSpec } from '@/api/types'
import { formatClock, methodChipStyle, statusTagType, shortUrl } from '@/utils/format'
import { useWorkspaceStore } from '@/store/modules/workspace'
import { useMessage } from '@/hooks/web/useMessage'

/**
 * 历史面板。
 *
 * 只记录"发出去过"的请求，且每条都带状态码与耗时——历史的价值在于
 * "刚才那一下到底成功没有"，一份只有 URL 的列表回答不了这个问题。
 */
const emit = defineEmits<{ open: [HistoryEntry] }>()

const workspace = useWorkspaceStore()
const { confirm } = useMessage()

const keyword = ref('')

const entries = computed<HistoryEntry[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return workspace.history
  return workspace.history.filter((e) => {
    const req: RequestSpec = e.request
    return (
      req.url.toLowerCase().includes(kw) ||
      req.method.toLowerCase().includes(kw) ||
      String(e.status).includes(kw)
    )
  })
})

/** 按"今天 / 更早"分组：历史长了之后，日期是最主要的定位线索。 */
const grouped = computed(() => {
  const today = new Date().toDateString()
  const groups: { label: string; items: HistoryEntry[] }[] = []
  for (const entry of entries.value) {
    const label = new Date(entry.at).toDateString() === today ? '今天' : '更早'
    const last = groups[groups.length - 1]
    if (last && last.label === label) last.items.push(entry)
    else groups.push({ label, items: [entry] })
  }
  return groups
})

async function clearAll() {
  try {
    await confirm(`确定清空全部 ${workspace.history.length} 条历史？集合与环境不受影响。`)
  } catch {
    return
  }
  await workspace.clearHistory()
}

async function remove(entry: HistoryEntry, event: Event) {
  event.stopPropagation()
  await workspace.deleteHistory(entry.id)
}

async function refresh() {
  await workspace.refresh()
}
</script>

<template>
  <div class="side-panel">
    <div class="side-toolbar">
      <ElInput
        v-model="keyword"
        size="small"
        placeholder="搜索地址、方法或状态码"
        clearable
        :prefix-icon="Search"
      />
      <ElTooltip content="刷新" placement="bottom">
        <ElButton size="small" :icon="Refresh" @click="refresh" />
      </ElTooltip>
      <ElTooltip content="清空历史" placement="bottom">
        <ElButton
          size="small"
          :icon="Delete"
          :disabled="!workspace.history.length"
          @click="clearAll"
        />
      </ElTooltip>
    </div>

    <ElScrollbar class="side-scroll">
      <div v-if="!entries.length" class="side-empty">
        <p v-if="keyword">没有匹配的历史</p>
        <template v-else>
          <p>还没有发送过请求。</p>
          <p class="side-empty-sub">每次发送都会在这里留一条记录。</p>
        </template>
      </div>

      <template v-for="group in grouped" :key="group.label">
        <div class="group-label">{{ group.label }}</div>
        <div
          v-for="entry in group.items"
          :key="entry.id"
          class="history-row"
          :title="entry.request.url"
          @click="emit('open', entry)"
        >
          <span class="item-method" :style="methodChipStyle(entry.request.method)">
            {{ (entry.request.method || 'GET').toUpperCase() }}
          </span>
          <span class="history-url">{{ shortUrl(entry.request.url) }}</span>
          <span class="history-meta">
            <ElTag
              v-if="entry.error"
              size="small"
              type="danger"
              effect="plain"
              class="history-status"
            >
              失败
            </ElTag>
            <ElTag
              v-else
              size="small"
              :type="statusTagType(entry.status)"
              effect="plain"
              class="history-status"
            >
              {{ entry.status }}
            </ElTag>
            <span class="history-time">{{ formatClock(entry.at) }}</span>
          </span>
          <ElIcon
            class="row-action is-danger history-remove"
            title="删除这条"
            @click="remove(entry, $event)"
          >
            <Delete />
          </ElIcon>
        </div>
      </template>
    </ElScrollbar>

    <div class="side-foot">
      <span class="history-count">共 {{ workspace.history.length }} 条</span>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use './side-panel.scss';

.group-label {
  padding: 6px 8px 2px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-color-secondary);
}

.history-row {
  display: flex;
  gap: 6px;
  align-items: center;
  padding: 5px 8px;
  font-size: 12px;
  cursor: pointer;
  transition: background-color 0.12s ease;
}

.history-row:hover {
  background-color: var(--el-fill-color-light);
}

.history-url {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.history-meta {
  display: flex;
  flex: 0 0 auto;
  gap: 6px;
  align-items: center;
}

.history-status {
  height: 17px;
  padding: 0 5px;
  font-family: var(--font-mono);
  font-size: 10px;
  line-height: 15px;
}

.history-time {
  color: var(--text-color-secondary);
}

.history-remove {
  flex: 0 0 auto;
  font-size: 13px;
}

.row-action.is-danger:hover {
  color: var(--el-color-danger);
}

.history-count {
  font-size: 11px;
  color: var(--text-color-secondary);
}
</style>
