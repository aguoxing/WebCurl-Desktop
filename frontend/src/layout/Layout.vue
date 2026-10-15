<script lang="ts" setup>
import { useRouter } from 'vue-router'
import { AppBar } from '@/layout/components/AppBar'
import { Sidebar } from '@/layout/components/Sidebar'
import { StatusBar } from '@/layout/components/StatusBar'
import { menuRoutes } from '@/router'
import { NativeEvent, useNativeEvent } from '@/hooks/event/useNativeEvent'

defineOptions({ name: 'Layout' })

const router = useRouter()

// KeepAlive 的 include 匹配的是组件名，与路由 name 一一对应
// （约定见 router/routes.ts）。未被标记 keepAlive 的页面每次进入都是新实例。
const cachedViews = menuRoutes
  .filter((item) => item.meta?.keepAlive)
  .map((item) => item.name as string)

// 原生菜单里的「设置」「关于」要驱动前端跳转。订阅放在 Layout 而不是 main.ts：
// 只有 Layout 挂载了才说明路由与视图都已就绪，此时跳转才有落点。
useNativeEvent<string>(NativeEvent.Navigate, (path) => {
  if (path && path !== router.currentRoute.value.path) {
    router.push(path)
  }
})
</script>

<template>
  <div class="v-layout">
    <AppBar />

    <div class="v-layout-body">
      <Sidebar />

      <main class="v-layout-content">
        <RouterView v-slot="{ Component, route }">
          <Transition name="v-page" mode="out-in">
            <KeepAlive :include="cachedViews">
              <component :is="Component" :key="route.path" />
            </KeepAlive>
          </Transition>
        </RouterView>
      </main>
    </div>

    <StatusBar />
  </div>
</template>

<style lang="scss" scoped>
.v-layout {
  display: flex;
  flex-direction: column;
  height: 100%;
}

/* min-height: 0 是关键：否则子项的滚动容器会被内容撑高，
   侧栏与状态栏被推出视口。 */
.v-layout-body {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  overflow: hidden;
}

.v-layout-content {
  flex: 1 1 auto;
  min-width: 0;
  overflow: auto;
}

/* 页面之间淡入淡出。桌面程序不会像网页那样硬切、
   也不该有明显位移，因此只用透明度且时长压到 150ms。 */
.v-page-enter-active,
.v-page-leave-active {
  transition: opacity 0.15s ease;
}

.v-page-enter-from,
.v-page-leave-to {
  opacity: 0;
}
</style>
