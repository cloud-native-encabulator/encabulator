<script setup lang="ts">
/**
 * Grammeters: the traffic controls and the live journey statistics.
 *
 * The cardinal grammeter generates real customer traffic through the real
 * gateway, so every number on this page is the result of requests that actually
 * happened. The four patterns are the ones the design defines, with their real
 * formulas in the tooltips.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useTrafficStore, type TrafficPattern } from '@/stores/controls'
import { useSessionStore } from '@/stores/session'
import { count, decimal, millis, percent, rate } from '@/lib/format'
import Lexeme from '@/components/panel/Lexeme.vue'
import Notice from '@/components/panel/Notice.vue'
import PanelPlate from '@/components/panel/PanelPlate.vue'
import Readout from '@/components/panel/Readout.vue'
import RockerSwitch from '@/components/panel/RockerSwitch.vue'

const traffic = useTrafficStore()
const session = useSessionStore()

const PATTERNS: { value: TrafficPattern; label: string; real: string }[] = [
  { value: 'steady', label: 'Steady', real: 'A constant rate.' },
  {
    value: 'sine',
    label: 'Modial interaction',
    real: 'rate × (1 + 0.8·sin(2πt/period)) · the modial interaction of magneto-reluctance.',
  },
  {
    value: 'burst',
    label: 'Capacitive diractance',
    real: 'Base rate for 80% of each period, five times the rate for the other 20%.',
  },
  { value: 'ramp', label: 'Ramp', real: '0 to the target rate linearly over the period, then hold.' },
]

const JOURNEYS = ['browse', 'quote', 'order', 'track'] as const

const running = ref(false)
const rps = ref(5)
const pattern = ref<TrafficPattern>('steady')
const periodSeconds = ref(60)
const customers = ref(50)
const mix = ref<Record<string, number>>({ browse: 40, quote: 25, order: 20, track: 15 })

let poll: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  void traffic.reload()
  poll = setInterval(() => void traffic.reload(), 3000)
})
onBeforeUnmount(() => {
  if (poll !== null) clearInterval(poll)
})

/** Mirror the upstream's own settings into the form, until the user edits it. */
const dirty = ref(false)
watch(
  () => traffic.data,
  (state) => {
    if (state === null || dirty.value) return
    running.value = state.running ?? false
    rps.value = state.rps ?? state.targetRps ?? rps.value
    pattern.value = state.pattern ?? pattern.value
    periodSeconds.value = state.periodSeconds ?? periodSeconds.value
    customers.value = state.customers ?? customers.value
    if (state.mix !== undefined) mix.value = { ...state.mix }
  },
  { immediate: true, deep: true },
)

const locked = computed(() => !session.canOperate)
const lockReason = 'Driving traffic requires the operator or admin role.'

const mixTotal = computed(() =>
  JOURNEYS.reduce((sum, name) => sum + (mix.value[name] ?? 0), 0),
)

async function apply(): Promise<void> {
  dirty.value = false
  await traffic.change({
    running: running.value,
    rps: rps.value,
    pattern: pattern.value,
    periodSeconds: periodSeconds.value,
    customers: customers.value,
    mix: { ...mix.value },
  })
}

async function toggleRunning(next: boolean): Promise<void> {
  running.value = next
  dirty.value = false
  await traffic.change({ running: next })
}

