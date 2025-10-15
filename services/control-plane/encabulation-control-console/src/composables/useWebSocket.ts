import { ref, onMounted, onUnmounted } from 'vue'
import { io, Socket } from 'socket.io-client'
import { useSystemStore } from '@/stores/system'

export interface WebSocketMessage {
  type: string
  payload: any
}

export function useWebSocket() {
  const socket = ref<Socket | null>(null)
  const systemStore = useSystemStore()
  
  const wsUrl = import.meta.env.VITE_WS_URL || 'ws://localhost:8080'

  function connect() {
    socket.value = io(wsUrl, {
      transports: ['websocket'],
      reconnection: true,
      reconnectionDelay: 1000,
      reconnectionAttempts: 10
    })

    socket.value.on('connect', () => {
      console.log('WebSocket connected')
      systemStore.setConnected(true)
    })

    socket.value.on('disconnect', () => {
      console.log('WebSocket disconnected')
      systemStore.setConnected(false)
    })

    socket.value.on('message', (data: WebSocketMessage) => {
      handleMessage(data)
    })

    socket.value.on('error', (error: Error) => {
      console.error('WebSocket error:', error)
    })
  }

  function disconnect() {
    if (socket.value) {
      socket.value.disconnect()
      socket.value = null
    }
  }

  function sendMessage(message: WebSocketMessage) {
    if (socket.value?.connected) {
      socket.value.emit('message', message)
    } else {
      console.warn('WebSocket not connected, cannot send message')
    }
  }

  function handleMessage(data: WebSocketMessage) {
    switch (data.type) {
      case 'system_status':
        if (data.payload.services) {
          systemStore.updateServices(data.payload.services)
        }
        if (data.payload.metrics) {
          systemStore.updateMetrics(data.payload.metrics)
        }
        if (data.payload.trafficMetrics) {
          console.log('Received trafficMetrics:', data.payload.trafficMetrics.length, 'metrics')
          systemStore.updateTrafficMetrics(data.payload.trafficMetrics)
        } else {
          console.log('No trafficMetrics in payload')
        }
        break
      
      case 'service_update':
        if (data.payload.service && data.payload.status) {
          systemStore.updateServiceStatus(data.payload.service, data.payload.status)
        }
        break
      
      case 'error':
        console.error('Server error:', data.payload)
        break
      
      default:
        console.log('Unknown message type:', data.type)
    }
  }

  onMounted(() => {
    connect()
  })

  onUnmounted(() => {
    disconnect()
  })

  return {
    socket,
    sendMessage,
    connect,
    disconnect
  }
}
