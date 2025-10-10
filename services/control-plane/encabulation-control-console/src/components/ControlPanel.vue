<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'

const authStore = useAuthStore()
const selectedService = ref('')

const services = [
  { value: 'panametric-fam-processor', label: 'Panametric Fam Processor' },
  { value: 'dingle-arm-api-gateway', label: 'Dingle Arm Gateway' },
  { value: 'logarithmic-casing-service', label: 'Logarithmic Casing' },
  { value: 'hydrocoptic-datastore', label: 'Hydrocoptic Datastore' },
  { value: 'prefabulated-cache', label: 'Prefabulated Cache' }
]
</script>

<template>
  <div class="control-panel">
    <h3 class="text-lg font-bold mb-4">Quick Actions</h3>
    
    <div v-if="authStore.isGuest" class="p-3 bg-warning/20 border border-warning rounded-lg text-sm mb-4">
      <div class="flex items-center gap-2">
        <span>🔒</span>
        <span>Control actions require authentication</span>
      </div>
    </div>
    
    <div class="space-y-3">
      <div>
        <label class="block text-sm text-text-secondary mb-2">Select Service</label>
        <select 
          v-model="selectedService"
          :disabled="authStore.isGuest"
          class="input w-full"
        >
          <option value="">Choose a service...</option>
          <option 
            v-for="service in services" 
            :key="service.value"
            :value="service.value"
          >
            {{ service.label }}
          </option>
        </select>
      </div>
      
      <div class="grid grid-cols-2 gap-2">
        <button 
          :disabled="!selectedService || authStore.isGuest"
          class="btn btn-secondary text-sm"
        >
          Scale Up
        </button>
        <button 
          :disabled="!selectedService || authStore.isGuest"
          class="btn btn-secondary text-sm"
        >
          Scale Down
        </button>
      </div>
      
      <button 
        :disabled="!selectedService || authStore.isGuest"
        class="btn btn-danger w-full text-sm"
      >
        Restart Service
      </button>
    </div>
    
    <div class="mt-4 text-xs text-text-muted">
      Enhanced control actions coming in Task 6.3
    </div>
  </div>
</template>

<style scoped>
.control-panel {
  width: 100%;
}
</style>
