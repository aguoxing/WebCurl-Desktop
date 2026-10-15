import type { RouteRecordRaw } from 'vue-router'

export const Layout = () => import('@/layout/Layout.vue')

// ⚠️ 注意：meta.title 存储的是 i18n key，不要在模块顶层调用 useI18n().t()
// useTitle 会在路由切换时延迟解析翻译
//
// meta 同时是侧栏菜单、页面标题与 KeepAlive 白名单的**唯一来源**：
// 新增一个页面只需在 menuRoutes 里加一条，侧栏会自动出现对应菜单项。
//
//	title     页面标题的 i18n key，同时作为侧栏菜单项文案
//	icon      Element Plus 图标组件名（@element-plus/icons-vue 的导出名）
//	keepAlive 是否缓存页面实例；缺省不缓存
//	hidden    恒不在侧栏显示，但仍可通过路由访问（详情页、全屏页用）
//
// 关于 keepAlive：KeepAlive 的 include 匹配的是**组件名**，而 SFC 推断出的名字
// 是文件名（这些页面都叫 index），因此约定「路由 name === 页面组件的
// defineOptions name」，改动其中一处必须同步另一处。
//
// 工作台与 Mock 都挂 keepAlive：两者的状态（标签页、编辑中的请求、日志列表）
// 都在 Pinia 里，但滚动位置与展开的集合在组件本地状态里，切走再回来应该保持原样。
const menuRoutes: RouteRecordRaw[] = [
  {
    path: '/workbench',
    component: () => import('@/views/workbench/index.vue'),
    name: 'Workbench',
    meta: { title: 'router.workbench', icon: 'Connection', keepAlive: true }
  },
  {
    path: '/mock',
    component: () => import('@/views/mock/index.vue'),
    name: 'Mock',
    meta: { title: 'router.mock', icon: 'Platform', keepAlive: true }
  },
  {
    path: '/setting',
    component: () => import('@/views/setting/index.vue'),
    name: 'Setting',
    meta: { title: 'router.setting', icon: 'Setting', keepAlive: true }
  },
  {
    path: '/about',
    component: () => import('@/views/about/index.vue'),
    name: 'About',
    meta: { title: 'router.about', icon: 'InfoFilled', keepAlive: true }
  }
]

const routes: RouteRecordRaw[] = [
  {
    path: '',
    component: Layout,
    // 打开应用就落在工作台：这是本应用的主界面，其余页面都是辅助。
    redirect: '/workbench',
    children: menuRoutes
  }
]

export { menuRoutes, routes }
