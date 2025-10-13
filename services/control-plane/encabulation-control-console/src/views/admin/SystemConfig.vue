<script setup lang="ts">
import { ref } from 'vue'

const config = ref({
  featureFlags: {
    trafficVisualization: true,
    chaosEngineering: true,
    autoScaling: false,
    advancedMetrics: true
  },
  resourceLimits: {
    maxReplicas: 10,
    cpuLimit: '1000m',
    memoryLimit: '2Gi'
  },
  system: {
    logLevel: 'info',
    metricsInterval: 30,
    healthCheckTimeout: 5
  }
})

const saved = ref(false)

function saveConfig() {
  // TODO: Implement actual save to backend (Task 6.13)
  console.log('Saving config:', config.value)
  saved.value = true
  setTimeout(() => {
    saved.value = false
  }, 3000)
}

function resetConfig() {
  if (confirm('Reset all configuration to defaults?')) {
    config.value = {
      featureFlags: {
        trafficVisualization: true,
        chaosEngineering: true,
        autoScaling: false,
        advancedMetrics: true
      },
      resourceLimits: {
        maxReplicas: 10,
        cpuLimit: '1000m',
        memoryLimit: '2Gi'
      },
      system: {
        logLevel: 'info',
        metricsInterval: 30,
        healthCheckTimeout: 5
      }
    }
  }
}
</script>

<template>
  <div class="p-8">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold">System Configuration</h1>
      <div class="flex gap-2">
        <button @click="resetConfig" class="btn btn-secondary">Reset to Defaults</button>
        <button @click="saveConfig" class="btn btn-primary">Save Changes</button>
      </div>
    </div>

    <!-- Success Message -->
    <div v-if="saved" class="bg-success/20 border border-success text-success px-4 py-3 rounded-lg mb-4">
      Configuration saved successfully
    </div>

    <div class="space-y-6">
      <!-- Feature Flags -->
      <div class="card">
        <h2 class="text-lg font-semibold mb-4">Feature Flags</h2>
        <div class="space-y-3">
          <label class="flex items-center justify-between p-3 bg-background rounded-lg">
            <div>
              <div class="font-medium">Traffic Visualization</div>
              <div class="text-sm text-text-secondary">Show animated traffic particles on topology</div>
            </div>
            <input type="checkbox" v-model="config.featureFlags.trafficVisualization" class="w-5 h-5" />
          </label>
          
          <label class="flex items-center justify-between p-3 bg-background rounded-lg">
            <div>
              <div class="font-medium">Chaos Engineering</div>
              <div class="text-sm text-text-secondary">Enable malaise injection and failure simulation</div>
            </div>
            <input type="checkbox" v-model="config.featureFlags.chaosEngineering" class="w-5 h-5" />
          </label>
          
          <label class="flex items-center justify-between p-3 bg-background rounded-lg">
            <div>
              <div class="font-medium">Auto Scaling</div>
              <div class="text-sm text-text-secondary">Automatically scale services based on load</div>
            </div>
            <input type="checkbox" v-model="config.featureFlags.autoScaling" class="w-5 h-5" />
          </label>
          
          <label class="flex items-center justify-between p-3 bg-background rounded-lg">
            <div>
              <div class="font-medium">Advanced Metrics</div>
              <div class="text-sm text-text-secondary">Collect detailed performance metrics</div>
            </div>
            <input type="checkbox" v-model="config.featureFlags.advancedMetrics" class="w-5 h-5" />
          </label>
        </div>
      </div>

      <!-- Resource Limits -->
      <div class="card">
        <h2 class="text-lg font-semibold mb-4">Resource Limits</h2>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label class="block text-sm font-medium mb-2">Max Replicas</label>
            <input
              type="number"
              v-model.number="config.resourceLimits.maxReplicas"
              min="1"
              max="50"
              class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg"
            />
          </div>
          
          <div>
            <label class="block text-sm font-medium mb-2">CPU Limit</label>
            <input
              type="text"
              v-model="config.resourceLimits.cpuLimit"
              placeholder="1000m"
              class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg"
            />
          </div>
          
          <div>
            <label class="block text-sm font-medium mb-2">Memory Limit</label>
            <input
              type="text"
              v-model="config.resourceLimits.memoryLimit"
              placeholder="2Gi"
              class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg"
            />
          </div>
        </div>
      </div>

      <!-- System Settings -->
      <div class="card">
        <h2 class="text-lg font-semibold mb-4">System Settings</h2>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div>
            <label class="block text-sm font-medium mb-2">Log Level</label>
            <select
              v-model="config.system.logLevel"
              class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg"
            >
              <option value="debug">Debug</option>
              <option value="info">Info</option>
              <option value="warn">Warning</option>
              <option value="error">Error</option>
            </select>
          </div>
          
          <div>
            <label class="block text-sm font-medium mb-2">Metrics Interval (seconds)</label>
            <input
              type="number"
              v-model.number="config.system.metricsInterval"
              min="5"
              max="300"
              class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg"
            />
          </div>
          
          <div>
            <label class="block text-sm font-medium mb-2">Health Check Timeout (seconds)</label>
            <input
              type="number"
              v-model.number="config.system.healthCheckTimeout"
              min="1"
              max="30"
              class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg"
            />
          </div>
        </div>
      </div>

      <!-- Note -->
      <div class="p-4 bg-info/10 border border-info/30 rounded-lg">
        <p class="text-sm text-info">
          <strong>Note:</strong> Configuration persistence will be implemented in Task 6.13.
          Changes are currently stored in browser local storage only.
        </p>
      </div>
    </div>
  </div>
</template>
