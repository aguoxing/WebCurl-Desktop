import { defineStore } from 'pinia'
import { store } from '@/store'
import { setCssVar } from '@/utils'
import { getCssColorVariable, hexToRGB, mix } from '@/utils/color'
import { CACHE_KEY, useCache } from '@/hooks/web/useCache'

const { wsCache } = useCache()

interface AppState {
  pageLoading: boolean
  title: string
  theme: any
  isDark: boolean
  // 侧栏是否被用户手动折叠。桌面程序会记住这个选择，
  // 因此与 isDark 一样落到本地缓存而不是只存在内存里。
  sidebarCollapsed: boolean
}

export const useAppStore = defineStore('app', {
  state: (): AppState => {
    return {
      title: import.meta.env.VITE_APP_TITLE, // 标题
      pageLoading: false, // 路由跳转loading
      isDark: wsCache.get(CACHE_KEY.IS_DARK) || false, // 是否是暗黑模式
      // 首次运行默认折叠：主界面是工作台，它自己已经有一条集合/历史侧栏，
      // 应用级菜单再占满 200px 会让请求区被挤得没法用。用户展开后会被记住。
      sidebarCollapsed: wsCache.get(CACHE_KEY.LAYOUT)?.sidebarCollapsed ?? true,
      theme: wsCache.get(CACHE_KEY.THEME) || {
        // 主题色
        elColorPrimary: '#409eff'
      }
    }
  },
  getters: {
    getPageLoading(): boolean {
      return this.pageLoading
    },
    getTitle(): string {
      return this.title
    },
    getIsDark(): boolean {
      return this.isDark
    },
    getTheme(): any {
      return this.theme
    },
    getSidebarCollapsed(): boolean {
      return this.sidebarCollapsed
    }
  },
  actions: {
    setPageLoading(pageLoading: boolean) {
      this.pageLoading = pageLoading
    },
    setTitle(title: string) {
      this.title = title
    },
    toggleSidebar() {
      this.setSidebarCollapsed(!this.sidebarCollapsed)
    },
    setSidebarCollapsed(collapsed: boolean) {
      this.sidebarCollapsed = collapsed
      wsCache.set(CACHE_KEY.LAYOUT, { sidebarCollapsed: collapsed })
    },
    setIsDark(isDark: boolean) {
      this.isDark = isDark
      if (this.isDark) {
        document.documentElement.classList.add('dark')
        document.documentElement.classList.remove('light')
      } else {
        document.documentElement.classList.add('light')
        document.documentElement.classList.remove('dark')
      }
      wsCache.set(CACHE_KEY.IS_DARK, this.isDark)
      this.setPrimaryLight()
    },
    setPrimaryLight() {
      if (this.theme.elColorPrimary) {
        const elColorPrimary = this.theme.elColorPrimary
        const color = this.isDark ? '#000000' : '#ffffff'
        const lightList = [3, 5, 7, 8, 9]
        lightList.forEach((v) => {
          setCssVar(`--el-color-primary-light-${v}`, mix(color, elColorPrimary, v / 10))
        })
        setCssVar(`--el-color-primary-dark-2`, mix(color, elColorPrimary, 0.2))

        this.setAllColorRgbVars()
      }
    },

    // 处理element自带的主题色和辅助色的-rgb切换主题变化，如：--el-color-primary-rgb
    setAllColorRgbVars() {
      // 需要处理的颜色类型列表
      const colorTypes = ['primary', 'success', 'warning', 'danger', 'error', 'info']

      colorTypes.forEach((type) => {
        // 获取当前颜色值
        const colorValue = getCssColorVariable(`--el-color-${type}`)
        if (colorValue) {
          // 转换为rgba并提取RGB部分
          const rgbaString = hexToRGB(colorValue, 1)
          const rgbValues = rgbaString.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/i)
          if (rgbValues) {
            const [, r, g, b] = rgbValues
            // 设置对应的RGB变量
            setCssVar(`--el-color-${type}-rgb`, `${r}, ${g}, ${b}`)
          }
        }
      })
    }
  }
})

export const useAppStoreWithOut = () => {
  return useAppStore(store)
}
