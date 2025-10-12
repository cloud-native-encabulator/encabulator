<template>
  <div class="bg-gray-800 rounded-lg p-6 shadow-lg">
    <h2 class="text-xl font-bold text-cyan-400 mb-4">Encabulator Control Actions</h2>
    
    <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
      <!-- Synchronize Cardinal Grammeters -->
      <button
        @click="synchronizeGrammeters"
        :disabled="isLoading"
        class="flex items-center justify-center gap-2 bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 disabled:from-gray-600 disabled:to-gray-700 text-white font-semibold py-3 px-4 rounded-lg transition-all duration-200 transform hover:scale-105 disabled:scale-100 disabled:cursor-not-allowed"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
        Synchronize Cardinal Grammeters
      </button>

      <!-- Induce Side Fumbling -->
      <button
        @click="induceFumbling"
        :disabled="isLoading"
        class="flex items-center justify-center gap-2 bg-gradient-to-r from-orange-600 to-red-600 hover:from-orange-500 hover:to-red-500 disabled:from-gray-600 disabled:to-gray-700 text-white font-semibold py-3 px-4 rounded-lg transition-all duration-200 transform hover:scale-105 disabled:scale-100 disabled:cursor-not-allowed"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
        </svg>
        Induce Side Fumbling
      </button>

      <!-- Flex Dingle Arm Up -->
      <button
        @click="flexDingleArm('up')"
        :disabled="isLoading"
        class="flex items-center justify-center gap-2 bg-gradient-to-r from-green-600 to-emerald-600 hover:from-green-500 hover:to-emerald-500 disabled:from-gray-600 disabled:to-gray-700 text-white font-semibold py-3 px-4 rounded-lg transition-all duration-200 transform hover:scale-105 disabled:scale-100 disabled:cursor-not-allowed"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 15l7-7 7 7" />
        </svg>
        Flex Dingle Arm (Scale Up)
      </button>

      <!-- Flex Dingle Arm Down -->
      <button
        @click="flexDingleArm('down')"
        :disabled="isLoading"
        class="flex items-center justify-center gap-2 bg-gradient-to-r from-yellow-600 to-amber-600 hover:from-yellow-500 hover:to-amber-500 disabled:from-gray-600 disabled:to-gray-700 text-white font-semibold py-3 px-4 rounded-lg transition-all duration-200 transform hover:scale-105 disabled:scale-100 disabled:cursor-not-allowed"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
        Flex Dingle Arm (Scale Down)
      </button>

      <!-- Recalibrate Panametric Fam -->
      <button
        @click="recalibrateFam"
        :disabled="isLoading"
        class="flex items-center justify-center gap-2 bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-500 hover:to-indigo-500 disabled:from-gray-600 disabled:to-gray-700 text-white font-semibold py-3 px-4 rounded-lg transition-all duration-200 transform hover:scale-105 disabled:scale-100 disabled:cursor-not-allowed"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
        Recalibrate Panametric Fam
      </button>

      <!-- Engage Malaise Dampeners -->
      <button
        @click="engageDampeners"
        :disabled="isLoading"
        class="flex items-center justify-center gap-2 bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 disabled:from-gray-600 disabled:to-gray-700 text-white font-semibold py-3 px-4 rounded-lg transition-all duration-200 transform hover:scale-105 disabled:scale-100 disabled:cursor-not-allowed"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        Engage Malaise Dampeners
      </button>

      <!-- Purge Hydrocoptic Marzelvanes -->
      <button
        @click="purgeMarzelvanes"
        :disabled="isLoading"
        class="flex items-center justify-center gap-2 bg-gradient-to-r from-pink-600 to-rose-600 hover:from-pink-500 hover:to-rose-500 disabled:from-gray-600 disabled:to-gray-700 text-white font-semibold py-3 px-4 rounded-lg transition-all duration-200 transform hover:scale-105 disabled:scale-100 disabled:cursor-not-allowed"
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
        </svg>
        Purge Hydrocoptic Marzelvanes
      </button>
    </div>

    <!-- Action History -->
    <div class="mt-6">
      <h3 class="text-lg font-semibold text-gray-300 mb-3">Recent Actions</h3>
      <div class="bg-gray-900 rounded-lg p-4 max-h-48 overflow-y-auto">
        <div v-if="actionHistory.length === 0" class="text-gray-500 text-sm">
          No actions performed yet
        </div>
        <div
          v-for="(action, index) in actionHistory"
          :key="index"
          class="flex items-start gap-3 mb-2 pb-2 border-b border-gray-800 last:border-0"
        >
          <div
            :class="[
              'w-2 h-2 rounded-full mt-1.5 flex-shrink-0',
              action.success ? 'bg-green-500' : 'bg-red-500'
            ]"
          ></div>
          <div class="flex-1 min-w-0">
            <p class="text-sm text-gray-300">{{ action.message }}</p>
            <p class="text-xs text-gray-500 mt-0.5">{{ formatTimestamp(action.timestamp) }}</p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import axios from 'axios'

