<script lang="ts" setup>
import { computed, ref } from 'vue'
import { ArrowDown } from '@element-plus/icons-vue'
import type { KV, RequestSpec } from '@/api/types'
import KeyValueEditor from './KeyValueEditor.vue'

/**
 * 请求头面板。
 *
 * 额外提供一个"常用头"下拉：调试时反复手打 Content-Type / Accept 很费事，
 * 而这两个头又恰好是最常被写错（大小写、空格）的地方。
 */
const props = defineProps<{ request: RequestSpec }>()
const emit = defineEmits<{ update: [Partial<RequestSpec>] }>()

const headers = computed(() => props.request.headers ?? [])

const PRESETS: { label: string; name: string; value: string }[] = [
  { label: 'Content-Type: application/json', name: 'Content-Type', value: 'application/json' },
  {
    label: 'Content-Type: application/x-www-form-urlencoded',
    name: 'Content-Type',
    value: 'application/x-www-form-urlencoded'
  },
  { label: 'Accept: application/json', name: 'Accept', value: 'application/json' },
  { label: 'Accept: */*', name: 'Accept', value: '*/*' },
  { label: 'Authorization: Bearer ', name: 'Authorization', value: 'Bearer ' },
  { label: 'User-Agent', name: 'User-Agent', value: 'WebCurl/1.0' },
  { label: 'Referer', name: 'Referer', value: '' },
  { label: 'Origin', name: 'Origin', value: '' },
  { label: 'X-Request-Id', name: 'X-Request-Id', value: '' }
]

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
            <ElDropdownMenu>
              <ElDropdownItem
                v-for="(preset, index) in PRESETS"
                :key="preset.label"
                :command="index"
                :disabled="preset.value !== '' && existing.has(preset.name.toLowerCase())"
              >
                {{ preset.label }}
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
</style>
