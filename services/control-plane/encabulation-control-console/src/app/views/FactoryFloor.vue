<script setup lang="ts">
/**
 * The Factory Floor: the topology, the headline gauges, and the narration tape.
 *
 * Every gauge on this page is fed by the `headline` stream event, which the BFF
 * builds from the ledger, the detector, the depot and the recorder. When one of
 * those is unreachable, the gauge it feeds reads NO DATA and the plate footer
 * names the upstream. Nothing here degrades to a plausible number.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useLiveStore } from '@/stores/live'
import { EMPTY_STATES } from '@/lib/lexicon'
import { count, decimal, percent, rate } from '@/lib/format'
import GaugeDial from '@/components/panel/GaugeDial.vue'
import Lexeme from '@/components/panel/Lexeme.vue'
import NarrationTicker from '@/components/panel/NarrationTicker.vue'
import PanelPlate from '@/components/panel/PanelPlate.vue'
import Readout from '@/components/panel/Readout.vue'
import TopologyPanel from '@/components/topology/TopologyPanel.vue'

const live = useLiveStore()

/** Animate only while the tab is in front. */
const visible = ref(true)
function onVisibility(): void {
  visible.value = !document.hidden
}
onMounted(() => document.addEventListener('visibilitychange', onVisibility))
onBeforeUnmount(() => document.removeEventListener('visibilitychange', onVisibility))

const headline = computed(() => live.headline)

const sources = computed(() => live.headline?.sources ?? {})
const prometheusStatus = computed(() => live.sources.prometheus)
const kubernetesStatus = computed(() => live.sources.kubernetes)

const totalRps = computed(() => {
  const values = live.nodes.map((n) => n.rps).filter((v): v is number => v !== null)
  return values.length === 0 ? null : values.reduce((a, b) => a + b, 0)
})

const worstP99 = computed(() => {
  const values = live.nodes.map((n) => n.p99Ms).filter((v): v is number => v !== null)
  return values.length === 0 ? null : Math.max(...values)
})

