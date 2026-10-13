<script setup lang="ts">
/**
 * Scenarios: the rehearsed demonstrations, their narration, and their progress.
 *
 * Narration arrives on the live stream rather than by polling from the browser,
 * so every attached console sees the same line at the same moment. That matters
 * on a stage with a second screen.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useScenarioStore } from '@/stores/controls'
import { useLiveStore } from '@/stores/live'
import { useSessionStore } from '@/stores/session'
import { EMPTY_STATES } from '@/lib/lexicon'
import { clock } from '@/lib/format'
import { isRunActive } from '@/lib/runs'
import Notice from '@/components/panel/Notice.vue'
import PanelPlate from '@/components/panel/PanelPlate.vue'

const scenarios = useScenarioStore()
const live = useLiveStore()
const session = useSessionStore()

let poll: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  void scenarios.loadScenarios()
  void scenarios.loadRuns()
  poll = setInterval(() => void scenarios.loadRuns(), 4000)
})
onBeforeUnmount(() => {
  if (poll !== null) clearInterval(poll)
})

const locked = computed(() => !session.canOperate)

// The stream carries the orchestrator's latest run, whatever its status. Only a
// run that is still going holds the orchestrator; one that has completed,
// failed or been cancelled is shown as the last run and frees the Run buttons.
const latestRun = computed(() => live.scenario)
const runActive = computed(() => latestRun.value !== null && isRunActive(latestRun.value.status))
const activeRunId = computed(() => (runActive.value ? (latestRun.value?.runId ?? null) : null))
// Lines narrated while this page is open arrive on the feed. A page opened after
// a run ended has seen none of them, so it reads them from the run itself.
const narration = computed(() =>
  live.narration.length > 0 ? live.narration : [...(latestRun.value?.items ?? [])].reverse(),
)
const runError = computed(() => latestRun.value?.error ?? null)

const progressPercent = computed(() => {
  const run = latestRun.value
  if (run === null || run.stepIndex === null || run.stepCount === null || run.stepCount === 0) {
    return null
  }
  return Math.min(100, Math.round((run.stepIndex / run.stepCount) * 100))
})

const running = ref<string | null>(null)
async function start(id: string): Promise<void> {
  running.value = id
  await scenarios.run(id)
  running.value = null
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <Notice tone="error" :message="scenarios.notice" />
    <Notice
      v-if="locked"
      tone="info"
      message="Running a scenario requires the operator or admin role. Narration below is live for everyone."
    />

    <div class="grid gap-4 xl:grid-cols-[1fr_1fr]">
      <PanelPlate
        legend="Scenario library"
        sublegend="Built into the orchestrator image, plus anything mounted at /etc/scenarios"
        :source="scenarios.scenariosError === null ? null : { ok: false, detail: scenarios.scenariosError }"
        :empty="scenarios.scenarios.length === 0"
        :empty-note="EMPTY_STATES.noScenarios"
      >
        <ul class="flex flex-col gap-2">
          <li
            v-for="scenario in scenarios.scenarios"
            :key="scenario.id"
            class="flex items-start justify-between gap-3 border-b border-rule/50 pb-2 last:border-0"
          >
            <div class="min-w-0">
              <p class="engraved text-ink">{{ scenario.title }}</p>
              <p class="typed text-[0.72rem] text-ink-faint">{{ scenario.id }}</p>
              <p v-if="scenario.description" class="typed mt-0.5 text-[0.78rem] text-ink-soft">
                {{ scenario.description }}
              </p>
            </div>
            <button
              type="button"
              class="knob shrink-0"
              :disabled="locked || scenarios.busy || activeRunId !== null"
              :title="activeRunId !== null ? 'A run is already active; the orchestrator allows one at a time.' : undefined"
              @click="start(scenario.id)"
            >
              {{ running === scenario.id ? 'Starting…' : 'Run' }}
            </button>
          </li>
        </ul>
      </PanelPlate>

      <div class="flex flex-col gap-4">
        <PanelPlate
          :legend="runActive ? 'Current run' : 'Last run'"
          sublegend="From the live stream"
          :empty="latestRun === null"
          empty-note="No run is active. Start a scenario and its narration prints here as it happens."
        >
          <div v-if="latestRun !== null" class="flex flex-col gap-2">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <div>
                <p class="engraved text-ink">{{ latestRun.scenarioId ?? 'scenario' }}</p>
                <p class="typed text-[0.72rem] text-ink-faint">{{ latestRun.runId }}</p>
              </div>
              <span class="window px-2 py-0.5 text-[0.78rem]">{{ latestRun.status ?? 'no data' }}</span>
            </div>

            <p v-if="!runActive" class="typed text-[0.78rem] text-ink-soft">
              This run is over. Any scenario can be started again.
            </p>
            <p v-if="!runActive && runError !== null" class="typed text-[0.78rem] text-ink-soft">
              <span class="engraved text-ink-faint">stopped because:</span> {{ runError }}
            </p>

            <p v-if="latestRun.currentStep !== null" class="typed text-[0.82rem]">
              <span class="engraved text-ink-faint">{{ runActive ? 'step:' : 'last step:' }}</span>
              {{ latestRun.currentStep }}
            </p>

            <div v-if="progressPercent !== null" class="flex items-center gap-2">
              <div class="window h-3 flex-1 overflow-hidden p-0">
                <div
                  class="h-full"
                  :style="{
                    width: `${progressPercent}%`,
                    background: 'var(--c-phosphor)',
                    opacity: 0.8,
                  }"
                />
              </div>
              <span class="font-[family-name:var(--font-gauge)] text-[0.75rem]"
                >{{ progressPercent }}%</span
              >
            </div>

            <button
              v-if="runActive"
              type="button"
              class="knob knob-danger self-start"
              :disabled="locked || scenarios.busy || latestRun.runId === null"
              @click="latestRun.runId !== null && scenarios.cancel(latestRun.runId)"
            >
              Cancel run
            </button>
          </div>
          <template #footer>
            <p class="typed text-[0.7rem] text-ink-faint">
              Cancelling reverts the flags the run changed and clears the malaises it created. The
              orchestrator does that, not this page.
            </p>
          </template>
        </PanelPlate>

        <PanelPlate
          legend="Narration"
          sublegend="Every line the orchestrator has said, newest first"
          :empty="narration.length === 0"
          empty-note="Nothing narrated yet."
        >
          <ol class="flex flex-col gap-1">
            <li
              v-for="line in narration.slice(0, 40)"
              :key="line.id"
              class="typed flex gap-2 border-b border-rule/40 py-0.5 text-[0.8rem] last:border-0"
            >
              <span class="shrink-0 font-[family-name:var(--font-gauge)] text-[0.7rem] text-ink-faint">
                {{ clock(line.at) }}
              </span>
              <span>{{ line.headline }}</span>
            </li>
          </ol>
        </PanelPlate>

        <PanelPlate
          legend="Recent runs"
          :source="scenarios.runsError === null ? null : { ok: false, detail: scenarios.runsError }"
          :empty="scenarios.runs.length === 0"
          empty-note="No runs recorded. The orchestrator keeps its history in memory, so a restart clears it."
          dense
        >
          <table class="w-full border-collapse text-[0.78rem]">
            <thead>
              <tr class="border-b border-rule text-left">
                <th class="engraved py-1 pr-3">Scenario</th>
                <th class="engraved py-1 pr-3">Status</th>
                <th class="engraved py-1">Started</th>
              </tr>
            </thead>
            <tbody class="typed">
              <tr
                v-for="run in scenarios.runs"
                :key="run.id"
                class="border-b border-rule/40 last:border-0"
              >
                <td class="py-1 pr-3">{{ run.scenarioId ?? run.id }}</td>
                <td class="py-1 pr-3">{{ run.status ?? 'no data' }}</td>
                <td class="py-1">{{ clock(run.startedAt ?? null) }}</td>
              </tr>
            </tbody>
          </table>
        </PanelPlate>
      </div>
    </div>
  </div>
</template>
