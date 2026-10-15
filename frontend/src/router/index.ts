import { createRouter, createWebHashHistory } from 'vue-router'
import { routes } from './routes'

// 路由定义在 routes.ts（纯数据，无副作用），此处只负责创建实例。
// 拆开是为了让侧栏等布局组件能只依赖路由数据，不必连带引入 router 实例。
export { Layout, menuRoutes } from './routes'

// history 必须保持 hash 模式：页面由 Wails 的内嵌资源服务加载，
// history 模式下刷新或深链会落到服务端路径上直接 404。
export const router = createRouter({ history: createWebHashHistory(), routes })
