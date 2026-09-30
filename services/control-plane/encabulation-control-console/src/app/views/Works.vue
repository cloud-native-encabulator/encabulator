<script setup lang="ts">
/**
 * The Works: the storefront, the live feeds, and the parts cage.
 *
 * The storefront is not a mock. A quote goes to the real quoter through the real
 * gateway with your own token; an order runs the real saga, reserves real parts,
 * and appears in the feeds below within seconds. If the depot is short of
 * marzlevanes you get a real 409, which is the point of having it here.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useWorksStore } from '@/stores/controls'
import { useLiveStore } from '@/stores/live'
import { useSessionStore } from '@/stores/session'
import { EMPTY_STATES } from '@/lib/lexicon'
import { count, money, percent } from '@/lib/format'
import Lexeme from '@/components/panel/Lexeme.vue'
import LiveFeed from '@/components/panel/LiveFeed.vue'
import Notice from '@/components/panel/Notice.vue'
import PanelPlate from '@/components/panel/PanelPlate.vue'
import Readout from '@/components/panel/Readout.vue'

const works = useWorksStore()
const live = useLiveStore()
const session = useSessionStore()

/** From design section 3. Base prices are the quoter's, shown for context only. */
const MODELS = [
  { id: 'turbo', name: 'Turbo Encabulator (1944)', basePrice: 1944, note: '4% base defect rate' },
  { id: 'retro', name: 'Retro Encabulator (Rockwell)', basePrice: 2749, note: '3% base defect rate' },
  { id: 'hyper', name: 'HyperEncabulator (SANS ICS)', basePrice: 4096, note: '6% base defect rate' },
]

const model = ref('turbo')
const quantity = ref(1)
const dingleArm = ref(false)
const trackId = ref('')

let poll: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  void works.loadParts()
  void works.loadOrders()
  poll = setInterval(() => {
    void works.loadParts()
  }, 5000)
})
onBeforeUnmount(() => {
  if (poll !== null) clearInterval(poll)
})

const signedIn = computed(() => !session.isGuest)
const restockLocked = computed(() => !session.canOperate)

const order = computed(() => ({
  model: model.value,
  quantity: quantity.value,
  options: { dingleArm: dingleArm.value },
}))

const restockAmounts = ref<Record<string, number>>({})
function restockAmount(sku: string): number {
  return restockAmounts.value[sku] ?? 500
}

const selectedModel = computed(() => MODELS.find((m) => m.id === model.value) ?? MODELS[0]!)
</script>

