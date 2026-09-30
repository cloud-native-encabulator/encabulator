<script setup lang="ts">
/**
 * The console shell: the nameplate across the top, the page tabs, the session
 * badge, the enamel/control-room switch, and the links out to Grafana and Jaeger.
 *
 * The shell owns two cross-cutting behaviours:
 *   - it attaches the single SSE stream, so every page shares one connection;
 *   - it releases that stream when the tab is hidden, which is what makes the
 *     BFF stop polling upstreams for a console nobody is looking at.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink, RouterView } from 'vue-router'
import { useLiveStore } from '@/stores/live'
import { useSessionStore } from '@/stores/session'
import { ago } from '@/lib/format'
import StatusLamp from '@/components/panel/StatusLamp.vue'

const live = useLiveStore()
const session = useSessionStore()

const PAGES = [
  { to: '/', label: 'Factory Floor' },
  { to: '/grammeters', label: 'Grammeters' },
  { to: '/malaise', label: 'Malaise' },
  { to: '/scenarios', label: 'Scenarios' },
  { to: '/girdlespring', label: 'Girdlespring' },
  { to: '/works', label: 'Works' },
  { to: '/administration', label: 'Administration' },
]

const controlRoom = ref(false)
const now = ref(Date.now())
let release: (() => void) | null = null
let clockTimer: ReturnType<typeof setInterval> | null = null

const THEME_KEY = 'encabulator.finish'

function applyFinish(): void {
  const root = document.documentElement
  root.classList.toggle('control-room', controlRoom.value)
  root.classList.toggle('enamel', !controlRoom.value)
  try {
    localStorage.setItem(THEME_KEY, controlRoom.value ? 'control-room' : 'enamel')
  } catch {
    // A browser with storage disabled still gets a working panel.
  }
}

function toggleFinish(): void {
  controlRoom.value = !controlRoom.value
  applyFinish()
}

function onVisibility(): void {
  live.setVisible(!document.hidden)
}

onMounted(() => {
  try {
    const stored = localStorage.getItem(THEME_KEY)
    if (stored === 'control-room') controlRoom.value = true
    else if (stored === null)
      controlRoom.value = window.matchMedia?.('(prefers-color-scheme: dark)').matches === true
  } catch {
    // ignore
  }
  applyFinish()

  void session.load()
  release = live.attach()
  void live.primeTopology()
  document.addEventListener('visibilitychange', onVisibility)
  clockTimer = setInterval(() => {
    now.value = Date.now()
  }, 1000)
})

onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', onVisibility)
  if (clockTimer !== null) clearInterval(clockTimer)
  release?.()
  release = null
})

const streamLamp = computed(() => {
  switch (live.state) {
    case 'open':
      return 'healthy' as const
    case 'connecting':
    case 'retrying':
      return 'degraded' as const
    default:
      return 'nodata' as const
  }
})

const lastSeen = computed(() => ago(live.lastEventAt, now.value))

async function signOut(): Promise<void> {
  await session.signOut()
}
</script>

<template>
  <div class="min-h-screen">
    <!-- nameplate -->
    <header class="border-b-2 border-brass-dark bg-enamel">
      <div class="mx-auto flex max-w-[1600px] flex-wrap items-end justify-between gap-3 px-4 pt-3">
        <div>
          <p class="engraved text-[0.6rem] text-brass-dark">The Encabulator Works · Est. 1944</p>
          <h1 class="font-[family-name:var(--font-plate)] text-2xl font-semibold tracking-wide">
            Encabulation Control Console
          </h1>
          <p class="typed text-[0.72rem] text-ink-faint">
            Every reading on this panel is measured. Nothing is estimated, and nothing is invented.
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-3 pb-1">
          <span class="flex items-center gap-1.5">
            <StatusLamp :health="streamLamp" />
            <span class="engraved">live feed {{ live.state }}</span>
            <span class="text-[0.65rem] text-ink-faint typed">· last frame {{ lastSeen }}</span>
          </span>

          <a
            v-if="session.links.grafana !== null"
            :href="session.links.grafana"
            target="_blank"
            rel="noreferrer noopener"
            class="knob"
            >Grafana</a
          >
          <a
            v-if="session.links.jaeger !== null"
            :href="session.links.jaeger"
            target="_blank"
            rel="noreferrer noopener"
            class="knob"
            >Jaeger</a
          >

          <button type="button" class="knob" @click="toggleFinish">
            {{ controlRoom ? 'Enamel' : 'Control room' }}
          </button>

          <div class="flex items-center gap-2 border-l border-rule pl-3">
            <div class="text-right leading-tight">
              <p class="engraved text-ink">{{ session.session.displayName }}</p>
              <p class="text-[0.65rem] text-ink-faint typed">{{ session.roleLabel }}</p>
            </div>
            <RouterLink v-if="session.isGuest" to="/sign-in" class="knob">Sign in</RouterLink>
            <button v-else type="button" class="knob" @click="signOut">Sign out</button>
          </div>
        </div>
      </div>

      <nav class="mx-auto mt-2 max-w-[1600px] px-4" aria-label="Console pages">
        <ul class="flex flex-wrap gap-1">
          <li v-for="page in PAGES" :key="page.to">
            <RouterLink
              :to="page.to"
              class="inline-block border border-b-0 border-rule px-3 py-1.5 engraved transition-colors hover:text-ink"
              :class="
                $route.path === page.to
                  ? 'bg-plate text-ink'
                  : 'bg-enamel-deep text-ink-faint'
              "
            >
              {{ page.label }}
            </RouterLink>
          </li>
        </ul>
      </nav>
    </header>

    <main class="mx-auto max-w-[1600px] px-4 py-4">
      <p
        v-if="session.isGuest"
        class="typed mb-3 rounded border border-brass/40 bg-plate px-3 py-2 text-[0.78rem] text-ink-soft"
      >
        <span class="engraved text-brass-dark">Guest.</span>
        You are a viewer: every gauge is live, every control is locked. Sign in as an operator to
        drive traffic, induce malaise, run a scenario, or flip a flag.
      </p>

      <RouterView />
    </main>

    <footer
      class="mx-auto max-w-[1600px] px-4 pb-6 pt-2 text-[0.68rem] text-ink-faint typed"
    >
      <p>
        “The original machine had a base-plate of prefabulated amulite, surmounted by a malleable
        logarithmic casing in such a way that the two main spurving bearings were in a direct line
        with the pentametric fan.” · J. H. Quick, 1944. The names are nonsense; the system is not.
      </p>
    </footer>
  </div>
</template>
