<script setup lang="ts">
/**
 * The two words this console is built around. Rendered as a distinct element so
 * a reader can never mistake an absent measurement for a small one.
 */
import { NO_DATA } from '@/lib/format'

const props = withDefaults(defineProps<{ why?: string }>(), { why: '' })
</script>

<template>
  <span class="no-data" :title="props.why">{{ NO_DATA }}</span>
</template>
