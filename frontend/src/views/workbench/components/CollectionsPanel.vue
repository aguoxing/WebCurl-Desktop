<script lang="ts" setup>
import { computed, ref } from 'vue'
import { Delete, Edit, Folder, FolderOpened, Plus, Upload } from '@element-plus/icons-vue'
import type { Collection, CollectionItem, RequestSpec } from '@/api/types'
import { methodChipStyle } from '@/utils/format'
import { useWorkspaceStore } from '@/store/modules/workspace'
import { useMessage } from '@/hooks/web/useMessage'

/**
 * 集合面板：把调试过的接口沉淀成可复用的条目。
 *
 * 用自绘的折叠列表而不是 el-tree：集合层级只可能有两级（集合 → 接口），
 * el-tree 的拖拽、复选框、懒加载在这里全是多余的噪声，还会带来
 * "为什么拖不动"的困惑——本版刻意不做拖拽排序。
 */
const emit = defineEmits<{
  open: [
    spec: RequestSpec,
    source: { collectionId: string; itemId: string } | undefined,
    title: string
  ]
}>()

const workspace = useWorkspaceStore()
const { success, error: toastError, confirm } = useMessage()

const keyword = ref('')
const expanded = ref<Set<string>>(new Set())

const filtered = computed<Collection[]>(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return workspace.collections
  return workspace.collections
    .map((c) => ({
      ...c,
      items: c.items.filter(
        (i) => i.name.toLowerCase().includes(kw) || i.request.url.toLowerCase().includes(kw)
      )
    }))
    .filter((c) => c.name.toLowerCase().includes(kw) || c.items.length > 0)
})

function isExpanded(id: string): boolean {
  // 搜索时全部展开：用户输入关键字就是为了马上看到匹配到哪几条。
  if (keyword.value.trim()) return true
  return expanded.value.has(id)
}

function toggle(id: string) {
  const next = new Set(expanded.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expanded.value = next
}

function openItem(collection: Collection, item: CollectionItem) {
  emit('open', item.request, { collectionId: collection.id, itemId: item.id }, item.name)
}

async function createCollection() {
  try {
    const { value } = await ElMessageBox.prompt('集合名称', '新建集合', {
      confirmButtonText: '创建',
      cancelButtonText: '取消',
      inputPlaceholder: '例如：用户中心'
    })
    const name = (value ?? '').trim()
    if (!name) return
    const collection = workspace.createCollection(name)
    await workspace.saveCollection(collection)
    expanded.value = new Set([...expanded.value, collection.id])
    success(`已创建集合「${name}」`)
  } catch {
    // 用户取消
  }
}

async function renameCollection(collection: Collection) {
  try {
    const { value } = await ElMessageBox.prompt('集合名称', '重命名集合', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputValue: collection.name
    })
    const name = (value ?? '').trim()
    if (!name || name === collection.name) return
    await workspace.saveCollection({ ...collection, name, updatedAt: new Date().toISOString() })
  } catch {
    // 用户取消
  }
}

async function removeCollection(collection: Collection) {
  try {
    await confirm(`确定删除集合「${collection.name}」及其中的 ${collection.items.length} 个接口？`)
  } catch {
    return
  }
  await workspace.deleteCollection(collection.id)
}

async function removeItem(collection: Collection, item: CollectionItem) {
  try {
    await confirm(`确定从集合中移除「${item.name}」？`)
  } catch {
    return
  }
  await workspace.deleteCollectionItem(collection.id, item.id)
}

async function rename(request: CollectionItem, collection: Collection) {
  try {
    const { value } = await ElMessageBox.prompt('接口名称', '重命名接口', {
      confirmButtonText: '保存',
      cancelButtonText: '取消',
      inputValue: request.name
    })
    const name = (value ?? '').trim()
    if (!name || name === request.name) return
    await workspace.updateCollectionItem(collection.id, { ...request, name })
  } catch {
    // 用户取消
  }
}

async function importFile() {
  try {
    const result = await workspace.importFromFile()
    if (!result) return
    success(`导入完成：${result.collections} 个集合、${result.history} 条历史`)
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

async function exportFile() {
  try {
    const path = await workspace.exportToFile()
    if (path) success(`已导出到 ${path}`)
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

defineExpose({ createCollection, importFile, exportFile })
</script>

<template>
  <div class="side-panel">
    <div class="side-toolbar">
      <ElInput
        v-model="keyword"
        size="small"
        placeholder="搜索集合或接口"
        clearable
        :prefix-icon="FolderOpened"
      />
      <ElTooltip content="新建集合" placement="bottom">
        <ElButton size="small" :icon="Plus" @click="createCollection" />
      </ElTooltip>
      <ElTooltip content="从文件导入" placement="bottom">
        <ElButton size="small" :icon="Upload" @click="importFile" />
      </ElTooltip>
    </div>

    <ElScrollbar class="side-scroll">
      <div v-if="!filtered.length" class="side-empty">
        <p v-if="keyword">没有匹配的集合或接口</p>
        <template v-else>
          <p>还没有任何集合。</p>
          <p class="side-empty-sub">把调试好的请求保存进集合，下次可以直接取用。</p>
        </template>
      </div>

      <div v-for="collection in filtered" :key="collection.id" class="collection">
        <div class="collection-head" @click="toggle(collection.id)">
          <ElIcon class="collection-icon">
            <component :is="isExpanded(collection.id) ? FolderOpened : Folder" />
          </ElIcon>
          <span class="collection-name">{{ collection.name }}</span>
          <span class="collection-count">{{ collection.items.length }}</span>
          <span class="row-actions" @click.stop>
            <ElIcon class="row-action" title="重命名" @click="renameCollection(collection)">
              <Edit />
            </ElIcon>
            <ElIcon class="row-action is-danger" title="删除" @click="removeCollection(collection)">
              <Delete />
            </ElIcon>
          </span>
        </div>

        <div v-if="isExpanded(collection.id)" class="collection-items">
          <div v-if="!collection.items.length" class="collection-empty">集合还是空的</div>
          <div
            v-for="item in collection.items"
            :key="item.id"
            class="item"
            :title="item.request.url"
            @click="openItem(collection, item)"
          >
            <span class="item-method" :style="methodChipStyle(item.request.method)">
              {{ (item.request.method || 'GET').toUpperCase() }}
            </span>
            <span class="item-name">{{ item.name }}</span>
            <span class="row-actions" @click.stop>
              <ElIcon class="row-action" title="重命名" @click="rename(item, collection)">
                <Edit />
              </ElIcon>
              <ElIcon
                class="row-action is-danger"
                title="移除"
                @click="removeItem(collection, item)"
              >
                <Delete />
              </ElIcon>
            </span>
          </div>
        </div>
      </div>
    </ElScrollbar>

    <div class="side-foot">
      <ElButton link size="small" @click="exportFile">导出</ElButton>
      <ElButton link size="small" @click="importFile">导入</ElButton>
      <ElButton link size="small" @click="workspace.openDataDir()">数据目录</ElButton>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use './side-panel.scss';
</style>
