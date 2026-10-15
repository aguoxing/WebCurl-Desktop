<script lang="ts" setup>
import { ref } from 'vue'
import { Close, Plus } from '@element-plus/icons-vue'
import type { RequestTab } from '@/api/types'
import { methodChipStyle } from '@/utils/format'

/**
 * 多标签栏。
 *
 * 标签上同时显示方法与标题：调试时常常开着同一路径的 GET 与 POST 两个变体，
 * 只看路径根本分不出哪个是哪个。
 */
defineProps<{
  tabs: RequestTab[]
  activeId: string
}>()

const emit = defineEmits<{
  select: [string]
  close: [string]
  closeOthers: [string]
  closeAll: []
  create: []
}>()

/** 右键菜单的受控状态：记录在哪个标签上右键、以及菜单出现在哪里。 */
const contextMenu = ref<{ visible: boolean; tabId: string; x: number; y: number }>({
  visible: false,
  tabId: '',
  x: 0,
  y: 0
})

function openContextMenu(event: MouseEvent, tabId: string) {
  event.preventDefault()
  contextMenu.value = { visible: true, tabId, x: event.clientX, y: event.clientY }
}

function runCommand(command: string) {
  const { tabId } = contextMenu.value
  contextMenu.value.visible = false
  if (command === 'others') emit('closeOthers', tabId)
  if (command === 'all') emit('closeAll')
}

/** 中键关闭，与浏览器的标签页一致。 */
function onMouseUp(event: MouseEvent, id: string) {
  if (event.button === 1) {
    event.preventDefault()
    emit('close', id)
  }
}
</script>

<template>
  <div class="tab-bar">
    <div class="tab-list">
      <div
        v-for="tab in tabs"
        :key="tab.id"
        class="tab"
        :class="{ 'is-active': tab.id === activeId }"
        :title="tab.request.url || tab.title"
        @click="emit('select', tab.id)"
        @mouseup="onMouseUp($event, tab.id)"
        @contextmenu="openContextMenu($event, tab.id)"
      >
        <span class="tab-method" :style="methodChipStyle(tab.request.method)">
          {{ (tab.request.method || 'GET').toUpperCase() }}
        </span>
        <span class="tab-title">{{ tab.title }}</span>
        <span v-if="tab.sending" class="tab-sending" title="发送中">
          <span class="dot" />
        </span>
        <ElIcon class="tab-close" @click.stop="emit('close', tab.id)"><Close /></ElIcon>
      </div>
    </div>

    <button class="tab-add" type="button" title="新建请求（Ctrl+T）" @click="emit('create')">
      <ElIcon><Plus /></ElIcon>
    </button>

    <Teleport to="body">
      <div
        v-if="contextMenu.visible"
        class="tab-context"
        :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }"
        @click="contextMenu.visible = false"
      >
        <div class="tab-context-item" @click="runCommand('others')">关闭其他标签</div>
        <div
          class="tab-context-item"
          :class="{ 'is-disabled': tabs.length <= 1 }"
          @click="tabs.length > 1 && runCommand('all')"
        >
          关闭全部标签
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style lang="scss" scoped>
.tab-bar {
  display: flex;
  flex: 0 0 auto;
  align-items: stretch;
  height: 34px;
  background-color: var(--chrome-bgc);
  border-bottom: 1px solid var(--chrome-border);
}

.tab-list {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;

  &::-webkit-scrollbar {
    height: 3px;
  }
}

.tab {
  display: flex;
  flex: 0 0 auto;
  gap: 6px;
  align-items: center;
  max-width: 220px;
  padding: 0 6px 0 10px;
  font-size: 12px;
  color: var(--text-color-secondary);
  cursor: pointer;
  border-right: 1px solid var(--chrome-border);
  border-bottom: 2px solid transparent;
  transition:
    background-color 0.12s ease,
    color 0.12s ease;
}

.tab:hover {
  background-color: var(--el-fill-color-light);
}

/* 激活态用一条下边框而不是整块底色：标签多的时候整块底色连成一片，
   看不出边界在哪。 */
.tab.is-active {
  color: var(--text-color);
  background-color: var(--app-bgc);
  border-bottom-color: var(--el-color-primary);
}

.tab-method {
  flex: 0 0 auto;
  padding: 0 4px;
  font-family: var(--font-mono);
  font-size: 10px;
  font-weight: 700;
  line-height: 15px;
  border-radius: 3px;
}

.tab-title {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tab-sending {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
}

.tab-sending .dot {
  width: 6px;
  height: 6px;
  background-color: var(--el-color-primary);
  border-radius: 50%;
  animation: tab-pulse 1s ease-in-out infinite;
}

@keyframes tab-pulse {
  0%,
  100% {
    opacity: 0.25;
  }

  50% {
    opacity: 1;
  }
}

.tab-close {
  flex: 0 0 auto;
  width: 14px;
  height: 14px;
  border-radius: 3px;
  opacity: 0;
  transition: opacity 0.12s ease;
}

.tab:hover .tab-close,
.tab.is-active .tab-close {
  opacity: 0.75;
}

.tab-close:hover {
  color: var(--el-color-danger);
  background-color: var(--el-fill-color);
}

.tab-add {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 32px;
  color: var(--text-color-secondary);
  cursor: pointer;
  background: transparent;
  border: none;
  border-left: 1px solid var(--chrome-border);
}

.tab-add:hover {
  color: var(--el-color-primary);
  background-color: var(--el-fill-color-light);
}

.tab-context {
  position: fixed;
  z-index: 3000;
  padding: 4px;
  font-size: 13px;
  background-color: var(--el-bg-color-overlay);
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
  box-shadow: var(--el-box-shadow-light);
}

.tab-context-item {
  padding: 5px 14px;
  cursor: pointer;
  border-radius: 4px;
}

.tab-context-item:hover {
  color: var(--el-color-primary);
  background-color: var(--el-fill-color-light);
}

.tab-context-item.is-disabled {
  color: var(--el-text-color-disabled);
  cursor: not-allowed;
}
</style>
