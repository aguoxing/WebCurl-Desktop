<script lang="ts" setup>
import { computed } from 'vue'
import type { Auth, AuthKind, RequestSpec } from '@/api/types'

/**
 * 认证面板。
 *
 * 把 Basic / Bearer / API-Key 做成显式选项而不是让用户在 Headers 里手写：
 * Basic 需要 base64 编码、API-Key 可能放 query——这些都是"记不住又容易做错"
 * 的细节，交给后端按规范拼装比让用户自己拼更可靠。
 */
const props = defineProps<{ request: RequestSpec }>()
const emit = defineEmits<{ update: [Partial<RequestSpec>] }>()

const auth = computed<Auth>(() => props.request.auth ?? { kind: 'none' })
const kind = computed<AuthKind>(() => auth.value.kind ?? 'none')

const KINDS: { value: AuthKind; label: string; tip: string }[] = [
  { value: 'none', label: '无认证', tip: '不附加任何认证信息。' },
  {
    value: 'basic',
    label: 'Basic Auth',
    tip: '后端会按 RFC 7617 把 用户名:密码 做 Base64 后写入 Authorization 头。'
  },
  {
    value: 'bearer',
    label: 'Bearer Token',
    tip: '后端会写入 Authorization: Bearer <token>。'
  },
  {
    value: 'api-key',
    label: 'API Key',
    tip: '可放在请求头或查询参数里；放在查询参数时会被拼进最终 URL。'
  }
]

const currentTip = computed(() => KINDS.find((k) => k.value === kind.value)?.tip ?? '')

function setKind(value: AuthKind) {
  emit('update', { auth: { ...auth.value, kind: value } })
}

function set(patch: Partial<Auth>) {
  emit('update', { auth: { ...auth.value, ...patch } })
}
</script>

<template>
  <div class="panel">
    <div class="panel-body">
      <div class="field-row">
        <div class="field-label">认证方式</div>
        <div class="field-control">
          <ElSelect :model-value="kind" style="width: 220px" @update:model-value="setKind">
            <ElOption v-for="k in KINDS" :key="k.value" :label="k.label" :value="k.value" />
          </ElSelect>
        </div>
      </div>
      <div class="field-tip">{{ currentTip }}</div>

      <template v-if="kind === 'basic'">
        <div class="field-row">
          <div class="field-label">用户名</div>
          <div class="field-control">
            <ElInput
              :model-value="auth.username ?? ''"
              placeholder="username"
              @update:model-value="(v: string) => set({ username: v })"
            />
          </div>
        </div>
        <div class="field-row">
          <div class="field-label">密码</div>
          <div class="field-control">
            <ElInput
              :model-value="auth.password ?? ''"
              type="password"
              show-password
              placeholder="password"
              @update:model-value="(v: string) => set({ password: v })"
            />
          </div>
        </div>
      </template>

      <template v-else-if="kind === 'bearer'">
        <div class="field-row">
          <div class="field-label">Token</div>
          <div class="field-control">
            <ElInput
              :model-value="auth.token ?? ''"
              placeholder="粘贴 token，不需要写 Bearer 前缀"
              @update:model-value="(v: string) => set({ token: v })"
            />
          </div>
        </div>
      </template>

      <template v-else-if="kind === 'api-key'">
        <div class="field-row">
          <div class="field-label">键名</div>
          <div class="field-control">
            <ElInput
              :model-value="auth.apiKeyName ?? ''"
              placeholder="例如 X-API-Key 或 api_key"
              @update:model-value="(v: string) => set({ apiKeyName: v })"
            />
          </div>
        </div>
        <div class="field-row">
          <div class="field-label">键值</div>
          <div class="field-control">
            <ElInput
              :model-value="auth.apiKeyValue ?? ''"
              placeholder="密钥内容"
              @update:model-value="(v: string) => set({ apiKeyValue: v })"
            />
          </div>
        </div>
        <div class="field-row">
          <div class="field-label">位置</div>
          <div class="field-control">
            <ElRadioGroup
              :model-value="auth.paramIn ?? 'header'"
              @update:model-value="(v: any) => set({ paramIn: v })"
            >
              <ElRadio value="header">请求头</ElRadio>
              <ElRadio value="query">查询参数</ElRadio>
            </ElRadioGroup>
          </div>
        </div>
      </template>
    </div>

    <div class="panel-foot">
      <div class="foot-left">
        <span class="count">在 Headers 里手写的同名头会覆盖这里的设置</span>
      </div>
      <div class="preview">
        <span class="preview-label">当前</span>
        <span class="preview-url">{{ KINDS.find((k) => k.value === kind)?.label }}</span>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use './panel.scss';
</style>
