<script lang="ts" setup>
import { useAppStore } from '@/store/modules/app'
import { useIcon } from '@/hooks/web/useIcon'
import { useDesign } from '@/hooks/web/useDesign'

defineOptions({ name: 'ThemeSwitch' })

const { getPrefixCls } = useDesign()

const prefixCls = getPrefixCls('theme-switch')

const Sun = useIcon({ icon: 'ep:sunny', color: '#606266' })

const CrescentMoon = useIcon({ icon: 'ep:moon', color: '#606266' })

const appStore = useAppStore()

// 初始化获取是否是暗黑主题
const isDark = ref(appStore.getIsDark)

const themeChange = (val: boolean) => {
  appStore.setIsDark(val)
}
</script>

<template>
  <ElSwitch
    v-model="isDark"
    :active-action-icon="CrescentMoon"
    :class="prefixCls"
    :inactive-action-icon="Sun"
    @change="themeChange"
  />
</template>
