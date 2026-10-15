<script lang="ts" setup>
import { computed } from 'vue'
import { Delete, MagicStick, Paperclip, Plus, Upload } from '@element-plus/icons-vue'
import { requestApi } from '@/api'
import type { BodyKind, FileField, KV, RequestSpec } from '@/api/types'
import { prettyJSON } from '@/utils/format'
import KeyValueEditor from './KeyValueEditor.vue'

/**
 * 请求体面板。
 *
 * 主体按"文本类 / 表单类 / 文件类"分成三种形态，而不是把所有类型都摊成一个
 * 大文本框：这三类的编辑方式差别太大（一个有格式化、一个要成对输入、一个要选文件），
 * 混在一起会让每种操作都别扭。
 */
const props = defineProps<{ request: RequestSpec }>()
const emit = defineEmits<{ update: [Partial<RequestSpec>] }>()

/** 各类型对应的 Content-Type，用于在界面上明确告知"后端会补上这个头"。 */
const CONTENT_TYPES: Record<BodyKind, string> = {
  none: '',
  json: 'application/json',
  xml: 'application/xml',
  text: 'text/plain',
  html: 'text/html',
  'x-www-form-urlencoded': 'application/x-www-form-urlencoded',
  'form-data': 'multipart/form-data（含随机 boundary）',
  binary: '按文件扩展名推断，兜底 application/octet-stream'
}

const KIND_OPTIONS: { value: BodyKind; label: string }[] = [
  { value: 'none', label: '无' },
  { value: 'json', label: 'JSON' },
  { value: 'xml', label: 'XML' },
  { value: 'text', label: 'Text' },
  { value: 'html', label: 'HTML' },
  { value: 'x-www-form-urlencoded', label: 'Form URL Encoded' },
  { value: 'form-data', label: 'Multipart Form Data' },
  { value: 'binary', label: 'Binary' }
]

const kind = computed<BodyKind>(() => props.request.bodyKind ?? 'none')
const isTextKind = computed(() => ['json', 'xml', 'text', 'html'].includes(kind.value))
const isFormKind = computed(
  () => kind.value === 'x-www-form-urlencoded' || kind.value === 'form-data'
)

const contentType = computed(() => CONTENT_TYPES[kind.value])

function patch(p: Partial<RequestSpec>) {
  emit('update', p)
}

function setKind(value: BodyKind) {
  patch({ bodyKind: value })
}

function setBody(value: string) {
  patch({ body: value })
}

function formatBody() {
  const next = prettyJSON(props.request.body ?? '')
  if (next === (props.request.body ?? '')) {
    return
  }
  setBody(next)
}

function clearBody() {
  patch({ body: '' })
}

// --- 表单 ---

const form = computed(() => props.request.form ?? [])

function setForm(next: KV[]) {
  patch({ form: next })
}

const editor = ref<InstanceType<typeof KeyValueEditor> | null>(null)

function importForm() {
  const text = window.prompt('粘贴表单内容（每行一条，形如 key=value 或 key: value）')
  if (!text) return
  editor.value?.importFromText(text)
}

// --- 文件 ---

const files = computed<FileField[]>(() => props.request.files ?? [])

/**
 * 选文件。
 *
 * 文件路径只能由系统对话框给出，界面不提供手输：让用户手打一个 Windows 路径
 * 再让后端去读，是"打错了才发现"的典型场景，而路径正确性完全无法预判。
 */
async function pickFiles(target: 'files' | 'binary') {
  try {
    const picked = await requestApi.pickFiles(target === 'files')
    if (!picked.length) return
    if (target === 'binary') {
      patch({ binaryPath: picked[0] })
      return
    }
    const next: FileField[] = picked.map((path) => ({
      fieldName: guessFieldName(path),
      path,
      enabled: true
    }))
    const existed = new Set(files.value.map((f) => f.path))
    patch({ files: [...files.value, ...next.filter((f) => !existed.has(f.path))] })
  } catch (error) {
    console.error('[body] 选择文件失败', error)
  }
}

/** 用文件名猜一个字段名，用户改起来比从空开始快。 */
function guessFieldName(path: string): string {
  const base = path.split(/[\\/]/).pop() ?? 'file'
  const dot = base.lastIndexOf('.')
  return dot > 0 ? base.slice(0, dot) : base
}

function updateFile(index: number, p: Partial<FileField>) {
  patch({ files: files.value.map((f, i) => (i === index ? { ...f, ...p } : f)) })
}

function removeFile(index: number) {
  patch({ files: files.value.filter((_, i) => i !== index) })
}

function shortPath(path: string): string {
  return path.split(/[\\/]/).pop() ?? path
}
</script>

