import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useActionsStore } from '../actions'

describe('Actions Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('initializes with empty history', () => {
    const store = useActionsStore()
    
    expect(store.history).toEqual([])
    expect(store.pendingActions.size).toBe(0)
  })

  it('adds action to history', () => {
    const store = useActionsStore()
    
    const actionId = store.addAction({
      type: 'scale_service',
      description: 'Scale up service',
      status: 'pending'
    })
    
    expect(actionId).toBeTruthy()
    expect(store.history).toHaveLength(1)
    expect(store.history[0].type).toBe('scale_service')
    expect(store.history[0].timestamp).toBeInstanceOf(Date)
  })

  it('tracks pending actions', () => {
    const store = useActionsStore()
    
    const actionId = store.addAction({
      type: 'test_action',
      description: 'Test',
      status: 'pending'
    })
    
    expect(store.pendingActions.has(actionId)).toBe(true)
  })

  it('updates action status', () => {
    const store = useActionsStore()
    
    const actionId = store.addAction({
      type: 'test_action',
      description: 'Test',
      status: 'pending'
    })
    
    store.updateActionStatus(actionId, 'success', { result: 'ok' })
    
    expect(store.history[0].status).toBe('success')
    expect(store.history[0].details?.result).toBe('ok')
    expect(store.pendingActions.has(actionId)).toBe(false)
  })

  it('adds new actions to the beginning of history', () => {
    const store = useActionsStore()
    
    store.addAction({
      type: 'action1',
      description: 'First',
      status: 'success'
    })
    
    store.addAction({
      type: 'action2',
      description: 'Second',
      status: 'success'
    })
    
    expect(store.history[0].type).toBe('action2')
    expect(store.history[1].type).toBe('action1')
  })

  it('limits history to 100 items', () => {
    const store = useActionsStore()
    
    // Add 105 actions
    for (let i = 0; i < 105; i++) {
      store.addAction({
        type: `action${i}`,
        description: `Action ${i}`,
        status: 'success'
      })
    }
    
    expect(store.history).toHaveLength(100)
  })

  it('clears history', () => {
    const store = useActionsStore()
    
    store.addAction({
      type: 'test',
      description: 'Test',
      status: 'pending'
    })
    
    store.clearHistory()
    
    expect(store.history).toEqual([])
    expect(store.pendingActions.size).toBe(0)
  })
})
