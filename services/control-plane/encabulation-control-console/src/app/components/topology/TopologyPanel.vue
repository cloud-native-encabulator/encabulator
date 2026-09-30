<script setup lang="ts">
/**
 * The Factory Floor topology.
 *
 * Two layers, one geometry:
 *
 *   - An SVG layer for everything structural: plane bands, edges, node plates,
 *     pod dots, labels. Vue re-renders it only when a snapshot changes, which is
 *     once every three seconds, so there is no per-frame DOM work.
 *   - A canvas layer, exactly on top, for the particles. One
 *     `requestAnimationFrame` loop draws all of them; see `lib/particles.ts` for
 *     the caps, the adaptive back-off, and why the jitter is deterministic.
 *
 * Honesty, restated because this is where it would be easiest to cheat: the
 * particle rate on an edge is proportional to that edge's measured requests per
 * second, and an edge with no service-graph data emits nothing and is drawn as a
 * dashed grey line. There is no idle animation.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { EdgeView, Health, NodeView, TopologySnapshot } from '@shared/topology'
import { PLANE_LABEL } from '@shared/registry'
import {
  GRID_ROWS,
  healthLabel,
  layoutEdges,
  layoutNodes,
  podDots,
  type PlacedEdge,
  type PlacedNode,
} from '@/lib/layout'
import { createParticleLoop, type LoopHandles, type ParticleColours } from '@/lib/particles'
import { count, millis, percent, rate } from '@/lib/format'
import StatusLamp from '../panel/StatusLamp.vue'

const props = withDefaults(
  defineProps<{
    snapshot: TopologySnapshot | null
    /** Turn the particle loop off, e.g. when the tab is hidden. */
    animate?: boolean
  }>(),
  { animate: true },
)

/** A fixed drawing surface; CSS scales it to the container. */
const VIEW = { width: 1200, height: 660 }

const container = ref<HTMLDivElement | null>(null)
const canvas = ref<HTMLCanvasElement | null>(null)
const hoveredNode = ref<NodeView | null>(null)
const hoveredEdge = ref<EdgeView | null>(null)
const hoveredPod = ref<{ node: NodeView; index: number } | null>(null)
const tip = ref({ x: 0, y: 0 })
const diagnostics = ref(false)
/** Sampled from the loop while the diagnostics footer is open. */
const diag = ref({ particles: 0, frameMs: 0, multiplier: 1 })

const nodes = computed<NodeView[]>(() => props.snapshot?.nodes ?? [])
const edges = computed<EdgeView[]>(() => props.snapshot?.edges ?? [])
const placedNodes = computed<PlacedNode[]>(() => layoutNodes(nodes.value, VIEW))
const placedEdges = computed<PlacedEdge[]>(() =>
  layoutEdges(edges.value, placedNodes.value, VIEW),
)

const bands = computed(() => {
  const rows = new Map<number, string>()
  for (const node of nodes.value) {
    if (!rows.has(node.lay.row)) rows.set(node.lay.row, PLANE_LABEL[node.plane])
  }
  const cellH = VIEW.height / GRID_ROWS
  return [...rows.entries()]
    .sort((a, b) => a[0] - b[0])
    .map(([row, label]) => ({ row, label, y: row * cellH, height: cellH }))
})

const activeEdgeCount = computed(() => edges.value.filter((e) => e.health !== 'nodata').length)

// ── the particle loop ────────────────────────────────────────────────────────

let loop: LoopHandles | null = null

function colours(): ParticleColours {
  const styles = getComputedStyle(document.documentElement)
  const read = (name: string, fallback: string): string => {
    const value = styles.getPropertyValue(name).trim()
    return value === '' ? fallback : value
  }
  return {
    healthy: read('--c-phosphor', '#6bf0a0'),
    degraded: read('--c-amber', '#d99a2b'),
    faulted: read('--c-lacquer', '#c0473a'),
    nodata: read('--c-slate', '#8a8378'),
  }
}

function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

onMounted(() => {
  const element = canvas.value
  if (element === null) return
  // A fixed backing store at the SVG's coordinate scale: the particle geometry
  // and the SVG geometry are then literally the same numbers.
  element.width = VIEW.width
  element.height = VIEW.height
  loop = createParticleLoop({
    canvas: element,
    colours,
    view: () => VIEW,
    reducedMotion: prefersReducedMotion,
  })
  loop.setEdges(placedEdges.value)
  if (props.animate) loop.start()
})

/** The loop is not reactive, so sample it on a slow timer when asked to. */
let diagTimer: ReturnType<typeof setInterval> | null = null

