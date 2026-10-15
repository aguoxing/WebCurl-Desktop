<script lang="ts" setup>
import { computed } from 'vue'
import { ArrowDown, Clock, Document, Download, Loading } from '@element-plus/icons-vue'
import type { RequestSpec } from '@/api/types'
import { HTTP_METHODS, methodChipStyle } from '@/utils/format'
import { collectRefs } from '@/utils/variables'
import { useWorkspaceStore } from '@/store/modules/workspace'

/**
 * 请求行：方法 + 地址 + 发送。
 *
 * 这是整个界面里唯一"永远可见"的控件组，因此把最高频的三个动作（切方法、
 * 改地址、发出去）放在同一行，其余动作（保存、curl）收进次要按钮——
 * 调试时手是放在鼠标上连着点发送的，任何一次误点都要能立刻撤销。
 */
const props = defineProps<{
  request: RequestSpec
  sending: boolean
  /** 已保存到集合时显示的名字，为空说明尚未保存。 */
  savedTitle?: string
}>()

const emit = defineEmits<{
  update: [Partial<RequestSpec>]
  send: []
  cancel: []
  save: []
  curl: []
  history: []
}>()

const workspace = useWorkspaceStore()

const method = computed(() => (props.request.method || 'GET').toUpperCase())
const chipStyle = computed(() => methodChipStyle(method.value))
const url = computed(() => props.request.url ?? '')

/**
 * 引用了但当前环境没有定义的变量。
 *
 * 界面不会阻止发送：有些接口地址本来就依赖服务端临时注入的变量，真发一次
 * 拿到报错比在这里被拦住更容易排查。这里只做提示。
 */
const undefinedVars = computed(() =>
  collectRefs(props.request).filter((name) => !workspace.definedVarNames.has(name))
)

const hasVars = computed(() => collectRefs(props.request).length > 0)

function setMethod(value: string) {
  emit('update', { method: value })
}

function setUrl(value: string) {
  emit('update', { url: value })
}

/** Ctrl/Cmd + Enter 发送，与多数调试工具一致。 */
function onUrlKeydown(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') {
    event.preventDefault()
    if (!props.sending) emit('send')
  }
}
</script>

<template>
  <div class="request-line">
    <ElDropdown trigger="click" placement="bottom-start" @command="setMethod">
      <button class="method-btn" type="button" :style="chipStyle">
        <span class="method-text">{{ method }}</span>
        <ElIcon class="method-caret"><ArrowDown /></ElIcon>
      </button>
      <template #dropdown>
        <ElDropdownMenu>
          <ElDropdownItem v-for="m in HTTP_METHODS" :key="m" :command="m">
            <span class="method-option" :style="{ color: methodChipStyle(m).color }">{{ m }}</span>
          </ElDropdownItem>
        </ElDropdownMenu>
      </template>
    </ElDropdown>

    <ElInput
      class="url-input"
      :model-value="url"
      placeholder="请求地址，可含 {{变量}}；不带协议时默认 https"
      spellcheck="false"
      @update:model-value="setUrl"
      @keydown="onUrlKeydown"
    >
      <template #prefix>
        <ElTooltip
          v-if="hasVars"
          :content="
            undefinedVars.length
              ? `以下变量在当前环境未定义，发送时会原样保留：${undefinedVars.join('、')}`
              : '地址中的变量均已在当前环境定义'
          "
        >
          <ElTag
            size="small"
            :type="undefinedVars.length ? 'warning' : 'success'"
            effect="plain"
            class="var-tag"
          >
            {{ undefinedVars.length ? `${undefinedVars.length} 个变量未定义` : '变量已解析' }}
          </ElTag>
        </ElTooltip>
      </template>
    </ElInput>

    <ElButton v-if="!sending" class="send-btn" type="primary" :icon="Clock" @click="emit('send')">
      发送
    </ElButton>
    <ElButton v-else class="send-btn" type="danger" :icon="Loading" @click="emit('cancel')">
      取消
    </ElButton>

    <ElTooltip content="保存到集合（Ctrl+S）" placement="bottom">
      <ElButton class="icon-btn" :icon="Document" @click="emit('save')" />
    </ElTooltip>
    <ElTooltip content="curl 导入 / 导出" placement="bottom">
      <ElButton class="icon-btn" :icon="Download" @click="emit('curl')" />
    </ElTooltip>
    <ElTooltip content="从历史中挑一条填充（Ctrl+H）" placement="bottom">
      <ElButton class="icon-btn" :icon="Clock" @click="emit('history')" />
    </ElTooltip>
  </div>
</template>

<style lang="scss" scoped>
.request-line {
  display: flex;
  flex: 0 0 auto;
  gap: 6px;
  align-items: center;
  padding: 10px 12px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.method-btn {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 4px;
  align-items: center;
  justify-content: space-between;
  width: 108px;
  height: 32px;
  padding: 0 8px;
  font-family: var(--font-mono);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.4px;
  cursor: pointer;
  border: 1px solid transparent;
  border-radius: 6px;
}

.method-btn:hover {
  border-color: currentcolor;
}

.method-text {
  flex: 1 1 auto;
  text-align: center;
}

.method-caret {
  flex: 0 0 auto;
  opacity: 0.7;
}

.method-option {
  font-family: var(--font-mono);
  font-weight: 600;
}

.url-input {
  flex: 1 1 auto;
  min-width: 0;

  :deep(.el-input__inner) {
    font-family: var(--font-mono);
    font-size: 13px;
  }
}

.var-tag {
  flex: 0 0 auto;
}

.send-btn {
  flex: 0 0 auto;
  width: 88px;
}

.icon-btn {
  flex: 0 0 auto;
}
</style>
