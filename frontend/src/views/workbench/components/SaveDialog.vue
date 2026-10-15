<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { Plus } from '@element-plus/icons-vue'
import type { RequestSpec } from '@/api/types'
import { useWorkspaceStore } from '@/store/modules/workspace'
import { useMessage } from '@/hooks/web/useMessage'

/**
 * 保存请求到集合。
 *
 * 允许在对话框里直接新建集合：用户往往是在"想保存了"的那一刻才意识到
 * 还没有合适的集合，此时把他推到左侧栏去建一个再回来，等于让他丢掉
 * 当前已经填好的这个名称。
 */
const visible = defineModel<boolean>({ required: true })

const props = defineProps<{
  request: RequestSpec
  /** 当前标签页标题，作为默认名称。 */
  defaultName: string
  /** 已保存过时传入来源，用于定位默认选中的集合。 */
  source?: { collectionId: string; itemId: string }
}>()

const emit = defineEmits<{ saved: [collectionId: string, itemId: string, name: string] }>()

const workspace = useWorkspaceStore()
const { success, error: toastError, confirm } = useMessage()

const collectionId = ref('')
const name = ref('')
const creating = ref(false)
const newCollectionName = ref('')

watch(visible, (open) => {
  if (!open) return
  name.value = props.defaultName === '新建请求' ? '' : props.defaultName
  collectionId.value = props.source?.collectionId ?? workspace.collections[0]?.id ?? ''
  creating.value = workspace.collections.length === 0
  newCollectionName.value = ''
})

const selected = computed(() => workspace.collections.find((c) => c.id === collectionId.value))

const canSubmit = computed(
  () =>
    name.value.trim().length > 0 &&
    (creating.value ? newCollectionName.value.trim().length > 0 : Boolean(collectionId.value))
)

async function createCollection() {
  const raw = newCollectionName.value.trim()
  if (!raw) return
  const collection = workspace.createCollection(raw)
  await workspace.saveCollection(collection)
  collectionId.value = collection.id
  creating.value = false
}

async function submit() {
  if (!canSubmit.value) return
  try {
    let targetId = collectionId.value
    if (creating.value) {
      const collection = workspace.createCollection(newCollectionName.value.trim())
      await workspace.saveCollection(collection)
      targetId = collection.id
    }
    const item = await workspace.addToCollection(targetId, name.value.trim(), props.request)
    emit('saved', targetId, item.id, name.value.trim())
    success(`已保存到「${workspace.collections.find((c) => c.id === targetId)?.name}」`)
    visible.value = false
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

/** 覆盖已保存的那一条。 */
async function overwrite() {
  const src = props.source
  if (!src) return
  try {
    await confirm(`覆盖集合里的「${props.defaultName}」？原有内容会被当前编辑的内容替换。`)
  } catch {
    return
  }
  try {
    await workspace.updateCollectionItem(src.collectionId, {
      id: src.itemId,
      name: props.defaultName,
      request: props.request
    })
    emit('saved', src.collectionId, src.itemId, props.defaultName)
    success('已覆盖保存')
    visible.value = false
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}
</script>

<template>
  <ElDialog v-model="visible" title="保存请求" width="460px" append-to-body>
    <div class="form">
      <div class="form-row">
        <div class="form-label">名称</div>
        <ElInput v-model="name" placeholder="接口名称" maxlength="80" />
      </div>

      <div v-if="!creating" class="form-row">
        <div class="form-label">集合</div>
        <div class="form-control">
          <ElSelect v-model="collectionId" placeholder="选择集合" style="width: 100%">
            <ElOption
              v-for="c in workspace.collections"
              :key="c.id"
              :label="`${c.name}（${c.items.length}）`"
              :value="c.id"
            />
          </ElSelect>
          <ElButton link size="small" :icon="Plus" @click="creating = true">新建集合</ElButton>
        </div>
      </div>

      <div v-else class="form-row">
        <div class="form-label">新集合</div>
        <div class="form-control">
          <ElInput
            v-model="newCollectionName"
            placeholder="集合名称"
            maxlength="80"
            @keyup.enter="createCollection"
          />
          <ElButton v-if="workspace.collections.length" link size="small" @click="creating = false">
            选择已有集合
          </ElButton>
        </div>
      </div>

      <div class="preview">
        <span class="preview-method">{{ (request.method || 'GET').toUpperCase() }}</span>
        <span class="preview-url">{{ request.url || '（尚未填写地址）' }}</span>
      </div>
    </div>

    <template #footer>
      <div class="footer">
        <ElButton v-if="source" link type="warning" @click="overwrite">覆盖原条目</ElButton>
        <div class="footer-right">
          <ElButton @click="visible = false">取消</ElButton>
          <ElButton type="primary" :disabled="!canSubmit" @click="submit">
            {{ selected && source && source.collectionId === collectionId ? '另存为' : '保存' }}
          </ElButton>
        </div>
      </div>
    </template>
  </ElDialog>
</template>

<style lang="scss" scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.form-row {
  display: flex;
  gap: 10px;
  align-items: flex-start;
}

.form-label {
  flex: 0 0 56px;
  padding-top: 6px;
  font-size: 13px;
  color: var(--text-color-secondary);
}

.form-control {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
  align-items: flex-start;
}

.form-control :deep(.el-input),
.form-row > :deep(.el-input) {
  flex: 1 1 auto;
  min-width: 0;
}

.preview {
  display: flex;
  gap: 8px;
  align-items: baseline;
  padding: 8px 10px;
  font-size: 12px;
  background-color: var(--el-fill-color-lighter);
  border-radius: 4px;
}

.preview-method {
  flex: 0 0 auto;
  font-family: var(--font-mono);
  font-weight: 700;
  color: var(--el-color-primary);
}

.preview-url {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-family: var(--font-mono);
  color: var(--text-color-secondary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.footer-right {
  display: flex;
  gap: 8px;
  margin-left: auto;
}
</style>
