<script setup lang="ts">
/**
 * A term from the 1944 vocabulary, with the real meaning in the tooltip.
 *
 * The joke is the label. The tooltip is not a joke: it is what stops a funny
 * word from being a misleading one.
 */
import { computed } from 'vue'
import { LEXICON } from '@/lib/lexicon'

const props = withDefaults(defineProps<{ of: keyof typeof LEXICON; short?: boolean }>(), {
  short: false,
})

const lexeme = computed(() => LEXICON[props.of])
const tooltip = computed(() => {
  const l = lexeme.value
  if (l === undefined) return ''
  return l.source === undefined ? l.real : `${l.real}. Source: ${l.source}`
})
const text = computed(() => {
  const term = lexeme.value?.term ?? String(props.of)
  return props.short ? (term.split(' ').pop() ?? term) : term
})
</script>

<template>
  <abbr class="lexeme no-underline" :title="tooltip">{{ text }}</abbr>
</template>