interface ActionHistoryItem {
  message: string
  success: boolean
  timestamp: string
}

const isLoading = ref(false)
const actionHistory = ref<ActionHistoryItem[]>([])

const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:3000'

const addToHistory = (message: string, success: boolean) => {
  actionHistory.value.unshift({
    message,
    success,
    timestamp: new Date().toISOString()
  })
  // Keep only last 10 actions
  if (actionHistory.value.length > 10) {
    actionHistory.value = actionHistory.value.slice(0, 10)
  }
}

const synchronizeGrammeters = async () => {
  isLoading.value = true
  try {
    const response = await axios.post(`${API_BASE}/api/actions/synchronize-grammeters`)
    addToHistory(response.data.message || 'Cardinal Grammeters synchronized', true)
  } catch (error: any) {
    addToHistory(error.response?.data?.error || 'Failed to synchronize grammeters', false)
  } finally {
    isLoading.value = false
  }
}

const induceFumbling = async () => {
  isLoading.value = true
  try {
    const response = await axios.post(`${API_BASE}/api/actions/induce-fumbling`)
    addToHistory(response.data.message || 'Side fumbling induced', true)
  } catch (error: any) {
    addToHistory(error.response?.data?.error || 'Failed to induce fumbling', false)
  } finally {
    isLoading.value = false
  }
}

const flexDingleArm = async (direction: 'up' | 'down') => {
  isLoading.value = true
  try {
    const response = await axios.post(`${API_BASE}/api/actions/flex-dingle-arm`, { direction })
    addToHistory(response.data.message || `Dingle Arm flexed ${direction}`, true)
  } catch (error: any) {
    addToHistory(error.response?.data?.error || 'Failed to flex dingle arm', false)
  } finally {
    isLoading.value = false
  }
}

const recalibrateFam = async () => {
  isLoading.value = true
  try {
    const response = await axios.post(`${API_BASE}/api/actions/recalibrate-fam`)
    addToHistory(response.data.message || 'Panametric Fam recalibrated', true)
  } catch (error: any) {
    addToHistory(error.response?.data?.error || 'Failed to recalibrate fam', false)
  } finally {
    isLoading.value = false
  }
}

const engageDampeners = async () => {
  isLoading.value = true
  try {
    const response = await axios.post(`${API_BASE}/api/actions/engage-dampeners`)
    addToHistory(response.data.message || 'Malaise dampeners engaged', true)
  } catch (error: any) {
    addToHistory(error.response?.data?.error || 'Failed to engage dampeners', false)
  } finally {
    isLoading.value = false
  }
}

const purgeMarzelvanes = async () => {
  isLoading.value = true
  try {
    const response = await axios.post(`${API_BASE}/api/actions/purge-marzelvanes`)
    addToHistory(response.data.message || 'Hydrocoptic marzelvanes purged', true)
  } catch (error: any) {
    addToHistory(error.response?.data?.error || 'Failed to purge marzelvanes', false)
  } finally {
    isLoading.value = false
  }
}

const formatTimestamp = (timestamp: string): string => {
  const date = new Date(timestamp)
  return date.toLocaleTimeString()
}
</script>
