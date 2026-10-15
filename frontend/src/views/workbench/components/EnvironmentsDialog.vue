<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { Check, Delete, Plus } from '@element-plus/icons-vue'
import type { Environment, Variable } from '@/api/types'
import KeyValueEditor from './KeyValueEditor.vue'
import { useWorkspaceStore } from '@/store/modules/workspace'
import { useMessage } from '@/hooks/web/useMessage'

/**
 * 环境变量管理。
 *
 * 左选右编而不是把所有环境铺成一张大表：变量是"一套一套"使用的
 * （本地一套、测试一套），按环境分开编辑才能一眼看清某套环境里有哪些名字。
 */
const visible = defineModel<boolean>({ required: true })

/**
 * 变量语法的字面样例。
 *
 * 必须放在脚本里再插值，不能直接写进模板：模板里的 `{{` / `}}` 是插值定界符，
 * 写 `{{ '{{host}}' }}` 会让编译器在第一个 `}}` 处提前收尾，报"字符串未终止"。
 */
const VAR_EXAMPLE = '{{host}}'

const workspace = useWorkspaceStore()
const { success, confirm, error: toastError } = useMessage()

const selectedId = ref('')
const editor = ref<InstanceType<typeof KeyValueEditor> | null>(null)

/** 打开时定位到当前激活的环境，没有就选第一个。 */
watch(visible, (open) => {
  if (!open) return
  selectedId.value = workspace.activeEnv || workspace.environments[0]?.id || ''
})

const selected = computed<Environment | null>(
  () => workspace.environments.find((e) => e.id === selectedId.value) ?? null
)

/** 变量列表在一个环境内的重复名（启用状态下），重复会让替换结果取决于顺序。 */
const duplicateNames = computed(() => {
  const seen = new Map<string, number>()
  for (const v of selected.value?.variables ?? []) {
    if (!v.enabled || !v.name) continue
    seen.set(v.name, (seen.get(v.name) ?? 0) + 1)
  }
  return [...seen.entries()].filter(([, count]) => count > 1).map(([name]) => name)
})

async function createEnvironment() {
  const env: Environment = {
    id: `env-${Date.now().toString(36)}`,
    name: `环境 ${workspace.environments.length + 1}`,
    variables: []
  }
  await workspace.saveEnvironment(env)
  selectedId.value = env.id
}

async function removeEnvironment(env: Environment) {
  try {
    await confirm(`删除环境「${env.name}」？其中的变量会一并删除。`)
  } catch {
    return
  }
  await workspace.deleteEnvironment(env.id)
  if (selectedId.value === env.id) {
    selectedId.value = workspace.environments[0]?.id ?? ''
  }
}

async function renameEnvironment(env: Environment, name: string) {
  const trimmed = name.trim()
  if (!trimmed || trimmed === env.name) return
  await workspace.saveEnvironment({ ...env, name: trimmed })
}

async function activate(env: Environment) {
  await workspace.setActiveEnvironment(env.id === workspace.activeEnv ? '' : env.id)
  success(env.id === workspace.activeEnv ? '已切换到该环境' : '已取消环境')
}

async function updateVariables(variables: Variable[]) {
  const env = selected.value
  if (!env) return
  await workspace.saveEnvironment({ ...env, variables })
}

async function importVariables() {
  const text = window.prompt('粘贴变量（每行一条，形如 name=value 或 name: value）')
  if (!text) return
  const count = editor.value?.importFromText(text) ?? 0
  success(count ? `已导入 ${count} 个变量` : '没有解析到变量')
}

async function copyFromActive() {
  const env = selected.value
  const active = workspace.activeEnvironment
  if (!env || !active || active.id === env.id) return
  try {
    await confirm(`把「${active.name}」的变量复制到「${env.name}」？当前内容会被覆盖。`)
  } catch {
    return
  }
  await updateVariables(JSON.parse(JSON.stringify(active.variables)) as Variable[])
  success('已复制')
}
</script>

