import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ActionHistoryItem } from '@/types'

export const useActionsStore = defineStore('actions', () => {
  const history = ref<ActionHistoryItem[]>([])
  const pendingActions = ref<Set<string>>(new Set())

  function addAction(action: Omit<ActionHistoryItem, 'id' | 'timestamp'>) {
    const newAction: ActionHistoryItem = {
      ...action,
      id: crypto.randomUUID(),
      timestamp: new Date()
    }
    
    history.value.unshift(newAction)
    
    if (action.status === 'pending') {
      pendingActions.value.add(newAction.id)
    }
    
    // Keep only last 100 actions
    if (history.value.length > 100) {
      history.value = history.value.slice(0, 100)
    }
    
    return newAction.id
  }

  function updateActionStatus(id: string, status: ActionHistoryItem['status'], details?: Record<string, any>) {
    const action = history.value.find(a => a.id === id)
    if (action) {
      action.status = status
      if (details) {
        action.details = { ...action.details, ...details }
      }
      
      if (status !== 'pending') {
        pendingActions.value.delete(id)
      }
    }
  }

  function clearHistory() {
    history.value = []
    pendingActions.value.clear()
  }

  return {
    history,
    pendingActions,
    addAction,
    updateActionStatus,
    clearHistory
  }
})
