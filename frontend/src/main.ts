import { createApp } from 'vue'
import App from './App.vue'

import 'virtual:svg-icons-register'
import { router } from '@/router'
import { setupElementPlus } from '@/plugins/elementPlus'
import { setupStore } from '@/store'
import { setupI18n } from '@/plugins/vueI18n'
import { emitNative, NativeEvent } from '@/hooks/event/useNativeEvent'
import { useLocaleStoreWithOut } from '@/store/modules/locale'

import '@/styles/index.scss'

import './permission'

// 创建实例
const setupAll = async () => {
  const app = createApp(App)
  await setupI18n(app)

  setupStore(app)

  setupElementPlus(app)

  app.use(router)

  app.mount('#app')

  // 原生菜单的文案由 Go 侧持有，而界面语言只有前端知道，因此挂载后立即同步一次；
  // 少了这一步，菜单会一直停在 Go 侧的默认语言上。
  // 切换语言会重新加载页面（见 LocaleDropdown），这条路径会再跑一遍，无需额外接线。
  emitNative(NativeEvent.Language, useLocaleStoreWithOut().getCurrentLocale.lang)
}

setupAll()
