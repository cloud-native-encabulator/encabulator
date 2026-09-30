<script setup lang="ts">
/**
 * The narration ticker: paper tape across the bottom of the Factory Floor.
 *
 * It shows scenario narration from the orchestrator and nothing else. When no
 * scenario is running it says so, rather than scrolling filler.
 */
import { computed } from 'vue'
import { clock } from '@/lib/format'
import type { FeedItem } from '@shared/topology'
import type { ScenarioProgress } from '@/stores/live'

const props = defineProps<{
  narration: FeedItem[]
  scenario: ScenarioProgress | null
}>()

const latest = computed(() => props.narration.slice(0, 6))
const progress = computed(() => {
  const s = props.scenario
  if (s === null || s.stepIndex === null || s.stepCount === null) return null
  return `step ${s.stepIndex} of ${s.stepCount}`
})
</script>

<template>
  <div class="tape flex items-stretch overflow-hidden" role="log" aria-live="polite">
    <div
      class="flex shrink-0 items-center gap-2 border-r border-rule bg-plate-sunk px-3 py-1.5"
    >
      <span class="engraved">Narration</span>
      <span
        v-if="props.scenario !== null && props.scenario.status !== null"
        class="window px-1.5 py-0.5 text-[0.65rem]"
        >{{ props.scenario.status }}</span
      >
      <span v-if="progress !== null" class="text-[0.65rem] text-ink-faint typed">{{
        progress
      }}</span>
    </div>

    <p
      v-if="latest.length === 0"
      class="typed flex-1 px-3 py-1.5 text-[0.78rem] text-ink-faint"
    >
      No scenario running. Start one from Scenarios and the orchestrator’s narration prints here.
    </p>

    <ol v-else class="flex min-w-0 flex-1 items-center gap-6 overflow-x-auto px-3 py-1.5">
      <li
        v-for="item in latest"
        :key="item.id"
        class="typed flex shrink-0 items-baseline gap-2 text-[0.78rem]"
      >
        <span class="text-[0.65rem] text-ink-faint">{{ clock(item.at) }}</span>
        <span class="text-ink">{{ item.headline }}</span>
      </li>
    </ol>
  </div>
</template>
