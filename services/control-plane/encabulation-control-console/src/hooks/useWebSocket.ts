import { useEffect, useState, useCallback, useRef } from 'react'
import { io, Socket } from 'socket.io-client'

interface UseWebSocketOptions {
  url: string
  onMessage?: (data: any) => void
  reconnectInterval?: number
}

export function useWebSocket({ url, onMessage, reconnectInterval = 3000 }: UseWebSocketOptions) {
  const [connected, setConnected] = useState(false)
  const socketRef = useRef<Socket | null>(null)

  useEffect(() => {
    const socket = io(url, {
      transports: ['websocket'],
      reconnectionDelay: reconnectInterval,
    })

    socket.on('connect', () => {
      console.log('WebSocket connected')
      setConnected(true)
    })

    socket.on('disconnect', () => {
      console.log('WebSocket disconnected')
      setConnected(false)
    })

    socket.on('message', (data) => {
      if (onMessage) {
        onMessage(data)
      }
    })

    socketRef.current = socket

    return () => {
      socket.disconnect()
    }
  }, [url, onMessage, reconnectInterval])

  const sendMessage = useCallback((data: any) => {
    if (socketRef.current?.connected) {
      socketRef.current.emit('message', data)
    }
  }, [])

  return { connected, sendMessage }
}
