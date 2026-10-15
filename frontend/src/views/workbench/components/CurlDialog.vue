<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { CopyDocument, Upload } from '@element-plus/icons-vue'
import type { RequestSpec } from '@/api/types'
import { buildCurl, parseCurl } from '@/utils/curl'
import { useMessage } from '@/hooks/web/useMessage'

/**
 * curl 导入 / 导出。
 *
 * 两件事放在一个对话框里而不是两个菜单项：从 DevTools 复制一条 curl 进来、
 * 把调好的请求复制出去，本来就是同一段工作的两头，分开入口只会让人多找一次菜单。
 */
const visible = defineModel<boolean>({ required: true })

const props = defineProps<{ request: RequestSpec }>()
const emit = defineEmits<{ import: [RequestSpec] }>()

const { success, error: toastError } = useMessage()

const tab = ref<'import' | 'export'>('import')
const input = ref('')
const parseError = ref('')

watch(visible, (open) => {
  if (!open) return
  tab.value = 'import'
  input.value = ''
  parseError.value = ''
})

const exported = computed(() => buildCurl(props.request, true))

async function doImport() {
  parseError.value = ''
  const text = input.value.trim()
  if (!text) {
    parseError.value = '请先粘贴 curl 命令'
    return
  }
  try {
    const spec = parseCurl(text)
    emit('import', spec)
    success('已解析并填充到当前标签页')
    visible.value = false
  } catch (err) {
    parseError.value = err instanceof Error ? err.message : String(err)
  }
}

async function copyExport() {
  try {
    await navigator.clipboard.writeText(exported.value)
    success('curl 命令已复制')
  } catch (err) {
    console.error('[curl] 复制失败', err)
    toastError('复制失败，剪贴板不可用')
  }
}
</script>

<template>
  <ElDialog v-model="visible" title="curl 导入 / 导出" width="640px" append-to-body>
    <ElTabs v-model="tab">
      <ElTabPane label="从 curl 导入" name="import">
        <div class="pane">
          <div class="tip">
            把浏览器 DevTools 里「Copy as cURL」得到的命令整段粘进来即可。 支持 -X / -H / -d / -F /
            -u / -b / -k / -L 等常用开关； 地址上的查询串会被拆到 Params 面板里。
          </div>
          <ElInput
            v-model="input"
            type="textarea"
            :rows="12"
            resize="none"
            spellcheck="false"
            class="mono"
            placeholder="curl 'https://example.com/api' -H 'Content-Type: application/json' --data-raw '{&quot;a&quot;: 1}'"
          />
          <div v-if="parseError" class="error">{{ parseError }}</div>
        </div>
      </ElTabPane>

      <ElTabPane label="导出为 curl" name="export">
        <div class="pane">
          <div class="tip">
            这是当前标签页里请求的等价命令，粘贴到终端即可复现。
            {{ request.auth?.kind === 'bearer' ? '注意：Bearer token 会出现在命令里。' : '' }}
          </div>
          <pre class="export mono">{{ exported }}</pre>
        </div>
      </ElTabPane>
    </ElTabs>

    <template #footer>
      <template v-if="tab === 'import'">
        <ElButton @click="visible = false">取消</ElButton>
        <ElButton type="primary" :icon="Upload" :disabled="!input.trim()" @click="doImport">
          解析并填充
        </ElButton>
      </template>
      <template v-else>
        <ElButton @click="visible = false">关闭</ElButton>
        <ElButton type="primary" :icon="CopyDocument" @click="copyExport">复制命令</ElButton>
      </template>
    </template>
  </ElDialog>
</template>

<style lang="scss" scoped>
.pane {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.tip {
  font-size: 12px;
  line-height: 1.8;
  color: var(--text-color-secondary);
}

.mono {
  :deep(.el-textarea__inner) {
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.6;
  }
}

.export {
  max-height: 320px;
  margin: 0;
  padding: 10px 12px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-all;
  background-color: var(--el-fill-color-lighter);
  border-radius: 4px;
}

.error {
  padding: 6px 10px;
  font-size: 12px;
  color: var(--el-color-danger);
  background-color: var(--el-color-danger-light-9);
  border-radius: 4px;
}
</style>
