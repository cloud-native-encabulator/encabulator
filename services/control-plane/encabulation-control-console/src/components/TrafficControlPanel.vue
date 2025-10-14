<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import axios from 'axios'

const API_BASE = import.meta.env.VITE_API_BASE || ''

interface TrafficStatus {
  active: boolean
  target?: string
  pattern?: string
  requestsSent: number
  successRate: number
  avgLatency: number
}

const isActive = ref(false)
const isLoading = ref(false)
const fluorescentScoreMotion = ref(50) // TPS throttle (0-100)
const magnetoReluctance = ref(0) // Latency injection (0-100)
const capacitiveDiractance = ref(50) // Payload complexity (0-100)
const pattern = ref<'steady' | 'burst' | 'ramp' | 'sine'>('steady')
const stats = ref<TrafficStatus>({
  active: false,
  requestsSent: 0,
  successRate: 0,
  avgLatency: 0
})

// Calculate estimated TPS based on fluorescent score motion
const estimatedTPS = computed(() => {
  // Range: 0.5 TPS to 1000 TPS (exponential scaling)
  const minTPS = 0.5
  const maxTPS = 1000
  const normalizedFSM = fluorescentScoreMotion.value / 100
  const tps = minTPS + (maxTPS - minTPS) * Math.pow(normalizedFSM, 2)
  return tps >= 100 ? Math.round(tps).toString() : tps.toFixed(1)
})

// Calculate estimated latency from magneto reluctance
const estimatedLatency = computed(() => {
  // Range: 0ms to 500ms base + jitter
  const baseLatency = (magnetoReluctance.value / 100) * 500
  return Math.round(baseLatency)
})

let statusInterval: number | null = null

const startTraffic = async () => {
  isLoading.value = true
  try {
    await axios.post(`${API_BASE}/api/actions/synchronize-grammeters`, {
      fluorescentScoreMotion: fluorescentScoreMotion.value,
      magnetoReluctance: magnetoReluctance.value,
      capacitiveDiractance: capacitiveDiractance.value,
      pattern: pattern.value
    })
    isActive.value = true
    startStatusPolling()
  } catch (error) {
    console.error('Failed to start traffic:', error)
  } finally {
    isLoading.value = false
  }
}

const stopTraffic = async () => {
  isLoading.value = true
  try {
    await axios.post(`${API_BASE}/api/actions/engage-dampeners`)
    isActive.value = false
    stopStatusPolling()
  } catch (error) {
    console.error('Failed to stop traffic:', error)
  } finally {
    isLoading.value = false
  }
}

const updateParameters = async () => {
  if (!isActive.value) return
  
  try {
    await axios.post(`${API_BASE}/api/cardinal-grammeter/update`, {
      fluorescentScoreMotion: fluorescentScoreMotion.value,
      magnetoReluctance: magnetoReluctance.value,
      capacitiveDiractance: capacitiveDiractance.value
    })
  } catch (error) {
    console.error('Failed to update parameters:', error)
  }
}

const getScoreMotionColor = () => {
  if (fluorescentScoreMotion.value < 30) return 'from-blue-500 to-cyan-500'
  if (fluorescentScoreMotion.value < 70) return 'from-green-500 to-emerald-500'
  return 'from-orange-500 to-red-500'
}

const getScoreMotionLabel = () => {
  if (fluorescentScoreMotion.value < 30) return 'Minimal'
  if (fluorescentScoreMotion.value < 50) return 'Moderate'
  if (fluorescentScoreMotion.value < 70) return 'Elevated'
  if (fluorescentScoreMotion.value < 90) return 'High'
  return 'Maximum'
}

const fetchStatus = async () => {
  try {
    const response = await axios.get(`${API_BASE}/api/cardinal-grammeter/status`)
    stats.value = response.data
    isActive.value = response.data.active
  } catch (error) {
    // Silently fail - service might not be available
  }
}

const startStatusPolling = () => {
  if (statusInterval) return
  statusInterval = window.setInterval(fetchStatus, 2000)
}

const stopStatusPolling = () => {
  if (statusInterval) {
    clearInterval(statusInterval)
    statusInterval = null
  }
}

onMounted(() => {
  fetchStatus()
})

onUnmounted(() => {
  stopStatusPolling()
})
</script>

