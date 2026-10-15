<template>
  <div class="about-container">
    <div class="about-header">
      <img src="/vite.svg" alt="logo" class="about-logo" />
      <h2>{{ appName }}</h2>
      <p class="about-version">Version {{ version }}</p>
    </div>

    <el-card class="about-card">
      <div class="card-content-row">
        <div>
          <div class="card-label">检查更新</div>
          <div class="card-desc">检查是否有新版本可用</div>
        </div>
        <el-button type="primary" :loading="checking" @click="onCheckUpdate">
          {{ checking ? '检查中…' : '检查更新' }}
        </el-button>
      </div>
      <div v-if="message" class="update-message" :class="messageType">{{ message }}</div>
    </el-card>

    <el-card class="about-card">
      <div class="about-info">
        <p><strong>应用名称：</strong>{{ appName }}</p>
        <p><strong>版本号：</strong>{{ version }}</p>
        <p><strong>描述：</strong>{{ description }}</p>
      </div>
    </el-card>

    <el-card class="about-card">
      <div class="about-info">
        <p><strong>Wails 版本：</strong>{{ wailsVersionText }}</p>
        <p><strong>Webview：</strong>{{ webviewText }}</p>
        <p><strong>运行平台：</strong>{{ platformText }}</p>
      </div>
    </el-card>

    <p class="about-copyright">Copyright ©{{ currentYear }} {{ appName }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { GetAllConfig } from '@/bindings/webcurl-desktop/internal/binding/configservicebinding'
import { CheckAndInstall } from '@/bindings/webcurl-desktop/internal/binding/updateservicebinding'
import { GetRuntimeInfo } from '@/bindings/webcurl-desktop/internal/binding/systemservicebinding'

// 组件名必须与路由 name 一致，详见 router/routes.ts 的约定说明。
defineOptions({ name: 'About' })

// 运行环境由 Go 侧探测。各平台能拿到的字段并不一致——macOS 的 WKWebView
// 没有版本号，server 构建则完全没有 webview——因此每一项都用占位符兜底，
// 而不是把空串直接渲染到界面上。
interface RuntimeInfo {
  wailsVersion: string
  webviewName: string
  webviewVersion: string
  os: string
  arch: string
}

const UNKNOWN = '—'

// 版权信息从底部的状态栏搬到这里：状态栏属于"运行时状态"，
// 而版权是静态的关于信息，放在关于页更符合桌面程序的惯例。
const currentYear = new Date().getFullYear()

const appName = ref('')
const version = ref('')
const description = ref('')
const checking = ref(false)
const message = ref('')
const messageType = ref('')
const runtime = ref<RuntimeInfo>({
  wailsVersion: '',
  webviewName: '',
  webviewVersion: '',
  os: '',
  arch: ''
})

const wailsVersionText = computed(() => runtime.value.wailsVersion || UNKNOWN)

const webviewText = computed(() => {
  const { webviewName, webviewVersion } = runtime.value
  if (!webviewName) return UNKNOWN
  // 版本探测不到时只显示实现名（例如 macOS）。
  return webviewVersion ? `${webviewName} ${webviewVersion}` : webviewName
})

const platformText = computed(() => {
  const { os, arch } = runtime.value
  if (!os) return UNKNOWN
  return arch ? `${os} / ${arch}` : os
})

const loadInfo = async () => {
  const cfg = await GetAllConfig()
  appName.value = cfg.app.name
  version.value = cfg.app.version
  description.value = cfg.app.description

  const info = await GetRuntimeInfo()
  runtime.value = info
}

// CheckAndInstall 的返回约定：'' = 已是最新，'restarting' = 更新已就绪即将重启，其余为错误信息
const RESTARTING_CODE = 'restarting'

const onCheckUpdate = async () => {
  checking.value = true
  message.value = ''
  try {
    const result = await CheckAndInstall()
    if (!result) {
      message.value = '已是最新版本'
      messageType.value = 'success'
    } else if (result === RESTARTING_CODE) {
      message.value = '更新已就绪，应用即将重启并完成安装…'
      messageType.value = 'success'
    } else {
      message.value = result
      messageType.value = 'error'
    }
  } finally {
    checking.value = false
  }
}

onMounted(() => {
  loadInfo()
})
</script>

<style lang="scss" scoped>
/* 颜色一律走主题变量（--text-color-secondary / --el-color-*），不写死色值：
   写死的浅灰、浅绿底在暗色主题下会失去对比度。 */
.about-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 30px 10px;
  gap: 20px;
}

.about-header {
  text-align: center;

  .about-logo {
    width: 64px;
    height: 64px;
    margin-bottom: 12px;
  }

  h2 {
    margin: 0;
    font-size: 22px;
  }

  .about-version {
    color: var(--text-color-secondary);
    margin-top: 4px;
  }
}

.about-card {
  width: 100%;
  max-width: 500px;
}

.card-content-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-label {
  font-weight: 600;
}

.card-desc {
  font-size: 13px;
  color: var(--text-color-secondary);
  margin-top: 2px;
}

.update-message {
  margin-top: 12px;
  padding: 8px 12px;
  border-radius: 4px;
  font-size: 14px;

  &.success {
    background: var(--el-color-success-light-9);
    color: var(--el-color-success);
  }

  &.error {
    background: var(--el-color-danger-light-9);
    color: var(--el-color-danger);
  }
}

.about-info p {
  margin: 6px 0;
  font-size: 14px;
}

.about-copyright {
  font-size: 12px;
  color: var(--text-color-secondary);
}
</style>
