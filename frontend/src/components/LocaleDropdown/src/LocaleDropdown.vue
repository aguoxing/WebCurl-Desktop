<script lang="ts" setup>
import { useLocaleStore } from '@/store/modules/locale'
import { useLocale } from '@/hooks/web/useLocale'
import { useDesign } from '@/hooks/web/useDesign'
import { propTypes } from '@/utils/propTypes'

defineOptions({ name: 'LocaleDropdown' })

const { getPrefixCls } = useDesign()

const prefixCls = getPrefixCls('locale-dropdown')

defineProps({
  color: propTypes.string.def('')
})

const localeStore = useLocaleStore()

const langMap = computed(() => localeStore.getLocaleMap)

const currentLang = computed(() => localeStore.getCurrentLocale)

// 直接用 ref 存储当前选中的语言值
const selectedLang = ref(currentLang.value?.lang || 'zh_CN')

// 监听 currentLang 变化，同步更新 selectedLang
watch(
  currentLang,
  (newVal) => {
    if (newVal?.lang) {
      selectedLang.value = newVal.lang
    }
  },
  { immediate: true }
)

const setLang = async (lang: string) => {
  // 根据语言代码找到对应的语言对象
  const langObj = langMap.value.find((item) => item.lang === lang)
  if (!langObj) return

  // 如果语言没变化，直接返回
  if (lang === currentLang.value?.lang) return

  try {
    // 1. 先更新 store 中的语言
    localeStore.setCurrentLocale({
      lang: langObj.lang
    })

    // 2. 调用 changeLocale 更新 i18n 实例
    const { changeLocale } = useLocale()
    await changeLocale(langObj.lang)

    // 3. 语言切换成功后，刷新页面重新加载
    setTimeout(() => {
      window.location.reload()
    }, 100)
  } catch (error) {
    console.error('切换语言失败:', error)
    // 如果切换失败，回退到原来的语言
    selectedLang.value = currentLang.value?.lang || 'zh_CN'
  }
}
</script>

<template>
  <ElSelect v-model="selectedLang" :class="prefixCls" @change="setLang" style="width: 200px">
    <ElOption v-for="item in langMap" :key="item.lang" :label="item.name" :value="item.lang" />
  </ElSelect>
</template>
