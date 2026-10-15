import { h } from 'vue'
import type { VNode } from 'vue'
import { Icon } from '@/components/Icon'

export const useIcon = (props: any): VNode => {
  return h(Icon, props)
}
