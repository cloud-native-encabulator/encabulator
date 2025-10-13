<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

interface ActivityLog {
  id: string
  username: string
  action: string
  resource: string | null
  timestamp: string
  ip_address: string | null
  success: boolean
}

const authStore = useAuthStore()
const logs = ref<ActivityLog[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filterAction = ref('')
const filterSuccess = ref<'all' | 'success' | 'failed'>('all')

const API_BASE = 'http://unilateral-phase-detractor.encabulator-support.svc.cluster.local:8080'

const filteredLogs = computed(() => {
  return logs.value.filter(log => {
    // Search filter
    if (searchQuery.value) {
      const query = searchQuery.value.toLowerCase()
      const matchesSearch = 
        log.username.toLowerCase().includes(query) ||
        log.action.toLowerCase().includes(query) ||
        (log.resource && log.resource.toLowerCase().includes(query))
      if (!matchesSearch) return false
    }
    
    // Action filter
    if (filterAction.value && log.action !== filterAction.value) {
      return false
    }
    
    // Success filter
    if (filterSuccess.value === 'success' && !log.success) return false
    if (filterSuccess.value === 'failed' && log.success) return false
    
    return true
  })
})

const uniqueActions = computed(() => {
  const actions = new Set(logs.value.map(log => log.action))
  return Array.from(actions).sort()
})

async function fetchAllLogs() {
  loading.value = true
  try {
    // Fetch activity for all users and combine
    const response = await fetch(`${API_BASE}/api/v1/users`, {
      headers: { 'Authorization': `Bearer ${authStore.token}` }
    })
    if (!response.ok) throw new Error('Failed to fetch users')
    const users = await response.json()
    
    const allLogs: ActivityLog[] = []
    for (const user of users) {
      const activityResponse = await fetch(
        `${API_BASE}/api/v1/users/${user.username}/activity?limit=100`,
        { headers: { 'Authorization': `Bearer ${authStore.token}` } }
      )
      if (activityResponse.ok) {
        const userLogs = await activityResponse.json()
        allLogs.push(...userLogs)
      }
    }
    
    // Sort by timestamp descending
    logs.value = allLogs.sort((a, b) => 
      new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime()
    )
  } catch (error) {
    console.error('Failed to fetch logs:', error)
  } finally {
    loading.value = false
  }
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString()
}

function getActionBadgeClass(action: string): string {
  if (action.includes('delete')) return 'bg-red-500/20 text-red-400'
  if (action.includes('create')) return 'bg-green-500/20 text-green-400'
  if (action.includes('update')) return 'bg-blue-500/20 text-blue-400'
  if (action.includes('login')) return 'bg-purple-500/20 text-purple-400'
  return 'bg-gray-500/20 text-gray-400'
}

function exportLogs() {
  const csv = [
    ['Timestamp', 'Username', 'Action', 'Resource', 'IP Address', 'Success'].join(','),
    ...filteredLogs.value.map(log => [
      log.timestamp,
      log.username,
      log.action,
      log.resource || '',
      log.ip_address || '',
      log.success ? 'Yes' : 'No'
    ].join(','))
  ].join('\n')
  
  const blob = new Blob([csv], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `audit-log-${new Date().toISOString()}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

onMounted(() => {
  fetchAllLogs()
})
</script>

<template>
  <div class="p-8">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold">Audit Log</h1>
      <div class="flex gap-2">
        <button @click="fetchAllLogs" class="btn btn-secondary" :disabled="loading">
          Refresh
        </button>
        <button @click="exportLogs" class="btn btn-primary" :disabled="filteredLogs.length === 0">
          Export CSV
        </button>
      </div>
    </div>

    <!-- Filters -->
    <div class="card mb-6">
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label class="block text-sm font-medium mb-2">Search</label>
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Search username, action, or resource..."
            class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg"
          />
        </div>
        
        <div>
          <label class="block text-sm font-medium mb-2">Action</label>
          <select
            v-model="filterAction"
            class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg"
          >
            <option value="">All Actions</option>
            <option v-for="action in uniqueActions" :key="action" :value="action">
              {{ action }}
            </option>
          </select>
        </div>
        
        <div>
          <label class="block text-sm font-medium mb-2">Status</label>
          <select
            v-model="filterSuccess"
            class="w-full px-3 py-2 bg-background border border-text-muted/20 rounded-lg"
          >
            <option value="all">All</option>
            <option value="success">Success Only</option>
            <option value="failed">Failed Only</option>
          </select>
        </div>
      </div>
    </div>

    <!-- Logs Table -->
    <div class="card">
      <div v-if="loading" class="text-center py-8 text-text-muted">
        Loading audit logs...
      </div>
      <div v-else-if="filteredLogs.length === 0" class="text-center py-8 text-text-muted">
        No audit logs found
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full">
          <thead class="border-b border-text-muted/20">
            <tr class="text-left">
              <th class="pb-3 font-semibold">Timestamp</th>
              <th class="pb-3 font-semibold">User</th>
              <th class="pb-3 font-semibold">Action</th>
              <th class="pb-3 font-semibold">Resource</th>
              <th class="pb-3 font-semibold">IP Address</th>
              <th class="pb-3 font-semibold">Status</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="log in filteredLogs"
              :key="log.id"
              class="border-b border-text-muted/10 hover:bg-background/50"
            >
              <td class="py-3 text-sm">{{ formatDate(log.timestamp) }}</td>
              <td class="py-3 font-medium">{{ log.username }}</td>
              <td class="py-3">
                <span :class="['text-xs px-2 py-1 rounded', getActionBadgeClass(log.action)]">
                  {{ log.action }}
                </span>
              </td>
              <td class="py-3 text-sm text-text-secondary">{{ log.resource || '-' }}</td>
              <td class="py-3 text-sm text-text-secondary">{{ log.ip_address || '-' }}</td>
              <td class="py-3">
                <span :class="[
                  'inline-flex items-center gap-1 text-xs px-2 py-1 rounded',
                  log.success ? 'bg-success/20 text-success' : 'bg-danger/20 text-danger'
                ]">
                  <span :class="['w-1.5 h-1.5 rounded-full', log.success ? 'bg-success' : 'bg-danger']"></span>
                  {{ log.success ? 'Success' : 'Failed' }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      
      <div class="mt-4 text-sm text-text-muted text-center">
        Showing {{ filteredLogs.length }} of {{ logs.length }} total logs
      </div>
    </div>
  </div>
</template>
