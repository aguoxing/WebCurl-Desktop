<script lang="ts" setup>
import { computed } from 'vue'
import type { RequestSpec } from '@/api/types'

/**
 * 单个请求的选项面板。
 *
 * 这里的每一项都**只作用于当前这个请求**，修改后随请求一起保存到集合或历史里；
 * 全局默认值在设置页。两者刻意分开：调试某个接口时临时把超时调到 120 秒，
 * 不该顺带改掉以后所有新建请求的默认值。
 */
const props = defineProps<{ request: RequestSpec }>()
const emit = defineEmits<{ update: [Partial<RequestSpec>] }>()

function set(patch: Partial<RequestSpec>) {
  emit('update', patch)
}

const ENCODINGS = ['', 'gzip', 'gzip, deflate', 'gzip, deflate, br', 'identity']
</script>

<template>
  <div class="panel">
    <div class="panel-body">
      <div class="section">
        <div class="section-title">超时与重试</div>

        <div class="field-row">
          <div class="field-label">超时</div>
          <div class="field-control">
            <ElInputNumber
              :model-value="request.timeoutSec ?? 0"
              :min="0"
              :max="600"
              :step="5"
              controls-position="right"
              style="width: 160px"
              @update:model-value="(v: number | undefined) => set({ timeoutSec: v ?? 0 })"
            />
            <span class="unit">秒（0 表示不限制）</span>
          </div>
        </div>

        <div class="field-row">
          <div class="field-label">重试次数</div>
          <div class="field-control">
            <ElInputNumber
              :model-value="request.retryCount ?? 0"
              :min="0"
              :max="10"
              controls-position="right"
              style="width: 160px"
              @update:model-value="(v: number | undefined) => set({ retryCount: v ?? 0 })"
            />
            <span class="unit">仅在网络错误或 5xx 时重试</span>
          </div>
        </div>

        <div class="field-row">
          <div class="field-label">重试间隔</div>
          <div class="field-control">
            <ElInputNumber
              :model-value="request.retryDelaySec ?? 0"
              :min="0"
              :max="60"
              :step="0.5"
              controls-position="right"
              style="width: 160px"
              @update:model-value="(v: number | undefined) => set({ retryDelaySec: v ?? 0 })"
            />
            <span class="unit">秒</span>
          </div>
        </div>
      </div>

      <div class="section">
        <div class="section-title">连接</div>

        <div class="field-row">
          <div class="field-label">跳过证书校验</div>
          <div class="field-control">
            <ElSwitch
              :model-value="request.skipSslVerify ?? false"
              @update:model-value="(v: any) => set({ skipSslVerify: Boolean(v) })"
            />
            <span class="unit">仅用于自签名证书的测试环境</span>
          </div>
        </div>

        <div class="field-row">
          <div class="field-label">不跟随重定向</div>
          <div class="field-control">
            <ElSwitch
              :model-value="request.noFollowRedirect ?? false"
              @update:model-value="(v: any) => set({ noFollowRedirect: Boolean(v) })"
            />
            <span class="unit">打开后 3xx 会被当作最终响应返回</span>
          </div>
        </div>

        <div class="field-row">
          <div class="field-label">代理</div>
          <div class="field-control">
            <ElInput
              :model-value="request.proxyUrl ?? ''"
              placeholder="http://127.0.0.1:7890（留空表示直连）"
              @update:model-value="(v: string) => set({ proxyUrl: v })"
            />
          </div>
        </div>

        <div class="field-row">
          <div class="field-label">Accept-Encoding</div>
          <div class="field-control">
            <ElSelect
              :model-value="request.acceptEncoding ?? ''"
              clearable
              placeholder="留空表示不声明"
              style="width: 260px"
              @update:model-value="(v: string) => set({ acceptEncoding: v ?? '' })"
            >
              <ElOption
                v-for="enc in ENCODINGS"
                :key="enc"
                :label="enc || '（不声明）'"
                :value="enc"
              />
            </ElSelect>
          </div>
        </div>
        <div class="field-tip">
          留空时不会声明 Accept-Encoding，响应即按服务端原始返回呈现；填了则按声明解压，
          但响应头里就不会再出现 Content-Encoding，两种情况下看到的响应体都是可读的。
        </div>
      </div>
    </div>

    <div class="panel-foot">
      <div class="foot-left">
        <span class="count">这些选项随请求一起保存，不影响设置页里的全局默认值</span>
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
@use './panel.scss';

.unit {
  margin-left: 8px;
  font-size: 12px;
  color: var(--text-color-secondary);
}
</style>
