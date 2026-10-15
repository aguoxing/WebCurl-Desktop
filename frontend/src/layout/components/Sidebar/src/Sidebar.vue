<script lang="ts" setup>
import { computed } from 'vue'
import type { Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMediaQuery } from '@vueuse/core'
import * as ElIcons from '@element-plus/icons-vue'
import { Expand, Fold } from '@element-plus/icons-vue'
import { menuRoutes } from '@/router'
import { useAppStore } from '@/store/modules/app'
import { useI18n } from '@/hooks/web/useI18n'

defineOptions({ name: 'Sidebar' })

const { t } = useI18n()

const appStore = useAppStore()
const route = useRoute()
const router = useRouter()

// 窗口变窄时自动折叠，避免内容区被侧栏挤扁；用户的手动折叠与自动折叠取或，
// 这样放大窗口后用户之前的选择仍然保留。
const isNarrow = useMediaQuery('(max-width: 1024px)')
const collapsed = computed(() => appStore.getSidebarCollapsed || isNarrow.value)

// 图标名来自路由 meta（运行时数据），只能按名字查表而不能写成组件引用。
// 名字不存在时返回 undefined，模板会跳过图标而不是渲染空盒子。
const iconRegistry = ElIcons as unknown as Record<string, Component>
const resolveIcon = (name?: string): Component | undefined =>
  name ? iconRegistry[name] : undefined

const menuItems = computed(() =>
  menuRoutes
    .filter((item) => !item.meta?.hidden)
    .map((item) => ({
      path: item.path,
      title: t(item.meta?.title as string),
      icon: resolveIcon(item.meta?.icon as string)
    }))
)

const onSelect = (index: string) => {
  if (index !== route.path) {
    router.push(index)
  }
}
</script>

<template>
  <aside class="v-sidebar" :class="{ 'is-collapsed': collapsed }">
    <ElMenu
      class="v-sidebar-menu"
      :default-active="route.path"
      :collapse="collapsed"
      :collapse-transition="false"
      @select="onSelect"
    >
      <ElMenuItem v-for="item in menuItems" :key="item.path" :index="item.path">
        <ElIcon v-if="item.icon"><component :is="item.icon" /></ElIcon>
        <template #title>{{ item.title }}</template>
      </ElMenuItem>
    </ElMenu>

    <button
      class="v-sidebar-toggle"
      type="button"
      :title="collapsed ? t('sidebar.expand') : t('sidebar.collapse')"
      @click="appStore.toggleSidebar()"
    >
      <ElIcon><component :is="collapsed ? Expand : Fold" /></ElIcon>
    </button>
  </aside>
</template>

<style lang="scss" scoped>
.v-sidebar {
  display: flex;
  flex: 0 0 auto;
  flex-direction: column;
  width: 200px;
  height: 100%;
  overflow: hidden;
  background-color: var(--chrome-bgc);
  border-right: 1px solid var(--chrome-border);
  transition: width 0.15s ease;
}

/* 64px 与 Element Plus 折叠态的菜单宽度（--el-menu-collapse-width）保持一致，
   否则折叠后会出现一道空白或裁切。 */
.v-sidebar.is-collapsed {
  width: 64px;
}

.v-sidebar-menu {
  flex: 1 1 auto;
  overflow-x: hidden;
  overflow-y: auto;
  border-right: none;
  // 让菜单融进侧栏底色，激活/悬停色仍由 Element Plus 的主题变量决定
  --el-menu-bg-color: transparent;
  --el-menu-border-color: transparent;
}

.v-sidebar-toggle {
  flex: 0 0 auto;
  height: 36px;
  color: var(--text-color-secondary);
  cursor: pointer;
  background: transparent;
  border: none;
  border-top: 1px solid var(--chrome-border);
  transition: color 0.15s ease;
}

.v-sidebar-toggle:hover {
  color: var(--el-color-primary);
}
</style>
