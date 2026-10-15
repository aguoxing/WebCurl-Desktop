<script lang="ts" setup>
import { onMounted, ref } from 'vue'
import { configApi } from '@/api'
import type { RequestDefaultsConfig } from '@/api/types'
import { LocaleDropdown } from '@/components/LocaleDropdown'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { useWorkspaceStore } from '@/store/modules/workspace'
import { useMessage } from '@/hooks/web/useMessage'

// 组件名必须与路由 name 一致，详见 router/routes.ts 的约定说明。
defineOptions({ name: 'Setting' })

const { success, error: toastError } = useMessage()
const workspace = useWorkspaceStore()

/**
 * 设置页只放**全局默认值**。
 *
 * 单个请求的超时、重试、跳过证书校验在请求自己的「选项」面板里改；
 * 这里改的是"以后新建的请求默认带什么"。两者刻意分开，
 * 否则调试某个接口时的临时放宽会顺手污染所有后续请求。
 */
const request = ref<RequestDefaultsConfig>({
  defaultTimeoutSec: 30,
  defaultRetryCount: 0,
  defaultRetryDelaySec: 1,
  followRedirect: 1,
  verifySSL: 1,
  historyLimit: 500
})

const mock = ref({ enabled: 1, configPath: '' })
const storage = ref({ dir: '' })

const allConfig = ref({
  startup: { enabled: 0 },
  update: { autoCheck: 0, provider: '', apiUrl: '' },
  tray: { closeBehaviour: 'minimize' }
})

/** 备份过的原始值，用于「还原」按钮。 */
const snapshot = ref('')

async function load() {
  const [req, mk, st, all] = await Promise.all([
    configApi.getRequest(),
    configApi.getMock(),
    configApi.getStorage(),
    configApi.getAll()
  ])
  request.value = { ...request.value, ...(req as RequestDefaultsConfig) }
  mock.value = { ...mock.value, ...(mk as typeof mock.value) }
  storage.value = { ...storage.value, ...(st as typeof storage.value) }
  allConfig.value = {
    startup: all.startup ?? allConfig.value.startup,
    update: { ...allConfig.value.update, ...(all.update ?? {}) },
    tray: { ...allConfig.value.tray, ...(all.tray ?? {}) }
  }
  snapshot.value = JSON.stringify({
    request: request.value,
    mock: mock.value,
    storage: storage.value
  })
}

function restore() {
  if (!snapshot.value) return
  const parsed = JSON.parse(snapshot.value) as {
    request: RequestDefaultsConfig
    mock: typeof mock.value
    storage: typeof storage.value
  }
  request.value = parsed.request
  mock.value = parsed.mock
  storage.value = parsed.storage
  success('已还原为上次读取到的值（还需要点保存才会写盘）')
}

