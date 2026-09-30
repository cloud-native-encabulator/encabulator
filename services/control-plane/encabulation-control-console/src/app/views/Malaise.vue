<script setup lang="ts">
/**
 * Malaise: induce a fault, watch it live, clear it.
 *
 * The console does not break anything itself. It asks the malaise injection
 * engine, which holds the only write credentials in the system, and which
 * reverses everything it did on expiry. The countdowns below are the engine's own
 * `expiresAt`, so an operator can see a fault heal rather than wonder.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { MALAISE_KINDS, useMalaiseStore, type MalaiseKind } from '@/stores/controls'
import { useLiveStore } from '@/stores/live'
import { useSessionStore } from '@/stores/session'
import { EMPTY_STATES } from '@/lib/lexicon'
import { humanise, until } from '@/lib/format'
import Notice from '@/components/panel/Notice.vue'
import PanelPlate from '@/components/panel/PanelPlate.vue'

const malaise = useMalaiseStore()
const live = useLiveStore()
const session = useSessionStore()

const target = ref('')
const kind = ref<MalaiseKind>('latency')
const durationSeconds = ref(120)
const params = ref<Record<string, number>>({})
const now = ref(Date.now())

let poll: ReturnType<typeof setInterval> | null = null
let tick: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  void malaise.reload()
  poll = setInterval(() => void malaise.reload(), 3000)
  tick = setInterval(() => {
    now.value = Date.now()
  }, 1000)
  resetParams()
})
onBeforeUnmount(() => {
  if (poll !== null) clearInterval(poll)
  if (tick !== null) clearInterval(tick)
})

const spec = computed(() => MALAISE_KINDS.find((k) => k.kind === kind.value) ?? MALAISE_KINDS[0]!)

function resetParams(): void {
  const next: Record<string, number> = {}
  for (const field of spec.value.fields) next[field.name] = field.default
  params.value = next
}

/**
 * Targets come from the live topology, which is the static registry merged with
 * the cluster: exactly the set the engine will accept. Kubernetes-level kinds can
 * also target the backing stores, which carry the malaise-target label.
 */
const targets = computed(() => {
  const nodes = live.nodes.filter((node) => {
    // The console does not apply malaise to itself.
    if (node.name === 'encabulation-control-console') return false
    if (spec.value.level === 'app') return node.kind === 'service'
    return true
  })
  return nodes.map((node) => ({ name: node.name, display: node.display, plane: node.plane }))
})

const locked = computed(() => !session.canOperate)

async function induce(): Promise<void> {
  if (target.value === '') {
    malaise.notice = 'Choose a target first.'
    return
  }
  await malaise.induce({
    target: target.value,
    kind: kind.value,
    params: { ...params.value },
    durationSeconds: durationSeconds.value,
  })
}

