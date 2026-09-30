<script setup lang="ts">
/**
 * An enamel plate with an engraved legend and brass corner screws. The workhorse
 * container of the panel.
 *
 * `source` is the honesty affordance: pass the status of whatever fed this plate
 * and the footer says, in words, why the plate is empty. A plate that shows
 * nothing and explains nothing is the failure mode this prop exists to prevent.
 */
import type { SourceStatus } from '@shared/topology'

const props = withDefaults(
  defineProps<{
    legend: string
    /** Small print under the legend: units, window, or the real meaning. */
    sublegend?: string
    source?: SourceStatus | null
    /** Reason the plate is empty, shown when there is no content and no error. */
    emptyNote?: string | null
    /** Show the empty note. The parent decides what "empty" means. */
    empty?: boolean
    dense?: boolean
    screws?: boolean
  }>(),
  { sublegend: '', source: null, emptyNote: null, empty: false, dense: false, screws: true },
)
</script>

<template>
  <section
    class="plate flex flex-col"
    :class="[props.screws ? 'plate-screws' : '', props.dense ? 'p-3 pt-5' : 'p-4 pt-6']"
  >
    <header class="mb-3 border-b border-rule pb-2">
      <h2 class="engraved text-ink">{{ props.legend }}</h2>
      <p v-if="props.sublegend !== ''" class="mt-0.5 text-[0.7rem] text-ink-faint typed">
        {{ props.sublegend }}
      </p>
    </header>

    <div class="min-h-0 flex-1">
      <p
        v-if="props.source !== null && !props.source.ok"
        class="typed rounded border border-lacquer/40 bg-lacquer/8 px-2 py-1.5 text-[0.75rem] text-ink-soft"
        role="status"
      >
        <span class="engraved text-lacquer">No data.</span>
        {{ props.source.detail ?? 'The upstream did not answer.' }}
      </p>
      <p
        v-else-if="props.empty && props.emptyNote !== null"
        class="typed text-[0.8rem] leading-relaxed text-ink-soft"
      >
        {{ props.emptyNote }}
      </p>
      <slot v-else />
    </div>

    <footer v-if="$slots.footer" class="mt-3 border-t border-rule pt-2">
      <slot name="footer" />
    </footer>
  </section>
</template>
