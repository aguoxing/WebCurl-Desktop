<script lang="ts" setup>
import { onMounted, ref } from 'vue'
import { GetVersion } from '@/bindings/webcurl-desktop/internal/binding/updateservicebinding'
import { useI18n } from '@/hooks/web/useI18n'

defineOptions({ name: 'StatusBar' })

const { t } = useI18n()

// 版本号来自二进制自身的构建注入（见 internal/version），
// 因此状态栏显示的是"正在运行的这一份"，与配置文件无关。
const version = ref('')

onMounted(async () => {
  try {
    version.value = await GetVersion()
  } catch (error) {
    // 在浏览器里单独预览前端时没有 Wails 运行时，取不到版本号；
    // 状态栏少一项不是错误，不打扰用户。
    console.debug('[statusbar] version unavailable:', error)
  }
})
</script>

<template>
  <footer class="v-statusbar">
    <span>{{ t('statusbar.ready') }}</span>
    <span v-if="version">v{{ version }}</span>
  </footer>
</template>

<style lang="scss" scoped>
.v-statusbar {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  height: 26px;
  padding: 0 12px;
  font-size: 12px;
  color: var(--text-color-secondary);
  background-color: var(--chrome-bgc);
  border-top: 1px solid var(--chrome-border);
  user-select: none;
}
</style>