<template>
  <div class="bg-surface border border-primary/30 rounded-lg p-6 shadow-lg">
    <div class="flex items-center justify-between mb-4">
      <h3 class="text-lg font-bold text-primary">Cardinal Grammeter Control</h3>
      <div class="flex items-center gap-2">
        <div 
          class="w-3 h-3 rounded-full transition-colors"
          :class="isActive ? 'bg-green-500 animate-pulse' : 'bg-gray-500'"
        ></div>
        <span class="text-sm text-text-secondary">
          {{ isActive ? 'Active' : 'Inactive' }}
        </span>
      </div>
    </div>

    <!-- Control Buttons -->
    <div class="flex gap-3 mb-6">
      <button
        v-if="!isActive"
        @click="startTraffic"
        :disabled="isLoading"
        class="flex-1 flex items-center justify-center gap-2 bg-green-600 hover:bg-green-700 disabled:bg-gray-600 text-white font-semibold py-3 px-4 rounded-lg transition-all"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        Start Traffic Generation
      </button>
      <button
        v-else
        @click="stopTraffic"
        :disabled="isLoading"
        class="flex-1 flex items-center justify-center gap-2 bg-red-600 hover:bg-red-700 disabled:bg-gray-600 text-white font-semibold py-3 px-4 rounded-lg transition-all"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
        </svg>
        Stop Traffic Generation
      </button>
    </div>

    <!-- Pattern Selector -->
    <div class="mb-4">
      <label class="block text-sm font-medium text-text-secondary mb-2">
        Traffic Pattern
      </label>
      <div class="grid grid-cols-4 gap-2">
        <button
          v-for="p in ['steady', 'burst', 'ramp', 'sine']"
          :key="p"
          @click="pattern = p as any"
          :class="pattern === p 
            ? 'bg-primary text-background' 
            : 'bg-surface-hover text-text-secondary hover:bg-surface-active'"
          class="py-2 px-3 rounded text-sm font-medium transition-colors capitalize"
        >
          {{ p }}
        </button>
      </div>
    </div>

    <!-- Fluorescent Score Motion (TPS Throttle) -->
    <div class="mb-4 p-4 rounded-lg bg-gradient-to-r" :class="getScoreMotionColor()">
      <div class="flex justify-between items-center mb-3">
        <div>
          <label class="text-sm font-bold text-white drop-shadow">
            ⚡ Fluorescent Score Motion
          </label>
          <div class="text-xs text-white/90 mt-0.5">TPS Throttle Control</div>
        </div>
        <div class="text-right">
          <div class="text-2xl font-bold text-white drop-shadow">{{ fluorescentScoreMotion }}</div>
          <div class="text-xs text-white/90">{{ getScoreMotionLabel() }}</div>
        </div>
      </div>
      <input
        v-model.number="fluorescentScoreMotion"
        @change="updateParameters"
        type="range"
        min="0"
        max="100"
        class="w-full h-3 bg-white/30 rounded-lg appearance-none cursor-pointer slider-white"
      />
      <div class="flex justify-between text-xs text-white/90 mt-2">
        <span>0.5 TPS</span>
        <span class="font-semibold text-base">{{ estimatedTPS }} TPS</span>
        <span>1000 TPS</span>
      </div>
      <div v-if="fluorescentScoreMotion > 80" class="text-xs text-white/90 mt-2 text-center font-semibold">
        ⚠️ EXTREME LOAD - Will drive aggressive scaling
      </div>
    </div>

    <!-- Magneto Reluctance (Latency Injection) -->
    <div class="mb-4">
      <div class="flex justify-between items-center mb-2">
        <label class="text-sm font-medium text-text-secondary">
          🧲 Magneto Reluctance (Friction Induction)
        </label>
        <div class="text-right">
          <span class="text-sm font-mono text-primary">{{ magnetoReluctance }}</span>
          <span class="text-xs text-text-muted ml-2">~{{ estimatedLatency }}ms</span>
        </div>
      </div>
      <input
        v-model.number="magnetoReluctance"
        @change="updateParameters"
        type="range"
        min="0"
        max="100"
        class="w-full h-2 bg-surface-hover rounded-lg appearance-none cursor-pointer accent-orange-500"
      />
      <div class="flex justify-between text-xs text-text-muted mt-1">
        <span>No Latency</span>
        <span>High Latency + Jitter</span>
      </div>
    </div>

    <!-- Capacitive Diractance Slider -->
    <div class="mb-6">
      <div class="flex justify-between items-center mb-2">
        <label class="text-sm font-medium text-text-secondary">
          Capacitive Diractance (Complexity)
        </label>
        <span class="text-sm font-mono text-primary">{{ capacitiveDiractance }}</span>
      </div>
      <input
        v-model.number="capacitiveDiractance"
        @change="updateParameters"
        type="range"
        min="0"
        max="100"
        class="w-full h-2 bg-surface-hover rounded-lg appearance-none cursor-pointer accent-primary"
      />
      <div class="flex justify-between text-xs text-text-muted mt-1">
        <span>Simple</span>
        <span>Complex</span>
      </div>
    </div>

    <!-- Stats Display -->
    <div v-if="isActive" class="grid grid-cols-3 gap-4 pt-4 border-t border-text-muted/20">
      <div class="text-center">
        <div class="text-2xl font-bold text-primary">{{ stats.requestsSent }}</div>
        <div class="text-xs text-text-muted">Requests Sent</div>
      </div>
      <div class="text-center">
        <div class="text-2xl font-bold text-green-500">{{ stats.successRate.toFixed(1) }}%</div>
        <div class="text-xs text-text-muted">Success Rate</div>
      </div>
      <div class="text-center">
        <div class="text-2xl font-bold text-cyan-400">{{ stats.avgLatency.toFixed(1) }}ms</div>
        <div class="text-xs text-text-muted">Avg Latency</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.slider-white::-webkit-slider-thumb {
  appearance: none;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: white;
  cursor: pointer;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
}

.slider-white::-moz-range-thumb {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: white;
  cursor: pointer;
  border: none;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.3);
}
</style>
