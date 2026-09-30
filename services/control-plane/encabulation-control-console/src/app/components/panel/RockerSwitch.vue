<script setup lang="ts">
/**
 * A two-position brass rocker. Disabled for a viewer, with the reason in the
 * tooltip; the BFF refuses the write regardless, so this is courtesy only.
 */
const props = withDefaults(
  defineProps<{
    modelValue: boolean
    onLabel?: string
    offLabel?: string
    disabled?: boolean
    reason?: string
  }>(),
  { onLabel: 'ON', offLabel: 'OFF', disabled: false, reason: '' },
)

const emit = defineEmits<{ 'update:modelValue': [boolean] }>()
</script>

<template>
  <button
    type="button"
    role="switch"
    :aria-checked="props.modelValue"
    :disabled="props.disabled"
    :title="props.disabled && props.reason !== '' ? props.reason : undefined"
    class="inline-flex items-center gap-2 disabled:cursor-not-allowed disabled:opacity-60"
    @click="emit('update:modelValue', !props.modelValue)"
  >
    <span
      class="relative inline-block h-5 w-10 rounded-full border border-brass-dark transition-colors"
      :style="{
        background: props.modelValue
          ? 'color-mix(in srgb, var(--c-phosphor) 35%, var(--c-gauge-face))'
          : 'var(--c-plate-sunk)',
      }"
    >
      <span
        class="absolute top-0.5 h-3.5 w-3.5 rounded-full transition-all"
        :style="{
          left: props.modelValue ? '1.375rem' : '0.125rem',
          background: 'linear-gradient(135deg, var(--c-brass-bright), var(--c-brass-dark))',
        }"
      />
    </span>
    <span class="engraved">{{ props.modelValue ? props.onLabel : props.offLabel }}</span>
  </button>
</template>
