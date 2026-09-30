<script setup lang="ts">
/**
 * Girdlespring: the feature flags, and what each one really does.
 *
 * Every flag here changes real behaviour in a real service within five seconds,
 * because that is how often the services poll. The "affects" column is the
 * design's own table, so an operator flipping a switch on stage can say what is
 * about to happen before it happens.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useFlagStore, type Flag } from '@/stores/controls'
import { useSessionStore } from '@/stores/session'
import { EMPTY_STATES } from '@/lib/lexicon'
import { ago, humanise } from '@/lib/format'
import Notice from '@/components/panel/Notice.vue'
import PanelPlate from '@/components/panel/PanelPlate.vue'
import RockerSwitch from '@/components/panel/RockerSwitch.vue'

const flags = useFlagStore()
const session = useSessionStore()

/** From design section 6: who reads each flag, and what it does to them. */
const EFFECTS: Record<string, { readers: string; effect: string }> = {
  'turbo-encabulation': {
    readers: 'casing line, quoter',
    effect: 'Halves every fabrication stage duration, and adds a 15% turbo surcharge to quotes.',
  },
  'quote-cache': {
    readers: 'quoter',
    effect: 'Caches identical quotes in Valkey for sixty seconds.',
  },
  'retro-encabulator-sales': {
    readers: 'processor',
    effect: 'When off, retro orders are rejected with model_unavailable.',
  },
  'strict-side-fumbling': {
    readers: 'detector',
    effect:
      'Tightens the anomaly thresholds: z > 3.0 and a [0.8, 1.2] band. Catches subtler defects, and produces false positives, so the refabrication rate visibly rises.',
  },
  'amulite-foundry-rate': {
    readers: 'depot',
    effect: 'Units of each SKU restocked every ten seconds. Zero stops the foundry.',
  },
  'gateway-rate-limit-rps': {
    readers: 'gateway',
    effect: 'Per-subject token bucket rate, with a burst of twice this.',
  },
  'dingle-arm-damping': {
    readers: 'casing line',
    effect: 'Percentage by which the dingle arm option reduces a unit’s defect rate.',
  },
}

let poll: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  void flags.reload()
  poll = setInterval(() => void flags.reload(), 5000)
})
onBeforeUnmount(() => {
  if (poll !== null) clearInterval(poll)
})

const locked = computed(() => !session.canOperate)

/** Pending numeric edits, keyed by flag. Booleans commit on click. */
const drafts = ref<Record<string, number>>({})

function draftFor(flag: Flag): number {
  const existing = drafts.value[flag.key]
  if (existing !== undefined) return existing
  return typeof flag.value === 'number' ? flag.value : 0
}

async function setBoolean(flag: Flag, next: boolean): Promise<void> {
  await flags.set(flag.key, {
    value: next,
    ...(flag.rolloutPercent === null || flag.rolloutPercent === undefined
      ? {}
      : { rolloutPercent: flag.rolloutPercent }),
  })
}

async function commitNumber(flag: Flag): Promise<void> {
  const value = draftFor(flag)
  await flags.set(flag.key, { value })
  delete drafts.value[flag.key]
}

const sorted = computed(() => [...flags.data].sort((a, b) => a.key.localeCompare(b.key)))
</script>

<template>
  <div class="flex flex-col gap-4">
    <Notice tone="error" :message="flags.notice" />
    <Notice
      v-if="locked"
      tone="info"
      message="Setting a flag requires the operator or admin role. Values below are live, and refresh every five seconds."
    />

    <PanelPlate
      legend="Differential girdlespring"
      sublegend="On the “up” end of the grammeters. Services poll these every five seconds."
      :source="flags.error === null ? null : { ok: false, detail: flags.error }"
      :empty="sorted.length === 0"
      :empty-note="EMPTY_STATES.noFlags"
    >
      <ul class="flex flex-col divide-y divide-rule/50">
        <li v-for="flag in sorted" :key="flag.key" class="flex flex-col gap-2 py-3 first:pt-0">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="engraved text-ink">{{ humanise(flag.key) }}</p>
              <p class="typed text-[0.7rem] text-ink-faint">
                <code>{{ flag.key }}</code>
                · {{ flag.type }}
                <template v-if="EFFECTS[flag.key] !== undefined">
                  · read by {{ EFFECTS[flag.key]!.readers }}
                </template>
              </p>
            </div>

            <div class="flex shrink-0 items-center gap-3">
              <RockerSwitch
                v-if="flag.type === 'boolean'"
                :model-value="flag.value === true"
                :disabled="locked || flags.saving === flag.key"
                reason="Setting a flag requires the operator or admin role."
                @update:model-value="(next) => setBoolean(flag, next)"
              />
              <template v-else-if="flag.type === 'number'">
                <input
                  :value="draftFor(flag)"
                  type="number"
                  class="field w-28"
                  :disabled="locked"
                  @input="drafts[flag.key] = Number(($event.target as HTMLInputElement).value)"
                />
                <button
                  type="button"
                  class="knob"
                  :disabled="locked || flags.saving === flag.key || drafts[flag.key] === undefined"
                  @click="commitNumber(flag)"
                >
                  Set
                </button>
              </template>
              <span v-else class="window px-2 py-0.5 text-[0.78rem]">{{ flag.value }}</span>
            </div>
          </div>

          <p
            v-if="EFFECTS[flag.key] !== undefined"
            class="typed text-[0.78rem] leading-relaxed text-ink-soft"
          >
            {{ EFFECTS[flag.key]!.effect }}
          </p>
          <p v-else-if="flag.description" class="typed text-[0.78rem] text-ink-soft">
            {{ flag.description }}
          </p>

          <p class="typed text-[0.68rem] text-ink-faint">
            <template v-if="flag.rolloutPercent !== null && flag.rolloutPercent !== undefined">
              rollout {{ flag.rolloutPercent }}% (fnv1a32(key + ":" + subject) % 100) ·
            </template>
            <template v-if="flag.updatedBy">set by {{ flag.updatedBy }} ·</template>
            <template v-if="flag.updatedAt">{{ ago(flag.updatedAt) }}</template>
            <template v-else>never changed</template>
          </p>
        </li>
      </ul>

      <template #footer>
        <p class="typed text-[0.7rem] text-ink-faint">
          A flag with a rollout percentage applies to a subject when
          <code>fnv1a32(key + ":" + subject) % 100 &lt; rolloutPercent</code>. Go, TypeScript,
          Python and Rust all compute that the same way, so a subject is either in or out
          everywhere.
        </p>
      </template>
    </PanelPlate>
  </div>
</template>
