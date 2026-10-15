<script lang="ts" setup>
import { computed } from 'vue'
import type { KV, RequestSpec } from '@/api/types'
import { composeURL } from '@/utils/curl'
import KeyValueEditor from './KeyValueEditor.vue'

/**
 * 查询参数面板。
 *
 * 参数不在地址栏里编辑而是在这里成对编辑，同时把"实际会请求的 URL"显示出来：
 * 前者让增删改一目了然，后者保证用户始终知道最终发出去的是什么——
 * 少了后者，参数面板就成了一个"看不见结果的黑箱"。
 */
const props = defineProps<{ request: RequestSpec }>()
const emit = defineEmits<{ update: [Partial<RequestSpec>] }>()

const params = computed(() => props.request.params ?? [])

/** 只看启用的行，这样"关掉某一行"的效果能立刻在预览里体现。 */
const effectiveCount = computed(() => params.value.filter((p) => p.enabled && p.name).length)

const finalURL = computed(() => composeURL(props.request))

function update(patch: KV[]) {
  emit('update', { params: patch })
}

function removeEmpty() {
  update(params.value.filter((p) => p.name || p.value))
}

function clearAll() {
  update([])
}
</script>

<template>
  <div class="panel">
    <div class="panel-body">
      <KeyValueEditor
        :model-value="params"
        name-placeholder="参数名"
        value-placeholder="参数值"
        @update:model-value="update"
      />
    </div>

    <div class="panel-foot">
      <div class="foot-left">
        <ElButton link size="small" @click="removeEmpty">清理空行</ElButton>
        <ElButton link size="small" :disabled="!params.length" @click="clearAll">全部清空</ElButton>
        <span class="count">已启用 {{ effectiveCount }} 项</span>
      </div>
      <div class="preview" :title="finalURL">
        <span class="preview-label">实际请求</span>
        <span class="preview-url">{{ finalURL || '—' }}</span>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use './panel.scss';
</style>