const readyPods = computed(() => {
  const pods = live.nodes.flatMap((n) => n.pods)
  if (pods.length === 0) return null
  return pods.filter((p) => p.ready).length
})
const totalPods = computed(() => (live.podCount === 0 ? null : live.podCount))
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- headline gauges -->
    <section class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
      <PanelPlate
        legend="Encabulation rate"
        sublegend="Orders accepted per minute, from the ledger’s five-minute throughput"
        :source="sources['datastore'] ?? null"
      >
        <GaugeDial
          label="Orders / min"
          :value="headline?.ordersPerMin ?? null"
          :display="decimal(headline?.ordersPerMin ?? null, 1)"
          :max="120"
          unit="per minute"
          title="Orders accepted per minute, from hydrocoptic-datastore /v1/stats"
        />
      </PanelPlate>

      <PanelPlate
        legend="Units on the casing line"
        sublegend="Orders in fabricating or refabricating, from the ledger"
        :source="sources['datastore'] ?? null"
      >
        <GaugeDial
          label="In fabrication"
          :value="headline?.unitsInFabrication ?? null"
          :display="count(headline?.unitsInFabrication ?? null)"
          :max="60"
          :warn-at="30"
          :fault-at="48"
          unit="orders"
          title="Orders whose status is fabricating or refabricating"
        />
      </PanelPlate>

      <PanelPlate
        legend="Refabrication rate"
        sublegend="Five-minute inspection failure rate, from the detector"
        :source="sources['detector'] ?? null"
      >
        <GaugeDial
          label="Side fumbling"
          :value="headline?.refabricationRate === null || headline?.refabricationRate === undefined
            ? null
            : headline.refabricationRate * 100"
          :display="percent(headline?.refabricationRate ?? null)"
          :max="40"
          :warn-at="8"
          :fault-at="20"
          unit="% of units"
          title="Units the side-fumbling detector rejected, over the last five minutes"
        />
      </PanelPlate>

      <PanelPlate
        legend="Parts cage"
        sublegend="Total stock, and the SKU closest to running out"
        :source="sources['depot'] ?? null"
      >
        <div class="flex flex-col gap-2">
          <Readout
            label="Total stock"
            :value="count(headline?.partsTotal ?? null)"
            unit="units"
            title="Sum of every SKU's stock in the prefabulated amulite depot"
          />
          <Readout
            label="Shortest SKU"
            :value="
              headline?.partsLowestSku === null || headline?.partsLowestSku === undefined
                ? ''
                : `${headline.partsLowestSku} · ${count(headline.partsLowestStock)}`
            "
            size="sm"
            title="The SKU with the least stock, and how much is left"
          />
        </div>
      </PanelPlate>
    </section>

    <!-- the topology -->
    <TopologyPanel :snapshot="live.topology" :animate="visible" />

    <!-- narration -->
    <NarrationTicker :narration="live.narration" :scenario="live.scenario" />

    <!-- secondary readouts and source honesty -->
    <section class="grid gap-3 lg:grid-cols-3">
      <PanelPlate
        legend="Aggregate skor motion"
        :sublegend="'Summed across every pod Prometheus has scraped'"
        :source="prometheusStatus"
      >
        <div class="grid grid-cols-2 gap-3">
          <Readout
            label="Requests / s"
            :value="rate(totalRps)"
            title="Sum of per-pod http_server_request_duration_seconds_count rates"
          />
          <Readout
            label="Worst pod p99"
            :value="
              worstP99 === null ? '' : `${worstP99 < 1000 ? worstP99.toFixed(0) : (worstP99 / 1000).toFixed(2)}${worstP99 < 1000 ? ' ms' : ' s'}`
            "
            title="The highest p99 of any pod in the namespace"
          />
          <Readout
            label="Units delivered (5m)"
            :value="count(headline?.unitsDelivered ?? null)"
            title="From the ledger's five-minute throughput"
          />
          <Readout
            label="Repleneration mean"
            :value="decimal(headline?.replenerationMean ?? null, 3)"
            title="Mean sinusoidal repleneration amplitude, nominally 1.0"
          />
        </div>
        <template #footer>
          <p class="typed text-[0.7rem] text-ink-faint">
            <Lexeme of="skorMotion" /> is this console’s label for requests per second.
            <Lexeme of="repleneration" /> is the amplitude the casing line writes after every stage.
          </p>
        </template>
      </PanelPlate>

      <PanelPlate
        legend="Pods"
        sublegend="From the Kubernetes API, read only"
        :source="kubernetesStatus"
        :empty="live.podCount === 0"
        :empty-note="EMPTY_STATES.noPods"
      >
        <div class="grid grid-cols-2 gap-3">
          <Readout label="Ready" :value="count(readyPods)" />
          <Readout label="Total" :value="count(totalPods)" />
        </div>
        <template #footer>
          <p class="typed text-[0.7rem] text-ink-faint">
            Listed with <code>app.kubernetes.io/part-of=encabulator</code>. The console never
            mutates the cluster; it asks the malaise injection engine to.
          </p>
        </template>
      </PanelPlate>

      <PanelPlate
        legend="What is not drawn"
        sublegend="Service-graph peers outside the registry"
        :empty="live.unmapped.length === 0"
        empty-note="Every edge the service graph reported maps onto a registry node. Nothing is being hidden."
      >
        <ul class="typed space-y-1 text-[0.78rem]">
          <li v-for="edge in live.unmapped" :key="`${edge.client}->${edge.server}`">
            {{ edge.client }} → {{ edge.server }}
            <span class="text-ink-faint">({{ rate(edge.rps) }} req/s)</span>
          </li>
        </ul>
        <template #footer>
          <p class="typed text-[0.7rem] text-ink-faint">
            These are real calls in the service graph whose endpoints are not encabulator
            services. They are listed rather than drawn, and never invented as nodes.
          </p>
        </template>
      </PanelPlate>
    </section>
  </div>
</template>
