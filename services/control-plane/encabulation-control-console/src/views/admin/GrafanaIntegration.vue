<script setup lang="ts">
import { ref } from 'vue'

const selectedDashboard = ref<string | null>(null)

const dashboards = [
  {
    id: 'system-overview',
    name: 'System Overview',
    description: 'Overall system health and metrics',
    url: 'http://grafana.encabulator-monitoring.svc.cluster.local:3000/d/system-overview'
  },
  {
    id: 'service-metrics',
    name: 'Service Metrics',
    description: 'Individual service performance',
    url: 'http://grafana.encabulator-monitoring.svc.cluster.local:3000/d/service-metrics'
  },
  {
    id: 'kubernetes-cluster',
    name: 'Kubernetes Cluster',
    description: 'Cluster resource utilization',
    url: 'http://grafana.encabulator-monitoring.svc.cluster.local:3000/d/kubernetes-cluster'
  }
]

function selectDashboard(dashboardId: string) {
  selectedDashboard.value = dashboardId
}

function openInNewTab(url: string) {
  window.open(url, '_blank')
}
</script>

<template>
  <div class="p-8">
    <h1 class="text-2xl font-bold mb-6">Grafana Integration</h1>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
      <div
        v-for="dashboard in dashboards"
        :key="dashboard.id"
        @click="selectDashboard(dashboard.id)"
        :class="[
          'card cursor-pointer transition-all',
          selectedDashboard === dashboard.id
            ? 'border-primary ring-2 ring-primary/50'
            : 'hover:border-primary/50'
        ]"
      >
        <h3 class="font-semibold mb-2">{{ dashboard.name }}</h3>
        <p class="text-sm text-text-secondary mb-4">{{ dashboard.description }}</p>
        <button
          @click.stop="openInNewTab(dashboard.url)"
          class="btn btn-secondary btn-sm w-full"
        >
          Open in New Tab
        </button>
      </div>
    </div>

    <!-- Embedded Dashboard -->
    <div v-if="selectedDashboard" class="card">
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-lg font-semibold">
          {{ dashboards.find(d => d.id === selectedDashboard)?.name }}
        </h2>
        <button
          @click="openInNewTab(dashboards.find(d => d.id === selectedDashboard)!.url)"
          class="btn btn-secondary btn-sm"
        >
          Open in Full Screen
        </button>
      </div>
      
      <div class="bg-background rounded-lg overflow-hidden" style="height: 600px;">
        <iframe
          :src="dashboards.find(d => d.id === selectedDashboard)?.url"
          class="w-full h-full border-0"
          title="Grafana Dashboard"
        ></iframe>
      </div>
      
      <div class="mt-4 p-4 bg-warning/10 border border-warning/30 rounded-lg">
        <p class="text-sm text-warning">
          <strong>Note:</strong> Dashboard embedding requires Grafana proxy configuration (Task 6.13).
          For now, use "Open in New Tab" to view dashboards directly.
        </p>
      </div>
    </div>

    <div v-else class="card text-center py-12">
      <svg class="w-16 h-16 mx-auto mb-4 text-text-muted" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
      </svg>
      <p class="text-text-muted">Select a dashboard to view</p>
    </div>
  </div>
</template>
