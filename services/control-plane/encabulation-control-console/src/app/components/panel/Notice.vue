<script setup lang="ts">
/**
 * A stamped notice: the thing an operator reads when a control refused them, or
 * when an upstream is missing. Never dismissed automatically.
 */
const props = withDefaults(
  defineProps<{ tone?: 'info' | 'warn' | 'error'; message: string | null }>(),
  { tone: 'info' },
)
</script>

<template>
  <p
    v-if="props.message !== null && props.message !== ''"
    class="typed rounded border px-2 py-1.5 text-[0.78rem]"
    :class="{
      'border-rule bg-plate-sunk text-ink-soft': props.tone === 'info',
      'border-amber/50 text-ink': props.tone === 'warn',
      'border-lacquer/50 text-ink': props.tone === 'error',
    }"
    role="status"
  >
    {{ props.message }}
  </p>
</template>
