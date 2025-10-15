<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useSystemStore } from '@/stores/system'

const systemStore = useSystemStore()
const currentTime = ref(Date.now())

// Update current time every second for reactive "ago" text
let timeInterval: number | null = null
onMounted(() => {
  timeInterval = window.setInterval(() => {
    currentTime.value = Date.now()
  }, 1000)
})

onUnmounted(() => {
  if (timeInterval !== null) {
    clearInterval(timeInterval)
  }
})

const metrics = computed(() => [
  {
    label: 'Throughput',
    value: (systemStore.metrics?.totalRequests || 0).toLocaleString(),
    unit: ' req/s',
    color: 'text-primary'
  },
  {
    label: 'Avg Latency',
    value: systemStore.metrics?.avgLatency || 0,
    unit: 'ms',
    color: (systemStore.metrics?.avgLatency || 0) > 100 ? 'text-warning' : 'text-success'
  },
  {
    label: 'Error Rate',
    value: ((systemStore.metrics?.errorRate || 0) * 100).toFixed(2),
    unit: '%',
    color: (systemStore.metrics?.errorRate || 0) > 0.05 ? 'text-danger' : 'text-success'
  },
  {
    label: 'CPU Usage',
    value: (systemStore.metrics?.cpuUsage || 0).toFixed(1),
    unit: '%',
    color: (systemStore.metrics?.cpuUsage || 0) > 80 ? 'text-danger' : 'text-primary'
  },
  {
    label: 'Memory Usage',
    value: (systemStore.metrics?.memoryUsage || 0).toFixed(1),
    unit: '%',
    color: (systemStore.metrics?.memoryUsage || 0) > 80 ? 'text-danger' : 'text-primary'
  }
])

const lastUpdateText = computed(() => {
  if (!systemStore.lastUpdate) return 'Never'
  // Use currentTime.value to make this reactive
  const seconds = Math.floor((currentTime.value - systemStore.lastUpdate.getTime()) / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ago`
})
</script>

<template>
  <div class="metrics-panel">
    <div class="flex items-center justify-between mb-4">
      <h3 class="text-lg font-bold">System Metrics</h3>
      <span class="text-xs text-text-muted">Updated {{ lastUpdateText }}</span>
    </div>
    
    <div class="space-y-3">
      <div 
        v-for="metric in metrics" 
        :key="metric.label"
        class="metric-item"
      >
        <div class="flex justify-between items-baseline mb-1">
          <span class="text-sm text-text-secondary">{{ metric.label }}</span>
          <span :class="['text-lg font-bold', metric.color]">
            {{ metric.value }}<span class="text-sm">{{ metric.unit }}</span>
          </span>
        </div>
        <div class="h-1 bg-background rounded-full overflow-hidden">
          <div 
            class="h-full transition-all duration-300"
            :class="metric.color.replace('text-', 'bg-')"
            :style="{ 
              width: metric.label.includes('Usage') 
                ? `${metric.value}%` 
                : metric.label === 'Error Rate'
                ? `${Math.min(parseFloat(String(metric.value)) * 10, 100)}%`
                : '100%'
            }"
          ></div>
        </div>
      </div>
    </div>
    
    <div v-if="!systemStore.connected" class="mt-4 p-3 bg-danger/20 border border-danger rounded-lg text-sm">
      <div class="flex items-center gap-2">
        <span class="text-danger">⚠️</span>
        <span>Disconnected from server</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.metrics-panel {
  width: 100%;
}

.metric-item {
  padding: 0.75rem;
  background: var(--color-background);
  border-radius: 0.5rem;
  border: 1px solid rgba(102, 102, 102, 0.2);
}
</style>
