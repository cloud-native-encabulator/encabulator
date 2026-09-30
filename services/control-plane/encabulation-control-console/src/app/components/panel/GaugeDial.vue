<script setup lang="ts">
/**
 * A 240° moving-coil gauge: brass bezel, dark green glass, phosphor needle,
 * engraved scale.
 *
 * When `value` is null the needle is not drawn at all and the face reads NO
 * DATA. It does not rest at zero, because a needle at zero is a measurement and
 * "we did not measure" is not zero.
 */
import { computed } from 'vue'
import { NO_DATA, isMissing } from '@/lib/format'
import type { Measure } from '@shared/topology'

const props = withDefaults(
  defineProps<{
    label: string
    value: Measure
    /** Formatted reading, e.g. "12.4 /min". The parent formats; we display. */
    display: string
    min?: number
    max: number
    /** Values at or above this arc segment are amber. */
    warnAt?: number | null
    /** Values at or above this are lacquer red. */
    faultAt?: number | null
    unit?: string
    /** Tooltip: the plain-English meaning of the term on the plate. */
    title?: string
    size?: number
  }>(),
  { min: 0, warnAt: null, faultAt: null, unit: '', title: '', size: 148 },
)

const SWEEP = 240
const START = -210 // 0 at lower-left, sweeping clockwise to lower-right

const radius = computed(() => props.size / 2)
const dialRadius = computed(() => radius.value - 14)

function angleFor(value: number): number {
  const span = props.max - props.min
  const clamped = Math.min(props.max, Math.max(props.min, value))
  const fraction = span <= 0 ? 0 : (clamped - props.min) / span
  return START + fraction * SWEEP
}

function pointAt(angleDeg: number, r: number): { x: number; y: number } {
  const rad = (angleDeg * Math.PI) / 180
  return { x: radius.value + Math.cos(rad) * r, y: radius.value + Math.sin(rad) * r }
}

function arc(fromValue: number, toValue: number, r: number): string {
  const a = angleFor(fromValue)
  const b = angleFor(toValue)
  const start = pointAt(a, r)
  const end = pointAt(b, r)
  const large = Math.abs(b - a) > 180 ? 1 : 0
  return `M ${start.x} ${start.y} A ${r} ${r} 0 ${large} 1 ${end.x} ${end.y}`
}

const faceArc = computed(() => arc(props.min, props.max, dialRadius.value))
const warnArc = computed(() =>
  props.warnAt === null ? null : arc(props.warnAt, props.faultAt ?? props.max, dialRadius.value),
)
const faultArc = computed(() =>
  props.faultAt === null ? null : arc(props.faultAt, props.max, dialRadius.value),
)

const ticks = computed(() => {
  const out: { x1: number; y1: number; x2: number; y2: number; major: boolean }[] = []
  const steps = 12
  for (let i = 0; i <= steps; i += 1) {
    const value = props.min + ((props.max - props.min) * i) / steps
    const angle = angleFor(value)
    const major = i % 3 === 0
    const outer = pointAt(angle, dialRadius.value)
    const inner = pointAt(angle, dialRadius.value - (major ? 9 : 5))
    out.push({ x1: inner.x, y1: inner.y, x2: outer.x, y2: outer.y, major })
  }
  return out
})

const needle = computed(() => {
  if (isMissing(props.value)) return null
  return pointAt(angleFor(props.value as number), dialRadius.value - 12)
})

const band = computed<'ok' | 'warn' | 'fault'>(() => {
  if (isMissing(props.value)) return 'ok'
  const v = props.value as number
  if (props.faultAt !== null && v >= props.faultAt) return 'fault'
  if (props.warnAt !== null && v >= props.warnAt) return 'warn'
  return 'ok'
})

const needleColour = computed(() =>
  band.value === 'fault'
    ? 'var(--c-lacquer)'
    : band.value === 'warn'
      ? 'var(--c-amber)'
      : 'var(--c-phosphor)',
)
</script>

<template>
  <figure class="flex flex-col items-center gap-1" :title="props.title">
    <svg
      :width="props.size"
      :height="props.size * 0.78"
      :viewBox="`0 0 ${props.size} ${props.size * 0.78}`"
      role="img"
      :aria-label="`${props.label}: ${props.display}`"
    >
      <!-- brass bezel -->
      <circle
        :cx="radius"
        :cy="radius"
        :r="radius - 2"
        fill="var(--c-gauge-face)"
        stroke="var(--c-gauge-rim)"
        stroke-width="3"
      />
      <!-- scale -->
      <path :d="faceArc" fill="none" stroke="var(--c-slate)" stroke-width="2" opacity="0.5" />
      <path
        v-if="warnArc !== null"
        :d="warnArc"
        fill="none"
        stroke="var(--c-amber)"
        stroke-width="3"
        opacity="0.75"
      />
      <path
        v-if="faultArc !== null"
        :d="faultArc"
        fill="none"
        stroke="var(--c-lacquer)"
        stroke-width="3"
        opacity="0.85"
      />
      <line
        v-for="(tick, index) in ticks"
        :key="index"
        :x1="tick.x1"
        :y1="tick.y1"
        :x2="tick.x2"
        :y2="tick.y2"
        stroke="var(--c-phosphor)"
        :stroke-width="tick.major ? 1.8 : 1"
        :opacity="tick.major ? 0.75 : 0.4"
      />
      <!-- needle, drawn only when there is something to point at -->
      <template v-if="needle !== null">
        <line
          :x1="radius"
          :y1="radius"
          :x2="needle.x"
          :y2="needle.y"
          :stroke="needleColour"
          stroke-width="2.4"
          stroke-linecap="round"
        />
        <circle :cx="radius" :cy="radius" r="4" fill="var(--c-brass-bright)" />
      </template>
      <text
        v-else
        :x="radius"
        :y="radius + 4"
        text-anchor="middle"
        class="fill-slate"
        style="font-size: 0.6rem; letter-spacing: 0.16em"
      >
        {{ NO_DATA.toUpperCase() }}
      </text>

      <!-- reading -->
      <text
        :x="radius"
        :y="radius + dialRadius * 0.62"
        text-anchor="middle"
        fill="var(--c-phosphor)"
        style="font-family: var(--font-gauge); font-size: 0.82rem; font-variant-numeric: tabular-nums"
      >
        {{ props.display }}
      </text>
    </svg>
    <figcaption class="engraved text-center leading-tight">
      {{ props.label }}
      <span v-if="props.unit !== ''" class="text-ink-faint">({{ props.unit }})</span>
    </figcaption>
  </figure>
</template>
