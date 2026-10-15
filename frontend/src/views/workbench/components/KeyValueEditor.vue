<script lang="ts" setup>
import { computed } from 'vue'
import { Delete, Plus } from '@element-plus/icons-vue'
import type { KV } from '@/api/types'

/**
 * 通用的键值对编辑器，Params / Headers / Cookies / 表单字段共用。
 *
 * 为什么用无边框输入框而不是表格组件：这些面板在一屏里要能放下十几行，
 * Element Plus 的 el-table 每行都有内边距与边框，六行之后就要滚动了。
 * 这里每行就是两个裸输入框 + 一个开关，视觉密度接近专业调试工具的水准。
 */
const props = withDefaults(
  defineProps<{
    /** 键值对列表。 */
    modelValue: KV[]
    namePlaceholder?: string
    valuePlaceholder?: string
    /** 是否显示每行的启用开关。 */
    showEnable?: boolean
    /** 单行最小值宽度，见模板里的用途。 */
    minWidth?: string
  }>(),
  {
    namePlaceholder: '键',
    valuePlaceholder: '值',
    showEnable: true,
    minWidth: '0'
  }
)

const emit = defineEmits<{ 'update:modelValue': [KV[]] }>()

const rows = computed(() => props.modelValue ?? [])

/** 空行提示：最后一行总是空的，方便直接开始输入。 */
const showTrailingRow = computed(() => {
  if (rows.value.length === 0) return true
  const last = rows.value[rows.value.length - 1]
  return !last.name && !last.value
})

function update(patch: KV[]) {
  emit('update:modelValue', patch)
}

function updateCell(index: number, field: 'name' | 'value', value: string) {
  const next = rows.value.map((row, i) => (i === index ? { ...row, [field]: value } : row))
  update(next)
}

function toggle(index: number, enabled: boolean) {
  const next = rows.value.map((row, i) => (i === index ? { ...row, enabled } : row))
  update(next)
}

function addRow() {
  update([...rows.value, { name: '', value: '', enabled: true }])
}

function removeRow(index: number) {
  update(rows.value.filter((_, i) => i !== index))
}

/** 把 "name: value" 这样的多行文本批量导入为键值对。 */
function importFromText(text: string) {
  const added: KV[] = []
  for (const line of text.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const idx = trimmed.indexOf(':')
    if (idx > 0) {
      added.push({
        name: trimmed.slice(0, idx).trim(),
        value: trimmed.slice(idx + 1).trim(),
        enabled: true
      })
    } else {
      const eq = trimmed.indexOf('=')
      if (eq > 0) {
        added.push({
          name: trimmed.slice(0, eq).trim(),
          value: trimmed.slice(eq + 1).trim(),
          enabled: true
        })
      }
    }
  }
  if (added.length && !showTrailingRow.value) {
    update([...rows.value, ...added])
  } else if (added.length) {
    // 末尾是空行时替换掉它，避免留下一个孤立的空行。
    update([...rows.value.slice(0, -1), ...added])
  }
  return added.length
}

defineExpose({ importFromText })
</script>

<template>
  <div class="kv-editor">
    <div v-for="(row, index) in rows" :key="index" class="kv-row">
      <ElCheckbox
        v-if="showEnable"
        class="kv-enable"
        :model-value="row.enabled"
        @update:model-value="(v: any) => toggle(index, Boolean(v))"
      />
      <ElInput
        class="kv-input"
        :model-value="row.name"
        :placeholder="namePlaceholder"
        @update:model-value="(v: string) => updateCell(index, 'name', v)"
      />
      <ElInput
        class="kv-input"
        :model-value="row.value"
        :placeholder="valuePlaceholder"
        @update:model-value="(v: string) => updateCell(index, 'value', v)"
      />
      <ElButton
        class="kv-remove"
        link
        :icon="Delete"
        :disabled="showTrailingRow && index === rows.length - 1"
        @click="removeRow(index)"
      />
    </div>

    <div class="kv-actions">
      <ElButton link type="primary" :icon="Plus" @click="addRow">添加一行</ElButton>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.kv-editor {
  display: flex;
  flex-direction: column;
}

.kv-row {
  display: flex;
  gap: 6px;
  align-items: center;
  padding: 2px 0;
}

.kv-enable {
  flex: 0 0 auto;
  margin-right: 2px;
}

.kv-input {
  flex: 1 1 0;
  min-width: 0;

  :deep(.el-input__wrapper) {
    box-shadow: none;
    background-color: transparent;
    border: 1px solid var(--el-border-color-lighter);
    border-radius: 4px;
  }

  :deep(.el-input__wrapper:hover) {
    border-color: var(--el-border-color);
  }

  :deep(.el-input__wrapper.is-focus) {
    border-color: var(--el-color-primary);
    box-shadow: none;
  }
}

.kv-remove {
  flex: 0 0 auto;
  color: var(--el-text-color-placeholder);
}

.kv-actions {
  padding-top: 4px;
}
</style>