<template>
  <div class="panel body-panel">
    <div class="kind-bar">
      <ElRadioGroup :model-value="kind" size="small" @update:model-value="setKind">
        <ElRadioButton v-for="opt in KIND_OPTIONS" :key="opt.value" :value="opt.value">
          {{ opt.label }}
        </ElRadioButton>
      </ElRadioGroup>
    </div>

    <div class="panel-body">
      <!-- 无请求体 -->
      <div v-if="kind === 'none'" class="empty">
        该请求不带请求体。GET / HEAD 通常如此；需要发送数据时在上方选一种类型。
      </div>

      <!-- 文本类 -->
      <template v-else-if="isTextKind">
        <div class="editor-toolbar">
          <ElButton
            link
            size="small"
            :icon="MagicStick"
            :disabled="kind !== 'json'"
            @click="formatBody"
          >
            格式化
          </ElButton>
          <ElButton link size="small" :disabled="!request.body" @click="clearBody">清空</ElButton>
          <span class="ct">发送时自动带上 Content-Type: {{ contentType }}</span>
        </div>
        <ElInput
          class="body-text"
          type="textarea"
          :model-value="request.body ?? ''"
          :rows="12"
          resize="none"
          spellcheck="false"
          placeholder="请求体内容"
          @update:model-value="setBody"
        />
      </template>

      <!-- 表单类 -->
      <template v-else-if="isFormKind">
        <div class="section-title">表单字段</div>
        <KeyValueEditor
          ref="editor"
          :model-value="form"
          name-placeholder="字段名"
          value-placeholder="字段值"
          @update:model-value="setForm"
        />

        <template v-if="kind === 'form-data'">
          <div class="section-title files-title">
            文件字段
            <ElButton link size="small" :icon="Upload" @click="pickFiles('files')">
              选择文件
            </ElButton>
          </div>

          <div v-if="!files.length" class="empty small">
            还没有文件字段。multipart 表单里的文件需要走系统文件选择框，路径无法手输。
          </div>

          <div v-for="(file, index) in files" :key="file.path" class="file-row">
            <ElCheckbox
              :model-value="file.enabled"
              @update:model-value="(v: any) => updateFile(index, { enabled: Boolean(v) })"
            />
            <ElInput
              class="file-name"
              :model-value="file.fieldName"
              placeholder="字段名"
              @update:model-value="(v: string) => updateFile(index, { fieldName: v })"
            />
            <span class="file-path" :title="file.path">{{ shortPath(file.path) }}</span>
            <ElButton link :icon="Delete" @click="removeFile(index)" />
          </div>
        </template>

        <div class="ct-block">发送时自动带上 Content-Type: {{ contentType }}</div>
      </template>

      <!-- 二进制 -->
      <template v-else-if="kind === 'binary'">
        <div class="binary-box">
          <ElButton :icon="Paperclip" @click="pickFiles('binary')">选择文件…</ElButton>
          <div v-if="request.binaryPath" class="binary-path" :title="request.binaryPath">
            {{ request.binaryPath }}
          </div>
          <div v-else class="empty small">尚未选择文件。整份文件会作为原始请求体发出。</div>
          <ElButton v-if="request.binaryPath" link size="small" @click="patch({ binaryPath: '' })">
            清除
          </ElButton>
        </div>
        <div class="ct-block">发送时自动带上 Content-Type: {{ contentType }}</div>
      </template>
    </div>

    <div class="panel-foot">
      <div class="foot-left">
        <ElButton
          v-if="isFormKind"
          link
          size="small"
          :icon="Plus"
          @click="patch({ form: [...form, { name: '', value: '', enabled: true }] })"
        >
          添加字段
        </ElButton>
        <ElButton v-if="isFormKind" link size="small" @click="importForm">粘贴导入</ElButton>
        <span v-if="isFormKind" class="count">
          表单字段 {{ form.filter((f) => f.enabled && f.name).length }} 项
          <template v-if="kind === 'form-data'">
            · 文件 {{ files.filter((f) => f.enabled).length }} 个</template
          >
        </span>
      </div>
      <div class="preview">
        <span class="preview-label">类型</span>
        <span class="preview-url">{{ kind }}</span>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use './panel.scss';

.body-panel {
  overflow: hidden;
}

.kind-bar {
  flex: 0 0 auto;
  padding: 8px 12px 0;
  overflow-x: auto;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.kind-bar :deep(.el-radio-group) {
  flex-wrap: nowrap;
  padding-bottom: 6px;
}

.editor-toolbar {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 6px;
}

.ct {
  margin-left: auto;
  font-size: 12px;
  color: var(--text-color-secondary);
}

.ct-block {
  margin-top: 12px;
  font-size: 12px;
  color: var(--text-color-secondary);
}

.body-text {
  :deep(.el-textarea__inner) {
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.6;
  }
}

.empty {
  font-size: 13px;
  line-height: 1.8;
  color: var(--text-color-secondary);
}

.empty.small {
  font-size: 12px;
}

.files-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 16px;
}

.file-row {
  display: flex;
  gap: 6px;
  align-items: center;
  padding: 2px 0;
}

.file-name {
  flex: 0 0 160px;
}

.file-path {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-color-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.binary-box {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
}

.binary-path {
  max-width: 100%;
  overflow: hidden;
  font-family: var(--font-mono);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
