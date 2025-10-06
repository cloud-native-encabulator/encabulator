import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useWebSocket } from './useWebSocket'
import { io } from 'socket.io-client'

vi.mock('socket.io-client')

describe('useWebSocket', () => {
  let mockSocket: any

  beforeEach(() => {
    mockSocket = {
      on: vi.fn(),
      emit: vi.fn(),
      disconnect: vi.fn(),
      connected: false,
    }
    
    vi.mocked(io).mockReturnValue(mockSocket as any)
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('initializes socket connection with correct URL', () => {
    const url = 'http://localhost:3001'
    renderHook(() => useWebSocket({ url }))
    
    expect(io).toHaveBeenCalledWith(url, {
      transports: ['websocket'],
      reconnectionDelay: 3000,
    })
  })

  it('uses custom reconnect interval when provided', () => {
    const url = 'http://localhost:3001'
    const reconnectInterval = 5000
    
    renderHook(() => useWebSocket({ url, reconnectInterval }))
    
    expect(io).toHaveBeenCalledWith(url, {
      transports: ['websocket'],
      reconnectionDelay: 5000,
    })
  })

  it('registers event listeners on mount', () => {
    const url = 'http://localhost:3001'
    renderHook(() => useWebSocket({ url }))
    
    expect(mockSocket.on).toHaveBeenCalledWith('connect', expect.any(Function))
    expect(mockSocket.on).toHaveBeenCalledWith('disconnect', expect.any(Function))
    expect(mockSocket.on).toHaveBeenCalledWith('message', expect.any(Function))
  })

  it('sets connected state to true on connect event', async () => {
    const url = 'http://localhost:3001'
    const { result } = renderHook(() => useWebSocket({ url }))
    
    expect(result.current.connected).toBe(false)
    
    const connectHandler = mockSocket.on.mock.calls.find((call: any) => call[0] === 'connect')?.[1]
    
    await act(async () => {
      connectHandler()
    })
    
    await waitFor(() => {
      expect(result.current.connected).toBe(true)
    })
  })

  it('sets connected state to false on disconnect event', async () => {
    const url = 'http://localhost:3001'
    const { result } = renderHook(() => useWebSocket({ url }))
    
    const connectHandler = mockSocket.on.mock.calls.find((call: any) => call[0] === 'connect')?.[1]
    connectHandler()
    
    await waitFor(() => {
      expect(result.current.connected).toBe(true)
    })
    
    const disconnectHandler = mockSocket.on.mock.calls.find((call: any) => call[0] === 'disconnect')?.[1]
    disconnectHandler()
    
    await waitFor(() => {
      expect(result.current.connected).toBe(false)
    })
  })

  it('calls onMessage callback when message is received', () => {
    const url = 'http://localhost:3001'
    const onMessage = vi.fn()
    const testData = { type: 'test', payload: 'data' }
    
    renderHook(() => useWebSocket({ url, onMessage }))
    
    const messageHandler = mockSocket.on.mock.calls.find((call: any) => call[0] === 'message')?.[1]
    messageHandler(testData)
    
    expect(onMessage).toHaveBeenCalledWith(testData)
  })

  it('sendMessage emits message when socket is connected', () => {
    const url = 'http://localhost:3001'
    mockSocket.connected = true
    
    const { result } = renderHook(() => useWebSocket({ url }))
    
    const testMessage = { action: 'test' }
    result.current.sendMessage(testMessage)
    
    expect(mockSocket.emit).toHaveBeenCalledWith('message', testMessage)
  })

  it('sendMessage does not emit when socket is not connected', () => {
    const url = 'http://localhost:3001'
    mockSocket.connected = false
    
    const { result } = renderHook(() => useWebSocket({ url }))
    
    const testMessage = { action: 'test' }
    result.current.sendMessage(testMessage)
    
    expect(mockSocket.emit).not.toHaveBeenCalled()
  })

  it('disconnects socket on unmount', () => {
    const url = 'http://localhost:3001'
    const { unmount } = renderHook(() => useWebSocket({ url }))
    
    unmount()
    
    expect(mockSocket.disconnect).toHaveBeenCalled()
  })
})