watch(diagnostics, (open) => {
  if (diagTimer !== null) {
    clearInterval(diagTimer)
    diagTimer = null
  }
  if (!open) return
  diagTimer = setInterval(() => {
    diag.value = {
      particles: loop?.count ?? 0,
      frameMs: loop?.frameMs ?? 0,
      multiplier: loop?.multiplier ?? 1,
    }
  }, 500)
})

onBeforeUnmount(() => {
  if (diagTimer !== null) clearInterval(diagTimer)
  diagTimer = null
  loop?.destroy()
  loop = null
})

watch(placedEdges, (next) => loop?.setEdges(next))

watch(
  () => props.animate,
  (on) => {
    if (on) loop?.start()
    else loop?.stop()
  },
)

// ── hover ────────────────────────────────────────────────────────────────────

function locate(event: MouseEvent): void {
  const rect = container.value?.getBoundingClientRect()
  if (rect === undefined) return
  tip.value = { x: event.clientX - rect.left, y: event.clientY - rect.top }
}

function enterNode(node: NodeView, event: MouseEvent): void {
  hoveredEdge.value = null
  hoveredPod.value = null
  hoveredNode.value = node
  locate(event)
}

function enterEdge(edge: EdgeView, event: MouseEvent): void {
  hoveredNode.value = null
  hoveredPod.value = null
  hoveredEdge.value = edge
  locate(event)
}

function enterPod(node: NodeView, index: number, event: MouseEvent): void {
  hoveredNode.value = null
  hoveredEdge.value = null
  hoveredPod.value = { node, index }
  locate(event)
}

function clearHover(): void {
  hoveredNode.value = null
  hoveredEdge.value = null
  hoveredPod.value = null
}

function healthColour(health: Health): string {
  switch (health) {
    case 'healthy':
      return 'var(--c-phosphor)'
    case 'degraded':
      return 'var(--c-amber)'
    case 'faulted':
      return 'var(--c-lacquer)'
    default:
      return 'var(--c-slate)'
  }
}

function podColour(node: NodeView, index: number): string {
  const pod = node.pods[index]
  if (pod === undefined) return 'var(--c-slate)'
  if (pod.phase !== 'Running') return 'var(--c-lacquer)'
  if (!pod.ready) return 'var(--c-amber)'
  return 'var(--c-phosphor)'
}
</script>

