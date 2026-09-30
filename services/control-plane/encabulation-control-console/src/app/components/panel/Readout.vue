<script setup lang="ts">
/**
 * A sunken reading window: green glass, phosphor digits, an engraved legend.
 * Used where a dial would be overkill, which is most places.
 */
import { NO_DATA } from '@/lib/format'

const props = withDefaults(
  defineProps<{
    label: string
    /** Already formatted. Pass `NO_DATA` (or any falsy) for an absent reading. */
    value: string
    unit?: string
    title?: string
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { unit: '', title: '', size: 'md' },
)

const missing = (): boolean => props.value === NO_DATA || props.value === ''
</script>

<template>
  <div class="flex flex-col gap-1" :title="props.title">
    <span class="engraved leading-none">{{ props.label }}</span>
    <div
      class="window flex items-baseline gap-1 px-2 py-1"
      :class="{
        'text-sm': props.size === 'sm',
        'text-lg': props.size === 'md',
        'text-2xl': props.size === 'lg',
      }"
    >
      <span v-if="missing()" class="no-data !text-slate">{{ NO_DATA }}</span>
      <template v-else>
        <span>{{ props.value }}</span>
        <span v-if="props.unit !== ''" class="text-[0.6em] opacity-70">{{ props.unit }}</span>
      </template>
    </div>
  </div>
</template>