const statusRows = computed(() => {
  const codes = traffic.data?.statusCodes ?? {}
  return Object.entries(codes).sort((a, b) => a[0].localeCompare(b[0]))
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <Notice tone="error" :message="traffic.notice" />
    <Notice
      v-if="locked"
      tone="info"
      message="You are a viewer. The gauges below are live; the controls are locked, and the console's backend will refuse a write even if you find a way to click one."
    />

    <div class="grid gap-4 xl:grid-cols-[1fr_1.2fr]">
      <PanelPlate
        legend="Grammeter drive"
        sublegend="Synthetic customers through the dingle-arm gateway"
        :source="traffic.error === null ? null : { ok: false, detail: traffic.error }"
      >
        <div class="flex flex-col gap-4">
          <div class="flex items-center justify-between">
            <span class="engraved">Drive</span>
            <RockerSwitch
              :model-value="running"
              on-label="Running"
              off-label="Stopped"
              :disabled="locked"
              :reason="lockReason"
              @update:model-value="toggleRunning"
            />
          </div>

          <label class="flex flex-col gap-1">
            <span class="engraved"
              >Target rate · <Lexeme of="skorMotion" short /> (0.1 to 500 req/s)</span
            >
            <div class="flex items-center gap-2">
              <input
                v-model.number="rps"
                type="range"
                min="0.1"
                max="500"
                step="0.1"
                class="flex-1 accent-[var(--c-brass)]"
                :disabled="locked"
                @input="dirty = true"
              />
              <input
                v-model.number="rps"
                type="number"
                min="0.1"
                max="500"
                step="0.1"
                class="field w-24"
                :disabled="locked"
                @input="dirty = true"
              />
            </div>
          </label>

          <fieldset class="flex flex-col gap-1" :disabled="locked">
            <legend class="engraved">Pattern</legend>
            <label
              v-for="option in PATTERNS"
              :key="option.value"
              class="typed flex items-baseline gap-2 text-[0.8rem]"
              :title="option.real"
            >
              <input
                v-model="pattern"
                type="radio"
                :value="option.value"
                name="pattern"
                :disabled="locked"
                @change="dirty = true"
              />
              <span>{{ option.label }}</span>
              <span class="text-[0.7rem] text-ink-faint">{{ option.real }}</span>
            </label>
          </fieldset>

          <div class="grid grid-cols-2 gap-3">
            <label class="flex flex-col gap-1">
              <span class="engraved">Period (s)</span>
              <input
                v-model.number="periodSeconds"
                type="number"
                min="1"
                max="3600"
                class="field"
                :disabled="locked"
                @input="dirty = true"
              />
            </label>
            <label class="flex flex-col gap-1">
              <span class="engraved">Customers</span>
              <input
                v-model.number="customers"
                type="number"
                min="1"
                max="2000"
                class="field"
                :disabled="locked"
                @input="dirty = true"
              />
            </label>
          </div>

          <fieldset class="flex flex-col gap-2" :disabled="locked">
            <legend class="engraved">Journey mix (weights, total {{ mixTotal }})</legend>
            <label
              v-for="journey in JOURNEYS"
              :key="journey"
              class="flex items-center gap-2 text-[0.8rem]"
            >
              <span class="w-16 typed">{{ journey }}</span>
              <input
                v-model.number="mix[journey]"
                type="range"
                min="0"
                max="100"
                class="flex-1 accent-[var(--c-brass)]"
                :disabled="locked"
                @input="dirty = true"
              />
              <span class="window w-10 px-1 text-center text-[0.75rem]">{{ mix[journey] }}</span>
            </label>
          </fieldset>

          <div class="flex items-center gap-2">
            <button type="button" class="knob" :disabled="locked || traffic.saving" @click="apply">
              {{ traffic.saving ? 'Applying…' : 'Apply' }}
            </button>
            <span v-if="dirty" class="typed text-[0.72rem] text-amber">unapplied changes</span>
          </div>
        </div>
      </PanelPlate>

      <div class="flex flex-col gap-4">
        <PanelPlate
          legend="Drive readings"
          sublegend="Reported by the grammeter over the last sixty seconds"
          :source="traffic.error === null ? null : { ok: false, detail: traffic.error }"
        >
          <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
            <Readout
              label="Target rate"
              :value="rate(traffic.data?.targetRps ?? traffic.data?.rps ?? null)"
              unit="req/s"
            />
            <Readout label="Actual rate" :value="rate(traffic.data?.actualRps ?? null)" unit="req/s" />
            <Readout label="In flight" :value="count(traffic.data?.inFlight ?? null)" />
            <Readout
              label="Dropped"
              :value="count(traffic.data?.dropped ?? null)"
              title="Requests the scheduler shed because MAX_IN_FLIGHT was reached. Not queued."
            />
          </div>
        </PanelPlate>

        <PanelPlate
          legend="Journeys"
          sublegend="Counts, errors and latency per journey over the last sixty seconds"
          :empty="traffic.journeyRows.length === 0"
          empty-note="The grammeter has reported no journeys. Start the drive and they appear here."
        >
          <table class="w-full border-collapse text-[0.8rem]">
            <thead>
              <tr class="border-b border-rule text-left">
                <th class="engraved py-1 pr-3">Journey</th>
                <th class="engraved py-1 pr-3">Calls</th>
                <th class="engraved py-1 pr-3">Errors</th>
                <th class="engraved py-1 pr-3">p50</th>
                <th class="engraved py-1">p99</th>
              </tr>
            </thead>
            <tbody class="typed">
              <tr
                v-for="row in traffic.journeyRows"
                :key="row.name"
                class="border-b border-rule/50 last:border-0"
              >
                <td class="py-1 pr-3">{{ row.name }}</td>
                <td class="py-1 pr-3">{{ count(row.count) }}</td>
                <td class="py-1 pr-3">
                  {{ count(row.errors) }}
                  <span
                    v-if="row.errors !== null && row.count !== null && row.count > 0"
                    class="text-ink-faint"
                    >({{ percent(row.errors / row.count) }})</span
                  >
                </td>
                <td class="py-1 pr-3">{{ millis(row.p50Ms) }}</td>
                <td class="py-1">{{ millis(row.p99Ms) }}</td>
              </tr>
            </tbody>
          </table>
        </PanelPlate>

        <PanelPlate
          legend="Status codes"
          sublegend="What the gateway answered"
          :empty="statusRows.length === 0"
          empty-note="No responses counted yet."
          dense
        >
          <ul class="flex flex-wrap gap-2">
            <li
              v-for="[code, n] in statusRows"
              :key="code"
              class="window px-2 py-0.5 text-[0.78rem]"
            >
              {{ code }} · {{ decimal(n, 0) }}
            </li>
          </ul>
        </PanelPlate>
      </div>
    </div>
  </div>
</template>
