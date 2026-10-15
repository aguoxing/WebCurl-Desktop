<script lang="ts" setup>
import { ref } from 'vue'

/**
 * 可拖拽的分栏容器。
 *
 * 没有引入第三方分栏库：这里只需要"两栏 + 能拖 + 有最小尺寸"三件事，
 * 而第三方库会带进自己的 CSS 变量与插槽约定，为了改一条分隔线的颜色
 * 要翻一整套文档；自己实现反而只有几十行。
 */
const props = withDefaults(
  defineProps<{
    /** vertical = 上下两栏（分隔条水平），horizontal = 左右两栏。 */
    mode?: 'vertical' | 'horizontal'
    /** 第一栏的初始占比，取值 0~1。 */
    initial?: number
    /** 第一栏允许的最小像素尺寸，防止被拖成 0。 */
    minFirst?: number
    /** 第二栏允许的最小像素尺寸。 */
    minSecond?: number
  }>(),
  { mode: 'vertical', initial: 0.55, minFirst: 150, minSecond: 120 }
)

const container = ref<HTMLElement | null>(null)
const ratio = ref(props.initial)
const dragging = ref(false)

function onPointerDown(event: PointerEvent) {
  dragging.value = true
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
  // 拖动期间禁用文本选中：否则鼠标掠过面板文字会拉出一片高亮，拖完还得再点一下取消。
  document.body.style.userSelect = 'none'
}

function onPointerMove(event: PointerEvent) {
  if (!dragging.value) return
  const el = container.value
  if (!el) return

  const rect = el.getBoundingClientRect()
  const vertical = props.mode === 'vertical'
  const total = vertical ? rect.height : rect.width
  if (total <= 0) return

  const offset = vertical ? event.clientY - rect.top : event.clientX - rect.left
  const lower = props.minFirst / total
  const upper = 1 - props.minSecond / total
  // 窗口很矮时 lower 可能大于 upper，此时 clamp 会落到 lower，
  // 结果是"第二栏被压到最小"——比完全拖不动更好解释。
  ratio.value = Math.min(Math.max(upper, lower), Math.max(lower, offset / total))
}

function onPointerUp(event: PointerEvent) {
  dragging.value = false
  document.body.style.userSelect = ''
  try {
    ;(event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId)
  } catch {
    // 指针已经释放（例如浏览器自己取消了捕获），此处不必处理。
  }
}

/** 用 flex-grow 表达比例：basis 为 0，两边同时伸展，天然按比例分配剩余空间。 */
const firstStyle = computed(() => ({ flexGrow: String(ratio.value), flexBasis: '0' }))
const secondStyle = computed(() => ({ flexGrow: String(1 - ratio.value), flexBasis: '0' }))
</script>

<template>
  <div ref="container" class="v-split" :class="[`is-${mode}`, { 'is-dragging': dragging }]">
    <div class="v-split-pane" :style="firstStyle">
      <slot name="first" />
    </div>

    <div
      class="v-split-divider"
      role="separator"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
    />

    <div class="v-split-pane" :style="secondStyle">
      <slot name="second" />
    </div>
  </div>
</template>

<style lang="scss" scoped>
.v-split {
  display: flex;
  width: 100%;
  height: 100%;
  min-height: 0;
}

.v-split.is-vertical {
  flex-direction: column;
}

.v-split.is-horizontal {
  flex-direction: row;
}

/* min-height/min-width 必须为 0：否则内容（比如长 JSON）会把 pane 撑过比例，
   拖动分隔条时会看到两栏互相顶。 */
.v-split-pane {
  display: flex;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.v-split-pane > :deep(*) {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
}

.v-split-divider {
  flex: 0 0 auto;
  background-color: var(--chrome-border);
  transition: background-color 0.15s ease;
}

.is-vertical > .v-split-divider {
  height: 1px;
  margin: 0;
  cursor: row-resize;
  /* 视觉上只有 1px，但实际可抓取区域放大到 7px，否则很难精确点到。 */
  border-top: 3px solid transparent;
  border-bottom: 3px solid transparent;
  background-clip: padding-box;
}

.is-horizontal > .v-split-divider {
  width: 1px;
  cursor: col-resize;
  border-left: 3px solid transparent;
  border-right: 3px solid transparent;
  background-clip: padding-box;
}

.v-split-divider:hover,
.v-split.is-dragging > .v-split-divider {
  background-color: var(--el-color-primary);
}
</style>
