<script lang="ts" setup>
import { computed } from 'vue'
import type { KV, RequestSpec } from '@/api/types'
import KeyValueEditor from './KeyValueEditor.vue'

/**
 * Cookie 面板。
 *
 * 单独成一栏而不是让用户在 Headers 里手写 Cookie 头：一个 Cookie 头里
 * 往往塞着七八个键值对，写在一行里改其中一个几乎是不可能完成的任务。
 * 后端会把这里的内容重新拼成 Cookie 头。
 */
const props = defineProps<{ request: RequestSpec }>()
const emit = defineEmits<{ update: [Partial<RequestSpec>] }>()

const cookies = computed(() => props.request.cookies ?? [])

const enabledCount = computed(() => cookies.value.filter((c) => c.enabled && c.name).length)

const cookieHeader = computed(() =>
  cookies.value
    .filter((c) => c.enabled && c.name)
    .map((c) => `${c.name}=${c.value}`)
    .join('; ')
)

function update(patch: KV[]) {
  emit('update', { cookies: patch })
}

function clearAll() {
  update([])
}
</script>

<template>
  <div class="panel">
    <div class="panel-body">
      <KeyValueEditor
        :model-value="cookies"
        name-placeholder="Cookie 名"
        value-placeholder="值"
        @update:model-value="update"
      />
    </div>

    <div class="panel-foot">
      <div class="foot-left">
        <ElButton link size="small" :disabled="!cookies.length" @click="clearAll"
          >全部清空</ElButton
        >
        <span class="count">已启用 {{ enabledCount }} 项</span>
      </div>
      <div class="preview" :title="cookieHeader">
        <span class="preview-label">Cookie</span>
        <span class="preview-url">{{ cookieHeader || '—' }}</span>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use './panel.scss';
</style>
