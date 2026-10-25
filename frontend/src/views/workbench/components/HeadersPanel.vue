<script lang="ts" setup>
import { computed, ref } from 'vue'
import { ArrowDown } from '@element-plus/icons-vue'
import type { KV, RequestSpec } from '@/api/types'
import { COMMON_HEADER_NAMES, COMMON_HEADER_OPTIONS } from '@/utils/headers'
import KeyValueEditor from './KeyValueEditor.vue'

/**
 * 请求头面板。
 *
 * 额外提供一个"常用头"下拉：调试时反复手打 Content-Type / Accept 很费事，
 * 而这两个头又恰好是最常被写错（大小写、空格）的地方。
 *
 * 名称列的联想清单来自 @/utils/headers，与本面板的一键插入共用同一份字典：
 * 两处各写一份的话，迟早会出现"下拉里有、联想里没有"的割裂。
 */
const props = defineProps<{ request: RequestSpec }>()
const emit = defineEmits<{ update: [Partial<RequestSpec>] }>()

const headers = computed(() => props.request.headers ?? [])

/**
 * 一键插入的候选：只取带推荐值的条目。
 *
 * 没有推荐值的头（Referer、X-Api-Key 之类）插入一行空值反而多一步编辑，
 * 它们留给名称联想——敲两个字母就能选中。
 */
const PRESETS = COMMON_HEADER_OPTIONS.filter((option) => option.value !== '')

/** 名称联想：全量字典，同名头只留一条。 */
const nameSuggestions = COMMON_HEADER_NAMES

/** 已经存在的头名，用于把重复的预置项置灰而不是让用户点了才发现重复。 */
const existing = computed(() => new Set(headers.value.map((h) => h.name.toLowerCase())))

function update(patch: KV[]) {
  emit('update', { headers: patch })
}

function addPreset(index: number) {
  const preset = PRESETS[index]
  if (!preset) return
  update([...headers.value, { name: preset.name, value: preset.value, enabled: true }])
}

function clearAll() {
  update([])
}

const lastImportCount = ref(0)
const editor = ref<InstanceType<typeof KeyValueEditor> | null>(null)

/** 从标准请求头文本批量导入。 */
function importText() {
  const text = window.prompt('粘贴请求头（每行一条，形如 Name: value）')
  if (!text) return
  lastImportCount.value = editor.value?.importFromText(text) ?? 0
}
</script>

<template>
  <div class="panel">
    <div class="panel-body">
      <KeyValueEditor
        ref="editor"
        :model-value="headers"
        name-placeholder="请求头名称"
        value-placeholder="值"
        :name-suggestions="nameSuggestions"
        @update:model-value="update"
      />
    </div>

    <div class="panel-foot">
      <div class="foot-left">
        <ElDropdown trigger="click" @command="(i: number) => addPreset(i)">
          <ElButton link size="small">
            常用请求头<ElIcon class="el-icon--right"><ArrowDown /></ElIcon>
          </ElButton>
          <template #dropdown>
            <ElDropdownMenu class="preset-menu">
              <ElDropdownItem
                v-for="(preset, index) in PRESETS"
                :key="`${preset.name}-${preset.value}`"
                :command="index"
                :disabled="existing.has(preset.name.toLowerCase())"
              >
                <span class="preset-name">{{ preset.name }}</span>
                <span class="preset-value">{{ preset.value }}</span>
              </ElDropdownItem>
            </ElDropdownMenu>
          </template>
        </ElDropdown>
        <ElButton link size="small" @click="importText">粘贴导入</ElButton>
        <ElButton link size="small" :disabled="!headers.length" @click="clearAll"
          >全部清空</ElButton
        >
        <span v-if="lastImportCount" class="count">已导入 {{ lastImportCount }} 条</span>
      </div>
      <div class="preview">
        <span class="preview-label">已启用</span>
        <span class="preview-url"
          >{{ headers.filter((h) => h.enabled && h.name).length }} 个请求头</span
        >
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use './panel.scss';

/* 字典里的条目比原来多，超出可视高度的部分让它自己滚，
   而不是把下拉撑到屏幕外面去。 */
.preset-menu {
  max-height: 320px;
  overflow-y: auto;
}

.preset-name {
  font-family: var(--font-mono);
  font-size: 12px;
  font-weight: 600;
}

.preset-value {
  margin-left: 8px;
  font-family: var(--font-mono);
  font-size: 11px;
  color: var(--text-color-secondary);
}
</style>
