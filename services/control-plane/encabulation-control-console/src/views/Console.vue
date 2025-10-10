<script setup lang="ts">
import { computed } from 'vue'
import { useSystemStore } from '@/stores/system'
import { useAuthStore } from '@/stores/auth'
import { useWebSocket } from '@/composables/useWebSocket'
import SystemTopology from '@/components/SystemTopology.vue'
import ControlPanel from '@/components/ControlPanel.vue'
import MetricsPanel from '@/components/MetricsPanel.vue'

const systemStore = useSystemStore()
const authStore = useAuthStore()
useWebSocket()

const connectionStatus = computed(() => 
  systemStore.connected ? 'Connected' : 'Disconnected'
)

const connectionClass = computed(() => 
  systemStore.connected ? 'bg-success' : 'bg-danger'
)

const guestTimeRemaining = computed(() => {
  if (!authStore.isGuest) return ''
  const ms = authStore.getGuestSessionTimeRemaining()
  const hours = Math.floor(ms / (1000 * 60 * 60))
  return `${hours}h remaining`
})
</script>

<template>
  <div class="min-h-screen flex flex-col">
    <!-- Guest Banner -->
    <div v-if="authStore.isGuest" class="bg-warning/20 border-b border-warning px-6 py-2">
      <div class="max-w-7xl mx-auto flex items-center justify-between text-sm">
        <div class="flex items-center gap-2">
          <span class="text-warning">👤</span>
          <span>Viewing as Guest • {{ guestTimeRemaining }}</span>
        </div>
        <router-link to="/login" class="text-primary hover:underline font-medium">
          Sign in for full access
        </router-link>
      </div>
    </div>
    
    <!-- Header -->
    <header class="bg-surface border-b border-text-muted/20 px-6 py-4">
      <div class="flex items-center justify-between">
        <div class="flex items-center gap-4">
          <h1 class="text-2xl font-bold text-primary">
            🔧 Encabulation Control Console
          </h1>
        </div>
        
        <div class="flex items-center gap-4">
          <div class="flex items-center gap-2">
            <div 
              :class="['w-2 h-2 rounded-full', connectionClass]"
              class="animate-pulse"
            />
            <span class="text-sm text-text-secondary">
              {{ connectionStatus }}
            </span>
          </div>
          
          <router-link 
            v-if="authStore.isAdmin"
            to="/admin" 
            class="btn btn-secondary text-sm"
          >
            Admin Portal
          </router-link>
          
          <router-link 
            v-else-if="authStore.isGuest"
            to="/login" 
            class="btn btn-primary text-sm"
          >
            Sign In
          </router-link>
        </div>
      </div>
    </header>

    <!-- Main Content -->
    <main class="flex-1 p-6">
      <div class="max-w-7xl mx-auto">
        <!-- System Health Overview -->
        <div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
          <div class="card">
            <div class="text-text-secondary text-sm mb-1">System Health</div>
            <div class="text-2xl font-bold capitalize" :class="{
              'text-success': systemStore.systemHealth === 'healthy',
              'text-warning': systemStore.systemHealth === 'degraded',
              'text-danger': systemStore.systemHealth === 'unhealthy',
              'text-text-muted': systemStore.systemHealth === 'unknown'
            }">
              {{ systemStore.systemHealth }}
            </div>
          </div>
          
          <div class="card">
            <div class="text-text-secondary text-sm mb-1">Services</div>
            <div class="text-2xl font-bold">
              {{ systemStore.healthyServices }} / {{ systemStore.totalServices }}
            </div>
          </div>
          
          <div class="card">
            <div class="text-text-secondary text-sm mb-1">Avg Latency</div>
            <div class="text-2xl font-bold">
              {{ systemStore.metrics.avgLatency }}ms
            </div>
          </div>
          
          <div class="card">
            <div class="text-text-secondary text-sm mb-1">Error Rate</div>
            <div class="text-2xl font-bold">
              {{ (systemStore.metrics.errorRate * 100).toFixed(2) }}%
            </div>
          </div>
        </div>

        <!-- System Topology and Controls -->
        <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <div class="lg:col-span-2 card">
            <SystemTopology />
          </div>
          
          <div class="space-y-6">
            <div class="card">
              <ControlPanel />
            </div>
            
            <div class="card">
              <MetricsPanel />
            </div>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>
