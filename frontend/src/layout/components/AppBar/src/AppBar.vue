<script lang="ts" setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Setting } from '@element-plus/icons-vue'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { EnvironmentSelect } from '@/components/EnvironmentSelect'
import { useLocaleStore } from '@/store/modules/locale'
import { useLocale } from '@/hooks/web/useLocale'
import { useI18n } from '@/hooks/web/useI18n'

defineOptions({ name: 'AppBar' })

const { t } = useI18n()

const route = useRoute()
const router = useRouter()

const localeStore = useLocaleStore()

// 面包屑等价物：桌面程序的应用栏通常只显示"当前在哪"，层级感由侧栏提供。
const pageTitle = computed(() => {
  const title = route.meta?.title as string | undefined
  return title ? t(title) : ''
})

/** 环境切换器只在工作台里显示：其它页面的请求不经过变量替换，放了会误导。 */
const showEnvSelect = computed(() => route.path.startsWith('/workbench'))

const currentLang = computed(() => localeStore.getCurrentLocale?.lang ?? 'zh-CN')
const currentLangLabel = computed(
  () => localeStore.getLocaleMap.find((l) => l.lang === currentLang.value)?.name ?? '简体中文'
)

/**
 * 切换界面语言。
 *
 * 这里沿用模板里 LocaleDropdown 的做法：改完立即 reload。
 * 原因是原生菜单文案、Element Plus 的 locale 都在各自的初始化路径上读取语言，
 * 热更新覆盖不全——重载一次比留一堆"部分界面没跟着变"的边角问题更划算。
 */
async function setLang(lang: string) {
  if (lang === currentLang.value) return
  const target = localeStore.getLocaleMap.find((l) => l.lang === lang)
  if (!target) return
  localeStore.setCurrentLocale({ lang: target.lang })
  const { changeLocale } = useLocale()
  await changeLocale(target.lang)
  setTimeout(() => window.location.reload(), 100)
}

// macOS 的窗口使用 MacTitleBarHiddenInset，系统红绿灯浮在内容之上，
// 应用栏左侧必须让出位置，否则标题会被压住。
//
// 这里用 UA 而不是 runtime 的 System.Environment()：后者是异步的，
// 首帧拿不到值会让整条应用栏先渲染完再跳一下；而 WKWebView 的 UA
// 始终包含 "Macintosh"，同步且可靠。
const isMacOS = /Macintosh|Mac OS X/.test(navigator.userAgent)

const openSetting = () => {
  if (route.path !== '/setting') {
    router.push('/setting')
  }
}
</script>

<template>
  <header class="v-appbar" :class="{ 'is-macos': isMacOS }">
    <span class="v-appbar-title">{{ pageTitle }}</span>

    <div class="v-appbar-actions">
      <EnvironmentSelect v-if="showEnvSelect" />

      <ElDropdown trigger="click" placement="bottom-end" @command="setLang">
        <button class="v-appbar-action is-lang" type="button" :title="t('appbar.language')">
          {{ currentLangLabel }}
        </button>
        <template #dropdown>
          <ElDropdownMenu>
            <ElDropdownItem
              v-for="item in localeStore.getLocaleMap"
              :key="item.lang"
              :command="item.lang"
              :disabled="item.lang === currentLang"
            >
              {{ item.name }}
            </ElDropdownItem>
          </ElDropdownMenu>
        </template>
      </ElDropdown>

      <ThemeSwitch />

      <button
        class="v-appbar-action"
        type="button"
        :title="t('router.setting')"
        @click="openSetting"
      >
        <ElIcon><Setting /></ElIcon>
      </button>
    </div>
  </header>
</template>

<style lang="scss" scoped>
.v-appbar {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  height: 44px;
  padding: 0 12px;
  background-color: var(--chrome-bgc);
  border-bottom: 1px solid var(--chrome-border);
}

/* 让开 macOS 红绿灯。50px 是 window.go 里 InvisibleTitleBarHeight 的值，
   红绿灯垂直居中于其中，因此应用栏的可用起点在 70px 之后。 */
.v-appbar.is-macos {
  padding-left: 78px;
}

.v-appbar-title {
  flex: 0 0 auto;
  font-size: 14px;
  font-weight: 500;
  color: var(--text-color);
}

.v-appbar-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 10px;
}

.v-appbar-action {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  color: var(--text-color-secondary);
  cursor: pointer;
  background: transparent;
  border: none;
  border-radius: 6px;
  transition:
    color 0.15s ease,
    background-color 0.15s ease;
}

/* 语言按钮是文字不是图标，宽度由内容决定。 */
.v-appbar-action.is-lang {
  width: auto;
  padding: 0 8px;
  font-size: 12px;
}

.v-appbar-action:hover {
  color: var(--el-color-primary);
  background-color: var(--el-fill-color-light);
}
</style>
