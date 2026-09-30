<script setup lang="ts">
/**
 * An indicator lamp. Four states only, matching the topology health colouring:
 * lit green, amber, red, or dark. Dark means no data, and it never lights green
 * by default.
 */
import type { Health } from '@shared/topology'
import { healthLabel } from '@/lib/layout'

const props = withDefaults(
  defineProps<{ health: Health; label?: string; size?: number }>(),
  { label: '', size: 10 },
)

const COLOUR: Record<Health, string> = {
  healthy: 'var(--c-phosphor)',
  degraded: 'var(--c-amber)',
  faulted: 'var(--c-lacquer)',
  nodata: 'var(--c-slate)',
}
</script>

<template>
  <span class="inline-flex items-center gap-1.5" :title="healthLabel(props.health)">
    <span
      class="inline-block rounded-full"
      :style="{
        width: `${props.size}px`,
        height: `${props.size}px`,
        background: COLOUR[props.health],
        boxShadow:
          props.health === 'nodata'
            ? 'inset 0 1px 2px rgb(0 0 0 / 50%)'
            : `0 0 6px ${COLOUR[props.health]}`,
        border: '1px solid var(--c-brass-dark)',
      }"
      :aria-label="healthLabel(props.health)"
      role="img"
    />
    <span v-if="props.label !== ''" class="engraved">{{ props.label }}</span>
  </span>
</template>