<template>
  <div
    ref="container"
    class="plate plate-screws relative overflow-hidden p-3 pt-6"
    @mouseleave="clearHover"
  >
    <header class="mb-2 flex flex-wrap items-baseline justify-between gap-2">
      <div>
        <h2 class="engraved text-ink">Factory floor · live topology</h2>
        <p class="typed text-[0.7rem] text-ink-faint">
          Plates are the static registry; dots are live pods; lines are only drawn where the
          OpenTelemetry service graph has seen a call in the last minute.
        </p>
      </div>
      <div class="flex items-center gap-3">
        <StatusLamp health="healthy" label="in tolerance" />
        <StatusLamp health="degraded" label="off nominal" />
        <StatusLamp health="faulted" label="out of tolerance" />
        <StatusLamp health="nodata" label="no data" />
        <button
          type="button"
          class="engraved text-ink-faint hover:text-ink"
          @click="diagnostics = !diagnostics"
        >
          {{ diagnostics ? 'hide' : 'show' }} diagnostics
        </button>
      </div>
    </header>

    <div class="relative">
      <svg
        :viewBox="`0 0 ${VIEW.width} ${VIEW.height}`"
        class="block h-auto w-full"
        role="img"
        aria-label="Live service topology of the Encabulator Works"
      >
        <!-- plane bands -->
        <g>
          <rect
            v-for="band in bands"
            :key="band.row"
            x="0"
            :y="band.y"
            :width="VIEW.width"
            :height="band.height"
            :fill="band.row % 2 === 0 ? 'color-mix(in srgb, var(--c-ink) 3%, transparent)' : 'transparent'"
          />
          <text
            v-for="band in bands"
            :key="`label-${band.row}`"
            x="8"
            :y="band.y + 14"
            fill="var(--c-ink-faint)"
            style="font-size: 0.6rem; letter-spacing: 0.16em; text-transform: uppercase"
          >
            {{ band.label }}
          </text>
        </g>

        <!-- edges -->
        <g>
          <path
            v-for="placed in placedEdges"
            :key="`${placed.edge.source}->${placed.edge.target}`"
            :d="placed.path"
            fill="none"
            :stroke="healthColour(placed.edge.health)"
            :stroke-width="placed.width"
            :stroke-dasharray="placed.edge.health === 'nodata' ? '5 6' : undefined"
            :opacity="placed.edge.health === 'nodata' ? 0.45 : 0.8"
            stroke-linecap="round"
          />
          <!-- a fat transparent path per edge, so hovering a thin line is possible -->
          <path
            v-for="placed in placedEdges"
            :key="`hit-${placed.edge.source}->${placed.edge.target}`"
            :d="placed.path"
            fill="none"
            stroke="transparent"
            stroke-width="14"
            style="cursor: help"
            @mouseenter="enterEdge(placed.edge, $event)"
            @mousemove="locate"
          />
        </g>

        <!-- node plates -->
        <g>
          <g
            v-for="placed in placedNodes"
            :key="placed.node.name"
            style="cursor: help"
            @mouseenter="enterNode(placed.node, $event)"
            @mousemove="locate"
          >
            <rect
              :x="placed.box.x"
              :y="placed.box.y"
              :width="placed.box.width"
              :height="placed.box.height"
              rx="3"
              fill="var(--c-plate)"
              :stroke="placed.node.kind === 'store' ? 'var(--c-brass-dark)' : 'var(--c-brass)'"
              :stroke-width="placed.node.kind === 'store' ? 2.5 : 1.5"
              :stroke-dasharray="placed.node.pods.length === 0 ? '4 4' : undefined"
            />
            <!-- health lamp, top-left of the plate -->
            <circle
              :cx="placed.box.x + 9"
              :cy="placed.box.y + 9"
              r="4"
              :fill="healthColour(placed.node.health)"
              stroke="var(--c-brass-dark)"
              stroke-width="0.8"
            />
            <text
              :x="placed.box.cx"
              :y="placed.box.y + 24"
              text-anchor="middle"
              fill="var(--c-ink)"
              style="font-size: 0.72rem; font-weight: 600; letter-spacing: 0.02em"
            >
              {{ placed.node.display }}
            </text>
            <text
              :x="placed.box.cx"
              :y="placed.box.y + 38"
              text-anchor="middle"
              fill="var(--c-ink-faint)"
              style="font-size: 0.58rem; letter-spacing: 0.1em; text-transform: uppercase"
            >
              {{ placed.node.language }}
            </text>
            <text
              v-if="placed.node.rps !== null"
              :x="placed.box.cx"
              :y="placed.box.y + 52"
              text-anchor="middle"
              fill="var(--c-ink-soft)"
              style="font-family: var(--font-gauge); font-size: 0.6rem"
            >
              {{ rate(placed.node.rps) }}/s · p99 {{ millis(placed.node.p99Ms) }}
            </text>
            <text
              v-else
              :x="placed.box.cx"
              :y="placed.box.y + 52"
              text-anchor="middle"
              fill="var(--c-slate)"
              style="font-size: 0.55rem; letter-spacing: 0.14em; text-transform: uppercase"
            >
              no data
            </text>

            <!-- pod dots -->
            <circle
              v-for="(dot, index) in podDots(placed.box, placed.node.pods.length)"
              :key="`${placed.node.name}-pod-${index}`"
              :cx="dot.x"
              :cy="dot.y"
              r="3.2"
              :fill="podColour(placed.node, index)"
              stroke="var(--c-brass-dark)"
              stroke-width="0.7"
              style="cursor: help"
              @mouseenter.stop="enterPod(placed.node, index, $event)"
              @mousemove.stop="locate"
            />
          </g>
        </g>
      </svg>

      <!-- the particle layer, pixel-aligned with the SVG above -->
      <canvas
        ref="canvas"
        class="pointer-events-none absolute inset-0 h-full w-full"
        aria-hidden="true"
      />
    </div>

    <!-- tooltips -->
    <div
      v-if="hoveredNode !== null"
      class="plate pointer-events-none absolute z-20 max-w-xs p-2 text-[0.75rem] shadow-lg"
      :style="{ left: `${Math.min(tip.x + 12, 720)}px`, top: `${tip.y + 12}px` }"
    >
      <p class="engraved text-ink">{{ hoveredNode.display }}</p>
      <p class="typed mt-1 italic text-ink-faint">“{{ hoveredNode.epigraph }}”</p>
      <p class="typed mt-1 text-ink-soft">{{ hoveredNode.plainly }}</p>
      <dl class="mt-2 grid grid-cols-2 gap-x-3 gap-y-0.5 font-[family-name:var(--font-gauge)]">
        <dt class="text-ink-faint">requests/s</dt>
        <dd>{{ rate(hoveredNode.rps) }}</dd>
        <dt class="text-ink-faint">p50 / p99</dt>
        <dd>{{ millis(hoveredNode.p50Ms) }} / {{ millis(hoveredNode.p99Ms) }}</dd>
        <dt class="text-ink-faint">errors</dt>
        <dd>{{ percent(hoveredNode.errorRatio) }}</dd>
        <dt class="text-ink-faint">pods</dt>
        <dd>
          {{ hoveredNode.pods.length }}
          <template v-if="hoveredNode.desiredReplicas !== null">
            of {{ count(hoveredNode.desiredReplicas) }} desired
          </template>
        </dd>
        <dt class="text-ink-faint">health</dt>
        <dd>{{ healthLabel(hoveredNode.health) }}</dd>
      </dl>
    </div>

    <div
      v-else-if="hoveredPod !== null"
      class="plate pointer-events-none absolute z-20 max-w-xs p-2 text-[0.75rem] shadow-lg"
      :style="{ left: `${Math.min(tip.x + 12, 720)}px`, top: `${tip.y + 12}px` }"
    >
      <p class="engraved text-ink">
        {{ hoveredPod.node.pods[hoveredPod.index]?.name ?? 'pod' }}
      </p>
      <dl class="mt-1 grid grid-cols-2 gap-x-3 gap-y-0.5 font-[family-name:var(--font-gauge)]">
        <dt class="text-ink-faint">phase</dt>
        <dd>{{ hoveredPod.node.pods[hoveredPod.index]?.phase ?? 'no data' }}</dd>
        <dt class="text-ink-faint">ready</dt>
        <dd>{{ hoveredPod.node.pods[hoveredPod.index]?.ready === true ? 'yes' : 'no' }}</dd>
        <dt class="text-ink-faint">restarts</dt>
        <dd>{{ count(hoveredPod.node.pods[hoveredPod.index]?.restarts ?? null) }}</dd>
        <dt class="text-ink-faint">requests/s</dt>
        <dd>{{ rate(hoveredPod.node.pods[hoveredPod.index]?.rps ?? null) }}</dd>
        <dt class="text-ink-faint">p99</dt>
        <dd>{{ millis(hoveredPod.node.pods[hoveredPod.index]?.p99Ms ?? null) }}</dd>
        <dt class="text-ink-faint">errors</dt>
        <dd>{{ percent(hoveredPod.node.pods[hoveredPod.index]?.errorRatio ?? null) }}</dd>
      </dl>
    </div>

    <div
      v-else-if="hoveredEdge !== null"
      class="plate pointer-events-none absolute z-20 max-w-xs p-2 text-[0.75rem] shadow-lg"
      :style="{ left: `${Math.min(tip.x + 12, 720)}px`, top: `${tip.y + 12}px` }"
    >
      <p class="engraved text-ink">
        {{ hoveredEdge.source }} → {{ hoveredEdge.target }}
      </p>
      <p class="typed text-[0.68rem] text-ink-faint">
        observed as {{ hoveredEdge.via }} in the service graph
      </p>
      <dl class="mt-1 grid grid-cols-2 gap-x-3 gap-y-0.5 font-[family-name:var(--font-gauge)]">
        <dt class="text-ink-faint">requests/s</dt>
        <dd>{{ rate(hoveredEdge.rps) }}</dd>
        <dt class="text-ink-faint">p50 / p99</dt>
        <dd>{{ millis(hoveredEdge.p50Ms) }} / {{ millis(hoveredEdge.p99Ms) }}</dd>
        <dt class="text-ink-faint">errors</dt>
        <dd>{{ percent(hoveredEdge.errorRatio) }}</dd>
        <dt class="text-ink-faint">health</dt>
        <dd>{{ healthLabel(hoveredEdge.health) }}</dd>
      </dl>
    </div>

    <footer
      v-if="diagnostics"
      class="mt-2 border-t border-rule pt-2 font-[family-name:var(--font-gauge)] text-[0.68rem] text-ink-faint"
    >
      <span>{{ nodes.length }} registry nodes</span>
      ·
      <span>{{ activeEdgeCount }} of {{ edges.length }} edges carrying traffic</span>
      ·
      <span>{{ diag.particles }} particles</span>
      ·
      <span>{{ diag.frameMs.toFixed(1) }} ms/frame</span>
      ·
      <span>spawn ×{{ diag.multiplier.toFixed(2) }}</span>
      ·
      <span>{{ props.animate ? 'animating' : 'paused' }}</span>
    </footer>
  </div>
</template>