<template>
  <div class="flex flex-col gap-4">
    <Notice tone="error" :message="works.notice" />

    <div class="grid gap-4 xl:grid-cols-[0.85fr_1.15fr]">
      <!-- storefront -->
      <PanelPlate
        legend="Order desk"
        sublegend="Real orders, through the real gateway, with your own credentials"
      >
        <form class="flex flex-col gap-3" @submit.prevent="works.placeOrder(order)">
          <label class="flex flex-col gap-1">
            <span class="engraved">Model</span>
            <select v-model="model" class="field" :disabled="!signedIn">
              <option v-for="m in MODELS" :key="m.id" :value="m.id">
                {{ m.name }} — {{ money(m.basePrice) }}
              </option>
            </select>
            <span class="typed text-[0.7rem] text-ink-faint">{{ selectedModel.note }}</span>
          </label>

          <label class="flex flex-col gap-1">
            <span class="engraved">Quantity (1 to 10)</span>
            <input
              v-model.number="quantity"
              type="number"
              min="1"
              max="10"
              class="field"
              :disabled="!signedIn"
            />
            <span class="typed text-[0.7rem] text-ink-faint">
              Five or more earns a 5% volume discount from the quoter.
            </span>
          </label>

          <label class="flex items-start gap-2">
            <input
              v-model="dingleArm"
              type="checkbox"
              class="mt-1"
              :disabled="!signedIn"
            />
            <span>
              <span class="engraved">Fit a <Lexeme of="dingleArm" /></span>
              <span class="typed block text-[0.72rem] text-ink-faint">
                Adds one DNG-ARM part and {{ money(312) }}, and halves the defect rate by reducing
                sinusoidal repleneration.
              </span>
            </span>
          </label>

          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="knob"
              :disabled="!signedIn || works.busy"
              @click="works.getQuote(order)"
            >
              Quote
            </button>
            <button type="submit" class="knob" :disabled="!signedIn || works.busy">
              Place order
            </button>
          </div>

          <p v-if="!signedIn" class="typed text-[0.76rem] text-ink-soft">
            The order desk places real orders with your own token, so it needs you signed in. A
            guest can still watch every feed on this page.
          </p>
        </form>

        <template #footer>
          <div v-if="works.quote !== null" class="flex flex-col gap-2">
            <p class="engraved text-ink">Quote {{ works.quote.id }}</p>
            <div class="grid grid-cols-2 gap-2">
              <Readout label="Unit price" :value="money(works.quote.unitPrice ?? null)" size="sm" />
              <Readout label="Subtotal" :value="money(works.quote.subtotal ?? null)" size="sm" />
              <Readout label="Total" :value="money(works.quote.total ?? null)" size="sm" />
              <Readout
                label="Served from cache"
                :value="works.quote.cached === true ? 'yes' : 'no'"
                size="sm"
                title="True when the quote-cache flag is on and an identical quote was computed in the last sixty seconds"
              />
            </div>
            <ul
              v-if="works.quote.surcharges !== undefined && works.quote.surcharges.length > 0"
              class="typed text-[0.76rem] text-ink-soft"
            >
              <li v-for="s in works.quote.surcharges" :key="s.code">
                {{ s.code }}: {{ money(s.amount) }}
              </li>
            </ul>
          </div>
          <div v-else-if="works.placed !== null" class="flex flex-col gap-1">
            <p class="engraved text-ink">Order {{ works.placed.id }}</p>
            <p class="typed text-[0.78rem] text-ink-soft">
              status {{ works.placed.status ?? 'no data' }}
              <template v-if="works.placed.stage">· stage {{ works.placed.stage }}</template>
            </p>
          </div>
          <p v-else class="typed text-[0.7rem] text-ink-faint">
            A quote or a placed order appears here. Orders then show up in the feeds beside this
            plate as the factory works on them.
          </p>
        </template>
      </PanelPlate>

      <!-- feeds -->
      <div class="flex flex-col gap-4">
        <LiveFeed
          legend="Orders"
          sublegend="Every status change the ledger has recorded"
          :items="live.orders"
          :source="live.feedSources['datastore'] ?? null"
          :empty-note="EMPTY_STATES.noOrders"
          :columns="[
            { key: 'order', label: 'Order' },
            { key: 'model', label: 'Model' },
            { key: 'quantity', label: 'Qty' },
            { key: 'status', label: 'Status' },
            { key: 'stage', label: 'Stage' },
          ]"
        />

        <LiveFeed
          legend="Side fumbling"
          sublegend="Incidents the detector raised, with the metric and score that raised them"
          :items="live.incidents"
          :source="live.feedSources['detector'] ?? null"
          :empty-note="EMPTY_STATES.noIncidents"
          :columns="[
            { key: 'serial', label: 'Serial' },
            { key: 'stage', label: 'Stage' },
            { key: 'metric', label: 'Metric' },
            { key: 'score', label: 'z' },
            { key: 'strict', label: 'Strict' },
          ]"
        />

        <LiveFeed
          legend="Nofer trunnions"
          sublegend="Shipments dispatched and delivered"
          :items="live.shipments"
          :source="live.feedSources['dispatch'] ?? null"
          :empty-note="EMPTY_STATES.noShipments"
          :columns="[
            { key: 'order', label: 'Order' },
            { key: 'carrier', label: 'Carrier' },
            { key: 'tracking', label: 'Tracking' },
            { key: 'status', label: 'Status' },
          ]"
        />
      </div>
    </div>

    <!-- parts cage -->
    <PanelPlate
      legend="Parts cage"
      sublegend="Stock, reservations, and how many each encabulator consumes"
      :source="works.partsError === null ? null : { ok: false, detail: works.partsError }"
      :empty="works.parts.length === 0"
      :empty-note="EMPTY_STATES.noParts"
    >
      <table class="w-full border-collapse text-[0.8rem]">
        <thead>
          <tr class="border-b border-rule text-left">
            <th class="engraved py-1 pr-3">SKU</th>
            <th class="engraved py-1 pr-3">Part</th>
            <th class="engraved py-1 pr-3">Stock</th>
            <th class="engraved py-1 pr-3">Reserved</th>
            <th class="engraved py-1 pr-3">Per unit</th>
            <th class="engraved py-1 pr-3">Of cap</th>
            <th class="engraved py-1">Restock</th>
          </tr>
        </thead>
        <tbody class="typed">
          <tr
            v-for="part in works.parts"
            :key="part.sku"
            class="border-b border-rule/40 last:border-0"
          >
            <td class="py-1 pr-3 font-[family-name:var(--font-gauge)]">{{ part.sku }}</td>
            <td class="py-1 pr-3">{{ part.name }}</td>
            <td
              class="py-1 pr-3 font-[family-name:var(--font-gauge)]"
              :class="part.stock !== null && part.stock < 50 ? 'text-lacquer' : ''"
            >
              {{ count(part.stock) }}
            </td>
            <td class="py-1 pr-3 font-[family-name:var(--font-gauge)]">{{ count(part.reserved) }}</td>
            <td class="py-1 pr-3 font-[family-name:var(--font-gauge)]">{{ count(part.perUnit) }}</td>
            <td class="py-1 pr-3">
              <span
                v-if="part.stock === null"
                class="no-data"
                >no data</span
              >
              <span v-else class="font-[family-name:var(--font-gauge)]">{{
                percent(part.stock / 5000)
              }}</span>
            </td>
            <td class="py-1">
              <span class="flex items-center gap-1">
                <input
                  :value="restockAmount(part.sku)"
                  type="number"
                  min="1"
                  max="5000"
                  class="field w-20"
                  :disabled="restockLocked"
                  @input="
                    restockAmounts[part.sku] = Number(($event.target as HTMLInputElement).value)
                  "
                />
                <button
                  type="button"
                  class="knob"
                  :disabled="restockLocked || works.busy"
                  @click="works.restock(part.sku, restockAmount(part.sku))"
                >
                  Add
                </button>
              </span>
            </td>
          </tr>
        </tbody>
      </table>

      <template #footer>
        <p class="typed text-[0.7rem] text-ink-faint">
          The <Lexeme of="amuliteFoundry" /> tops every SKU up every ten seconds, at the rate the
          <code>amulite-foundry-rate</code> flag sets, to a cap of 5,000 per SKU. Setting that flag
          to zero is how the marzlevane-shortage scenario starves the line.
        </p>
      </template>
    </PanelPlate>

    <!-- tracking -->
    <PanelPlate legend="Track an order" sublegend="Through the ledger, including its timeline" dense>
      <form class="flex flex-wrap items-end gap-2" @submit.prevent="works.trackOrder(trackId)">
        <label class="flex flex-col gap-1">
          <span class="engraved">Order id</span>
          <input v-model="trackId" class="field w-80" placeholder="ord_…" />
        </label>
        <button type="submit" class="knob" :disabled="trackId === ''">Look up</button>
      </form>

      <div v-if="works.placed !== null && works.placed.timeline !== undefined" class="mt-3">
        <ol class="flex flex-col gap-1">
          <li
            v-for="(entry, index) in works.placed.timeline"
            :key="`${entry.time}-${index}`"
            class="typed flex gap-2 text-[0.78rem]"
          >
            <span class="shrink-0 font-[family-name:var(--font-gauge)] text-[0.7rem] text-ink-faint">
              {{ entry.time }}
            </span>
            <span class="engraved">{{ entry.type }}</span>
            <span v-if="entry.detail" class="text-ink-soft">{{ entry.detail }}</span>
          </li>
        </ol>
      </div>
    </PanelPlate>
  </div>
</template>
