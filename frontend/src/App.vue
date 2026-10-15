<template>
  <RouterView />
</template>

<script lang="ts" setup>
import { CACHE_KEY, useCache } from './hooks/web/useCache'
import { useAppStore } from './store/modules/app'

defineOptions({ name: 'APP' })

const appStore = useAppStore()
const { wsCache } = useCache()

// 主题默认取浅色，而不是跟随系统。
//
// 这是个调试工具，界面长期开着、旁边还有编辑器与终端；浅色底在"看 JSON、
// 对时间戳、读状态码"这类高频动作上更容易长时间阅读。系统深色偏好作为
// 首次启动的默认值会不断把用户拉回深色，因此这里只在用户显式选过之后才沿用。
const setDefaultTheme = () => {
  const cached = wsCache.get(CACHE_KEY.IS_DARK)
  if (cached === null || cached === undefined) {
    appStore.setIsDark(false)
    return
  }
  appStore.setIsDark(cached)
}
setDefaultTheme()
</script>

<style>
html,
body {
  width: 100%;
  height: 100%;
  padding: 0 !important;
  margin: 0;
  overflow: hidden;
  background-color: var(--app-bgc);

  #app {
    width: 100%;
    height: 100%;
  }
}
</style>