const activeByTarget = computed(() => {
  const out = new Map<string, number>()
  for (const m of malaise.active) out.set(m.target, (out.get(m.target) ?? 0) + 1)
  return out
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <Notice tone="error" :message="malaise.notice" />
    <Notice
      v-if="locked"
      tone="info"
      message="Inducing malaise requires the operator or admin role. You may watch what is already running."
    />

    <div class="grid gap-4 xl:grid-cols-[0.9fr_1.1fr]">
      <PanelPlate legend="Induce malaise" sublegend="Real faults, reversed automatically on expiry">
        <form class="flex flex-col gap-4" @submit.prevent="induce">
          <label class="flex flex-col gap-1">
            <span class="engraved">Kind</span>
            <select
              v-model="kind"
              class="field"
              :disabled="locked"
              @change="resetParams(); target = ''"
            >
              <optgroup label="Application level">
                <option
                  v-for="k in MALAISE_KINDS.filter((k) => k.level === 'app')"
                  :key="k.kind"
                  :value="k.kind"
                >
                  {{ k.label }} ({{ k.kind }})
                </option>
              </optgroup>
              <optgroup label="Kubernetes level">
                <option
                  v-for="k in MALAISE_KINDS.filter((k) => k.level === 'kubernetes')"
                  :key="k.kind"
                  :value="k.kind"
                >
                  {{ k.label }} ({{ k.kind }})
                </option>
              </optgroup>
            </select>
            <span class="typed text-[0.72rem] text-ink-faint">{{ spec.real }}</span>
          </label>

          <label class="flex flex-col gap-1">
            <span class="engraved">Target</span>
            <select v-model="target" class="field" :disabled="locked">
              <option value="" disabled>Choose a workload…</option>
              <option v-for="t in targets" :key="t.name" :value="t.name">
                {{ t.display }} — {{ t.plane }}
              </option>
            </select>
          </label>

          <div v-if="spec.fields.length > 0" class="grid grid-cols-2 gap-3">
            <label v-for="field in spec.fields" :key="field.name" class="flex flex-col gap-1">
              <span class="engraved">{{ field.label }}</span>
              <input
                v-model.number="params[field.name]"
                type="number"
                :min="field.min"
                :max="field.max"
                class="field"
                :disabled="locked"
              />
            </label>
          </div>

          <label class="flex flex-col gap-1">
            <span class="engraved">Duration (1 to 900 s)</span>
            <input
              v-model.number="durationSeconds"
              type="number"
              min="1"
              max="900"
              class="field"
              :disabled="locked"
            />
          </label>

          <div class="flex items-center gap-2">
            <button type="submit" class="knob knob-danger" :disabled="locked || malaise.busy">
              {{ malaise.busy ? 'Inducing…' : 'Induce' }}
            </button>
            <button
              type="button"
              class="knob"
              :disabled="locked || malaise.busy || malaise.active.length === 0"
              @click="malaise.clearAll()"
            >
              Clear all
            </button>
          </div>
        </form>

        <template #footer>
          <p class="typed text-[0.7rem] text-ink-faint">
            The console never touches the cluster. Pod kills, partitions and scaling are performed
            by the malaise injection engine, which remembers what it changed and puts it back.
          </p>
        </template>
      </PanelPlate>

      <div class="flex flex-col gap-4">
        <PanelPlate
          legend="Active malaise"
          sublegend="With the engine’s own countdown to expiry"
          :source="malaise.error === null ? null : { ok: false, detail: malaise.error }"
          :empty="malaise.active.length === 0"
          :empty-note="EMPTY_STATES.noMalaises"
        >
          <ul class="flex flex-col gap-2">
            <li
              v-for="m in malaise.active"
              :key="m.id"
              class="flex items-start justify-between gap-3 rounded border border-lacquer/40 bg-lacquer/5 px-2 py-1.5"
            >
              <div class="min-w-0">
                <p class="engraved text-ink">{{ humanise(m.kind) }} on {{ m.target }}</p>
                <p class="typed text-[0.72rem] text-ink-soft">
                  <template v-for="(value, key) in m.params" :key="key">
                    {{ key }}={{ value }}&nbsp;
                  </template>
                </p>
                <p class="typed text-[0.68rem] text-ink-faint">{{ m.id }}</p>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <span class="window px-2 py-0.5 text-[0.78rem]">{{ until(m.expiresAt, now) }}</span>
                <button
                  type="button"
                  class="knob"
                  :disabled="locked || malaise.busy"
                  @click="malaise.clear(m.id)"
                >
                  Clear
                </button>
              </div>
            </li>
          </ul>
        </PanelPlate>

        <PanelPlate
          legend="Where the malaise is landing"
          sublegend="Live health of every registry node, so cause and effect are visible together"
          :source="live.sources.prometheus"
        >
          <ul class="grid gap-1 sm:grid-cols-2">
            <li
              v-for="node in live.nodes"
              :key="node.name"
              class="flex items-center justify-between gap-2 border-b border-rule/40 py-0.5 text-[0.76rem] last:border-0"
            >
              <span class="typed truncate">{{ node.display }}</span>
              <span class="flex shrink-0 items-center gap-1.5">
                <span
                  v-if="(activeByTarget.get(node.name) ?? 0) > 0"
                  class="engraved text-lacquer"
                  >{{ activeByTarget.get(node.name) }} malaise</span
                >
                <span
                  class="inline-block h-2.5 w-2.5 rounded-full border border-brass-dark"
                  :style="{
                    background:
                      node.health === 'healthy'
                        ? 'var(--c-phosphor)'
                        : node.health === 'degraded'
                          ? 'var(--c-amber)'
                          : node.health === 'faulted'
                            ? 'var(--c-lacquer)'
                            : 'var(--c-slate)',
                  }"
                />
              </span>
            </li>
          </ul>
        </PanelPlate>
      </div>
    </div>
  </div>
</template>
