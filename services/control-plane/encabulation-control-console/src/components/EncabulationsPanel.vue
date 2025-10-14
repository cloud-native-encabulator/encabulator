<script setup lang="ts">
import { ref, onMounted } from 'vue'
import axios from 'axios'

const API_BASE = import.meta.env.VITE_API_BASE || ''

interface Encabulation {
  id: string
  grammeters: number
  prefabulation: boolean
  sinusoidalMode: string
  encabulationFactor: number
  dingleArmPosition: number
  prefabulationRate: number
  createdAt: string
}

const encabulations = ref<Encabulation[]>([])
const isLoading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const fetchEncabulations = async () => {
  isLoading.value = true
  try {
    const response = await axios.get(`${API_BASE}/api/encabulations`, {
      params: { page: page.value, pageSize: pageSize.value }
    })
    encabulations.value = response.data.encabulations || []
    total.value = response.data.total || 0
  } catch (error) {
    console.error('Failed to fetch encabulations:', error)
  } finally {
    isLoading.value = false
  }
}

const formatDate = (dateString: string) => {
  const date = new Date(dateString)
  return date.toLocaleString()
}

const getModeColor = (mode: string) => {
  switch (mode) {
    case 'STANDARD': return 'text-blue-400'
    case 'INVERSE': return 'text-purple-400'
    case 'LOGARITHMIC': return 'text-green-400'
    default: return 'text-gray-400'
  }
}

const nextPage = () => {
  if (page.value * pageSize.value < total.value) {
    page.value++
    fetchEncabulations()
  }
}

const prevPage = () => {
  if (page.value > 1) {
    page.value--
    fetchEncabulations()
  }
}

onMounted(() => {
  fetchEncabulations()
})
</script>

<template>
  <div class="bg-surface border border-primary/30 rounded-lg p-6 shadow-lg">
    <div class="flex items-center justify-between mb-4">
      <h3 class="text-lg font-bold text-primary">Recent Encabulations</h3>
      <button
        @click="fetchEncabulations"
        :disabled="isLoading"
        class="text-sm text-primary hover:text-primary/80 disabled:text-text-muted flex items-center gap-1"
      >
        <svg class="w-4 h-4" :class="{ 'animate-spin': isLoading }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
        Refresh
      </button>
    </div>

    <div v-if="isLoading && encabulations.length === 0" class="text-center py-8 text-text-muted">
      Loading encabulations...
    </div>

    <div v-else-if="encabulations.length === 0" class="text-center py-8 text-text-muted">
      No encabulations found. Start traffic generation to create some!
    </div>

    <div v-else class="space-y-3">
      <div
        v-for="enc in encabulations"
        :key="enc.id"
        class="bg-surface-hover border border-text-muted/20 rounded-lg p-4 hover:border-primary/30 transition-colors"
      >
        <div class="flex items-start justify-between mb-2">
          <div class="flex items-center gap-2">
            <span class="text-xs font-mono text-text-muted">{{ enc.id.slice(0, 8) }}</span>
            <span 
              :class="getModeColor(enc.sinusoidalMode)"
              class="text-xs font-semibold px-2 py-0.5 rounded bg-surface"
            >
              {{ enc.sinusoidalMode }}
            </span>
            <span 
              v-if="enc.prefabulation"
              class="text-xs font-semibold px-2 py-0.5 rounded bg-cyan-500/20 text-cyan-400"
            >
              PREFAB
            </span>
          </div>
          <span class="text-xs text-text-muted">{{ formatDate(enc.createdAt) }}</span>
        </div>

        <div class="grid grid-cols-4 gap-3 text-sm">
          <div>
            <div class="text-text-muted text-xs">Grammeters</div>
            <div class="font-mono font-semibold">{{ enc.grammeters }}</div>
          </div>
          <div>
            <div class="text-text-muted text-xs">Factor</div>
            <div class="font-mono font-semibold text-primary">{{ enc.encabulationFactor }}</div>
          </div>
          <div>
            <div class="text-text-muted text-xs">Dingle Arm</div>
            <div class="font-mono font-semibold text-green-400">{{ enc.dingleArmPosition }}</div>
          </div>
          <div>
            <div class="text-text-muted text-xs">Prefab Rate</div>
            <div class="font-mono font-semibold text-purple-400">{{ enc.prefabulationRate }}</div>
          </div>
        </div>
      </div>
    </div>

    <!-- Pagination -->
    <div v-if="total > pageSize" class="flex items-center justify-between mt-4 pt-4 border-t border-text-muted/20">
      <div class="text-sm text-text-muted">
        Showing {{ (page - 1) * pageSize + 1 }}-{{ Math.min(page * pageSize, total) }} of {{ total }}
      </div>
      <div class="flex gap-2">
        <button
          @click="prevPage"
          :disabled="page === 1"
          class="px-3 py-1 text-sm rounded bg-surface-hover hover:bg-surface-active disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Previous
        </button>
        <button
          @click="nextPage"
          :disabled="page * pageSize >= total"
          class="px-3 py-1 text-sm rounded bg-surface-hover hover:bg-surface-active disabled:opacity-50 disabled:cursor-not-allowed"
        >
          Next
        </button>
      </div>
    </div>
  </div>
</template>