<template>
  <ElDialog v-model="visible" title="环境变量" width="720px" append-to-body>
    <div class="env">
      <div class="env-list">
        <div class="env-list-head">
          <span>环境</span>
          <ElButton link size="small" :icon="Plus" @click="createEnvironment">新建</ElButton>
        </div>

        <div v-if="!workspace.environments.length" class="env-empty">
          还没有环境。新建一个后，就能在地址里用 <code>{{ VAR_EXAMPLE }}</code> 这样的占位符了。
        </div>

        <div
          v-for="env in workspace.environments"
          :key="env.id"
          class="env-item"
          :class="{ 'is-selected': env.id === selectedId }"
          @click="selectedId = env.id"
        >
          <ElIcon
            class="env-check"
            :class="{ 'is-active': env.id === workspace.activeEnv }"
            :title="env.id === workspace.activeEnv ? '当前生效的环境' : '设为当前环境'"
            @click.stop="activate(env)"
          >
            <Check />
          </ElIcon>
          <span class="env-name" :title="env.name">{{ env.name }}</span>
          <span class="env-var-count">{{ env.variables.length }}</span>
          <ElIcon class="env-del" title="删除" @click.stop="removeEnvironment(env)">
            <Delete />
          </ElIcon>
        </div>
      </div>

      <div class="env-editor">
        <template v-if="selected">
          <div class="env-editor-head">
            <ElInput
              :model-value="selected.name"
              size="small"
              maxlength="60"
              @update:model-value="(v: string) => renameEnvironment(selected!, v)"
            />
            <ElButton
              size="small"
              :type="selected.id === workspace.activeEnv ? 'success' : 'default'"
              @click="activate(selected)"
            >
              {{ selected.id === workspace.activeEnv ? '已启用' : '设为当前环境' }}
            </ElButton>
          </div>

          <div v-if="duplicateNames.length" class="env-warn">
            变量名重复：{{ duplicateNames.join('、') }}。替换时以靠前的一条为准。
          </div>

          <KeyValueEditor
            ref="editor"
            :model-value="selected.variables"
            name-placeholder="变量名"
            value-placeholder="变量值"
            @update:model-value="updateVariables"
          />

          <div class="env-hint">
            变量通过 <code>{{ VAR_EXAMPLE }}</code> 这样的写法在地址、请求头、请求体等处引用；
            替换只发生在发送前的一刻，编辑中的内容仍保持原文。
          </div>
        </template>

        <div v-else class="env-editor-empty">左侧选择或新建一个环境</div>
      </div>
    </div>

    <template #footer>
      <ElButton v-if="selected && workspace.activeEnvironment" @click="copyFromActive">
        从当前环境复制
      </ElButton>
      <ElButton v-if="selected" @click="importVariables">粘贴导入</ElButton>
      <div style="margin-left: auto" />
      <ElButton @click="visible = false">关闭</ElButton>
    </template>
  </ElDialog>
</template>

<style lang="scss" scoped>
.env {
  display: flex;
  gap: 14px;
  height: 420px;
}

.env-list {
  display: flex;
  flex: 0 0 200px;
  flex-direction: column;
  min-height: 0;
  overflow-y: auto;
  border-right: 1px solid var(--el-border-color-lighter);
}

.env-list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 8px 6px 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-color-secondary);
}

.env-empty,
.env-editor-empty {
  padding: 20px 12px;
  font-size: 12px;
  line-height: 1.8;
  color: var(--text-color-secondary);
}

.env-item {
  display: flex;
  gap: 6px;
  align-items: center;
  padding: 6px 8px;
  font-size: 13px;
  cursor: pointer;
  border-radius: 4px;
}

.env-item:hover {
  background-color: var(--el-fill-color-light);
}

.env-item.is-selected {
  background-color: var(--el-color-primary-light-9);
}

.env-check {
  flex: 0 0 auto;
  font-size: 13px;
  color: var(--el-text-color-placeholder);
}

.env-check.is-active {
  color: var(--el-color-success);
}

.env-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.env-var-count {
  flex: 0 0 auto;
  font-size: 11px;
  color: var(--text-color-secondary);
}

.env-del {
  flex: 0 0 auto;
  opacity: 0;
}

.env-item:hover .env-del {
  opacity: 1;
}

.env-del:hover {
  color: var(--el-color-danger);
}

.env-editor {
  display: flex;
  flex: 1 1 auto;
  min-width: 0;
  flex-direction: column;
  overflow-y: auto;
}

.env-editor-head {
  display: flex;
  flex: 0 0 auto;
  gap: 8px;
  align-items: center;
  margin-bottom: 12px;
}

.env-warn {
  margin-bottom: 8px;
  padding: 6px 10px;
  font-size: 12px;
  color: var(--el-color-warning);
  background-color: var(--el-color-warning-light-9);
  border-radius: 4px;
}

.env-hint {
  margin-top: 14px;
  font-size: 12px;
  line-height: 1.8;
  color: var(--text-color-secondary);
}

code {
  padding: 1px 4px;
  font-family: var(--font-mono);
  font-size: 11px;
  background-color: var(--el-fill-color);
  border-radius: 3px;
}
</style>
