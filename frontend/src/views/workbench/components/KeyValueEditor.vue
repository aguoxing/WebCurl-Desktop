<script lang="ts" setup>
import { computed } from 'vue'
import { ArrowDown, Delete, Plus } from '@element-plus/icons-vue'
import type { KV } from '@/api/types'

/**
 * 名称列的联想候选。
 *
 * 由调用方决定是否提供：请求头有公认的名字清单，参数名和表单字段名则没有，
 * 那种场景下这些面板应该保持成一个普通输入框。
 */
interface NameSuggestion {
  name: string
  /** 选中后填入的推荐值；为空表示只补名称。 */
  value?: string
  /** 列表右侧的说明，帮用户在相似的名字之间分辨。 */
  note?: string
}

/** 联想列表里的一项：ElAutocomplete 只认 value 字段。 */
interface SuggestionItem {
  value: string
  hint: string
}

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
    /** 名称列的联想候选；不传则名称列是普通输入框。 */
    nameSuggestions?: NameSuggestion[]
  }>(),
  {
    namePlaceholder: '键',
    valuePlaceholder: '值',
    showEnable: true,
    minWidth: '0',
    nameSuggestions: () => []
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

/**
 * 从联想列表里选中一个名称。
 *
 * 值列还是空的时候顺手填上推荐值：选了 Content-Type 之后最想做的就是
 * 敲 application/json，而这里也是"写了却没生效"最常发生的地方。
 */
function pickName(index: number, name: string) {
  const row = rows.value[index]
  if (!row) return
  const preset = (props.nameSuggestions ?? []).find((item) => item.name === name)
  const value = row.value || preset?.value || ''
  update(rows.value.map((r, i) => (i === index ? { ...r, name, value } : r)))
}

/** ElAutocomplete 的候选来源：按当前输入过滤，空输入时给出全部。 */
function fetchNames(query: string, callback: (items: SuggestionItem[]) => void) {
  const keyword = query.trim().toLowerCase()
  const items = (props.nameSuggestions ?? [])
    .filter((item) => !keyword || item.name.toLowerCase().includes(keyword))
    .map((item) => ({ value: item.name, hint: item.value || item.note || '' }))
  callback(items)
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
      <ElAutocomplete
        v-if="nameSuggestions.length"
        class="kv-input"
        :model-value="row.name"
        :placeholder="namePlaceholder"
        :fetch-suggestions="fetchNames"
        :debounce="120"
        trigger-on-focus
        hide-loading
        value-key="value"
        @update:model-value="(v: string) => updateCell(index, 'name', v)"
        @select="(item: SuggestionItem) => pickName(index, item.value)"
      >
        <template #default="{ item }">
          <span class="kv-suggest-name">{{ item.value }}</span>
          <span v-if="item.hint" class="kv-suggest-hint">{{ item.hint }}</span>
        </template>
        <template #suffix>
          <ElIcon class="kv-suggest-caret"><ArrowDown /></ElIcon>
        </template>
      </ElAutocomplete>
      <ElInput
        v-else
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

/* 名称列变成联想框后根节点是 .el-autocomplete，内部 el-input 自己撑满，
   这里只把箭头压暗到和裸输入框一致的视觉重量。 */
.kv-suggest-caret {
  color: var(--el-text-color-placeholder);
}

.kv-suggest-name {
  font-family: var(--font-mono);
  font-size: 13px;
}

.kv-suggest-hint {
  margin-left: 10px;
  overflow: hidden;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-color-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.kv-actions {
  padding-top: 4px;
}
</style>