async function saveRequest() {
  try {
    await configApi.updateRequest(request.value)
    // 立刻让工作区知道新的默认值，否则新建的请求还是旧的一套。
    await workspace.loadDefaults()
    success('请求默认值已保存')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

async function saveMock() {
  try {
    await configApi.updateMock(mock.value)
    success('Mock 配置已保存；改动在下次启动 Mock 引擎或重启应用后生效')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

async function saveStorage() {
  try {
    await configApi.updateStorage(storage.value)
    success('数据目录已保存；重启应用后在新目录生效')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

async function onStartupChange() {
  try {
    await configApi.updateStartup(allConfig.value.startup)
    success(allConfig.value.startup.enabled ? '已开启开机自启' : '已关闭开机自启')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

async function onUpdateChange() {
  try {
    await configApi.updateUpdate(allConfig.value.update)
    success('更新设置已保存')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

async function onTrayChange() {
  try {
    await configApi.updateTray(allConfig.value.tray)
    success('托盘行为已保存')
  } catch (err) {
    toastError(err instanceof Error ? err.message : String(err))
  }
}

onMounted(async () => {
  await load()
  // 数据目录那一栏要显示"当前实际使用的位置"，它来自工作区快照而不是配置。
  await workspace.load()
})
</script>

<template>
  <div class="setting-container">
    <section class="block">
      <h3 class="block-title">常规</h3>
      <ElCard shadow="never">
        <div class="row">
          <span class="row-label">界面语言</span>
          <LocaleDropdown />
        </div>
        <ElDivider />
        <div class="row">
          <span class="row-label">主题模式</span>
          <ThemeSwitch />
        </div>
      </ElCard>
    </section>

    <section class="block">
      <h3 class="block-title">新建请求的默认值</h3>
      <ElCard shadow="never">
        <p class="block-tip">
          这里只影响**以后新建**的请求。已保存的请求各自带着自己的选项，
          在请求面板的「选项」页签里修改。
        </p>
        <ElDivider />

        <div class="row">
          <span class="row-label">默认超时</span>
          <div class="row-value">
            <ElInputNumber v-model="request.defaultTimeoutSec" :min="0" :max="600" :step="5" />
            <span class="hint">秒（0 = 不限制）</span>
          </div>
        </div>
        <ElDivider />
        <div class="row">
          <span class="row-label">默认重试次数</span>
          <div class="row-value">
            <ElInputNumber v-model="request.defaultRetryCount" :min="0" :max="10" />
            <span class="hint">网络错误或 5xx 时重试</span>
          </div>
        </div>
        <ElDivider />
        <div class="row">
          <span class="row-label">默认重试间隔</span>
          <div class="row-value">
            <ElInputNumber v-model="request.defaultRetryDelaySec" :min="0" :max="60" :step="0.5" />
            <span class="hint">秒</span>
          </div>
        </div>
        <ElDivider />
        <div class="row">
          <span class="row-label">默认跟随重定向</span>
          <ElSwitch v-model="request.followRedirect" :active-value="1" :inactive-value="0" />
        </div>
        <ElDivider />
        <div class="row">
          <span class="row-label">默认校验 TLS 证书</span>
          <div class="row-value">
            <ElSwitch v-model="request.verifySSL" :active-value="1" :inactive-value="0" />
            <span class="hint">关掉后自签证书可用，但中间人攻击也无法识别</span>
          </div>
        </div>
        <ElDivider />
        <div class="row">
          <span class="row-label">历史保留条数</span>
          <div class="row-value">
            <ElInputNumber v-model="request.historyLimit" :min="50" :max="10000" :step="50" />
            <span class="hint">超出后丢弃最旧的记录</span>
          </div>
        </div>

        <div class="actions">
          <ElButton @click="restore">还原</ElButton>
          <ElButton type="primary" @click="saveRequest">保存</ElButton>
        </div>
      </ElCard>
    </section>

    <section class="block">
      <h3 class="block-title">Mock 引擎</h3>
      <ElCard shadow="never">
        <div class="row">
          <span class="row-label">随应用启动</span>
          <div class="row-value">
            <ElSwitch v-model="mock.enabled" :active-value="1" :inactive-value="0" />
            <span class="hint">关闭后只能在 Mock 页面手动启动</span>
          </div>
        </div>
        <ElDivider />
        <div class="row">
          <span class="row-label">配置文件路径</span>
          <ElInput
            v-model="mock.configPath"
            class="wide"
            placeholder="留空 = 数据目录下的 mock.json"
          />
        </div>
        <div class="actions">
          <ElButton type="primary" @click="saveMock">保存</ElButton>
        </div>
      </ElCard>
    </section>

    <section class="block">
      <h3 class="block-title">数据存储</h3>
      <ElCard shadow="never">
        <div class="row">
          <span class="row-label">数据目录</span>
          <ElInput
            v-model="storage.dir"
            class="wide"
            placeholder="留空 = 用户配置目录下的 webcurl-desktop/data"
          />
        </div>
        <div class="row">
          <span class="row-label" />
          <div class="row-value">
            <span class="hint">当前使用：{{ workspace.dataPath.dir || '—' }}</span>
            <ElButton link type="primary" @click="workspace.openDataDir()">打开目录</ElButton>
          </div>
        </div>
        <div class="actions">
          <ElButton type="primary" @click="saveStorage">保存</ElButton>
        </div>
      </ElCard>
    </section>

    <section class="block">
      <h3 class="block-title">应用</h3>
      <ElCard shadow="never">
        <div class="row">
          <span class="row-label">开机时自动启动</span>
          <ElSwitch
            v-model="allConfig.startup.enabled"
            :active-value="1"
            :inactive-value="0"
            @change="onStartupChange"
          />
        </div>
        <ElDivider />
        <div class="row">
          <span class="row-label">启动时检查更新</span>
          <ElSwitch
            v-model="allConfig.update.autoCheck"
            :active-value="1"
            :inactive-value="0"
            @change="onUpdateChange"
          />
        </div>
        <ElDivider />
        <div class="row">
          <span class="row-label">更新源</span>
          <ElSelect
            v-model="allConfig.update.provider"
            style="width: 200px"
            @change="onUpdateChange"
          >
            <ElOption label="关闭" value="" />
            <ElOption label="GitHub" value="github" />
            <ElOption label="自定义 API" value="custom" />
          </ElSelect>
        </div>
        <template v-if="allConfig.update.provider">
          <ElDivider />
          <div class="row">
            <span class="row-label">
              {{ allConfig.update.provider === 'github' ? '仓库（owner/repo）' : '更新接口地址' }}
            </span>
            <ElInput v-model="allConfig.update.apiUrl" class="wide" @change="onUpdateChange" />
          </div>
        </template>
        <ElDivider />
        <div class="row">
          <span class="row-label">关闭主窗口时</span>
          <ElSelect
            v-model="allConfig.tray.closeBehaviour"
            style="width: 200px"
            @change="onTrayChange"
          >
            <ElOption label="退出程序" value="quit" />
            <ElOption label="最小化到托盘" value="minimize" />
          </ElSelect>
        </div>
      </ElCard>
    </section>
  </div>
</template>

<style lang="scss" scoped>
.setting-container {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-width: 860px;
  padding: 12px 16px 24px;
  margin: 0 auto;
}

.block-title {
  margin: 14px 0 8px;
  font-size: 15px;
  font-weight: 600;
  color: var(--text-color);
}

.block-tip {
  font-size: 12px;
  line-height: 1.8;
  color: var(--text-color-secondary);
}

.row {
  display: flex;
  gap: 16px;
  align-items: center;
  justify-content: space-between;
  min-height: 32px;
}

.row-label {
  flex: 0 0 auto;
  font-size: 13px;
  color: var(--text-color);
}

.row-value {
  display: flex;
  flex: 0 0 auto;
  gap: 8px;
  align-items: center;
}

.hint {
  font-size: 12px;
  color: var(--text-color-secondary);
}

.wide {
  width: 420px;
  max-width: 60vw;
}

/* 保存按钮靠右而不是铺满：设置页的每一块都是"改完就存"，
   右下角的位置与多数桌面程序的习惯一致。 */
.actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
