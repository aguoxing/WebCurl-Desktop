<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { Setting, Switch } from '@element-plus/icons-vue'
import { useWorkspaceStore } from '@/store/modules/workspace'
import { useMessage } from '@/hooks/web/useMessage'
import EnvironmentsDialog from '@/views/workbench/components/EnvironmentsDialog.vue'

/**
 * 环境切换器（放在顶栏）。
 *
 * 环境是"整个工作台"级别的选择而不是某个请求的属性：切换后所有标签页的
 * {{变量}} 都会指向另一套值，因此它的入口必须在窗口级别可见，
 * 而不能埋在某一个页面的面板里。
 *
 * 自包含地挂上环境管理对话框：顶栏不依赖工作台页面是否挂载，
 * 在 Mock / 设置页也能直接改环境。
 */
defineOptions({ name: 'EnvironmentSelect' })

const workspace = useWorkspaceStore()
const { success } = useMessage()

const dialogVisible = ref(false)
const NONE = '__none__'

const value = computed({
  get: () => workspace.activeEnv || NONE,
  set: (v: string) => {
    workspace.setActiveEnvironment(v === NONE ? '' : v).catch((err) => {
      console.error('[env] 切换环境失败', err)
    })
  }
})

const label = computed(() => {
  const env = workspace.activeEnvironment
  if (!env) return '无环境'
  const count = env.variables.filter((v) => v.enabled && v.name).length
  return `${env.name}（${count}）`
})

/** 当前环境里被引用但未定义的名字数量，用于给出"变量没配全"的提醒。 */
const definedCount = computed(
  () => (workspace.activeEnvironment?.variables ?? []).filter((v) => v.enabled && v.name).length
)

function openDialog() {
  dialogVisible.value = true
}

onMounted(async () => {
  // 顶栏比工作台更早挂载，这里主动拉一次快照，用户一开局就能看到环境名。
  try {
    await workspace.load()
  } catch (error) {
    console.error('[env] 读取工作区数据失败', error)
  }
})

/**
 * 关闭管理框时给一句确认。
 *
 * 用 watch 而不是接对话框的 closed 事件：事件要经过两层组件转发才能到这里，
 * 而这只是"提醒一下"的弱需求，不值得为它把事件链拉长。
 */
watch(dialogVisible, (open, wasOpen) => {
  if (wasOpen && !open && workspace.activeEnvironment) {
    success(`当前环境：${workspace.activeEnvironment.name}`)
  }
})
</script>

<template>
  <div class="env-select">
    <ElTooltip
      :content="definedCount ? `当前环境定义 ${definedCount} 个变量` : '当前没有启用环境'"
      placement="bottom"
    >
      <ElDropdown trigger="click" placement="bottom-end">
        <button class="env-btn" type="button">
          <ElIcon class="env-icon"><Switch /></ElIcon>
          <span class="env-label">{{ label }}</span>
        </button>
        <template #dropdown>
          <ElDropdownMenu>
            <ElDropdownItem :disabled="!workspace.activeEnv" @click="value = NONE">
              无环境
            </ElDropdownItem>
            <ElDropdownItem
              v-for="env in workspace.environments"
              :key="env.id"
              :class="{ 'is-active': env.id === workspace.activeEnv }"
              @click="value = env.id"
            >
              <span class="env-option">{{ env.name }}</span>
              <span class="env-option-count">
                {{ env.variables.filter((v) => v.enabled && v.name).length }}
              </span>
            </ElDropdownItem>
            <ElDropdownItem divided @click="openDialog">
              <ElIcon><Setting /></ElIcon>
              管理环境变量…
            </ElDropdownItem>
          </ElDropdownMenu>
        </template>
      </ElDropdown>
    </ElTooltip>

    <EnvironmentsDialog v-model="dialogVisible" />
  </div>
</template>

<style lang="scss" scoped>
.env-select {
  display: flex;
  align-items: center;
}

.env-btn {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  max-width: 220px;
  height: 28px;
  padding: 0 8px;
  font-size: 12px;
  color: var(--text-color-secondary);
  cursor: pointer;
  background: transparent;
  border: 1px solid var(--chrome-border);
  border-radius: 6px;
  transition:
    color 0.15s ease,
    border-color 0.15s ease;
}

.env-btn:hover {
  color: var(--el-color-primary);
  border-color: var(--el-color-primary);
}

.env-icon {
  flex: 0 0 auto;
}

.env-label {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.env-option {
  margin-right: 12px;
}

.env-option-count {
  float: right;
  font-size: 11px;
  opacity: 0.6;
}
</style>
