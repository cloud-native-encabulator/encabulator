<script setup lang="ts">
/**
 * A live feed table: orders, incidents, or shipments. Every column is a field the
 * upstream supplied; a field it did not supply reads "no data".
 */
import { ago } from '@/lib/format'
import type { FeedItem, SourceStatus } from '@shared/topology'
import PanelPlate from './PanelPlate.vue'

const props = withDefaults(
  defineProps<{
    legend: string
    sublegend?: string
    items: FeedItem[]
    columns: { key: string; label: string }[]
    source?: SourceStatus | null
    emptyNote: string
    limit?: number
  }>(),
  { sublegend: '', source: null, limit: 12 },
)
</script>

<template>
  <PanelPlate
    :legend="props.legend"
    :sublegend="props.sublegend"
    :source="props.source"
    :empty="props.items.length === 0"
    :empty-note="props.emptyNote"
  >
    <div class="overflow-x-auto">
      <table class="w-full border-collapse text-[0.78rem]">
        <thead>
          <tr class="border-b border-rule text-left">
            <th v-for="col in props.columns" :key="col.key" class="engraved py-1 pr-3">
              {{ col.label }}
            </th>
            <th class="engraved py-1 text-right">When</th>
          </tr>
        </thead>
        <tbody class="typed">
          <tr
            v-for="item in props.items.slice(0, props.limit)"
            :key="item.id"
            class="border-b border-rule/50 last:border-0"
          >
            <td
              v-for="col in props.columns"
              :key="col.key"
              class="py-1 pr-3 align-top"
              :class="item.fields[col.key] === 'no data' ? 'no-data' : ''"
            >
              {{ item.fields[col.key] ?? 'no data' }}
            </td>
            <td class="py-1 text-right align-top text-ink-faint whitespace-nowrap">
              {{ ago(item.at) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </PanelPlate>
</template>
