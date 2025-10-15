import { createClient, RedisClientType } from 'redis'

export class RedisClient {
  private client: RedisClientType | null = null
  private connected = false
  private readonly redisUrl: string

  constructor() {
    this.redisUrl = process.env.REDIS_URL || 'redis://prefabulated-cache-redis.encabulator-data.svc.cluster.local:6379'
  }

  async connect(): Promise<void> {
    if (this.connected) return

    try {
      this.client = createClient({ url: this.redisUrl })
      
      this.client.on('error', (err) => {
        console.error('Redis Client Error:', err)
        this.connected = false
      })

      this.client.on('connect', () => {
        console.log('Redis connected')
        this.connected = true
      })

      await this.client.connect()
    } catch (error) {
      console.error('Failed to connect to Redis:', error)
      this.connected = false
    }
  }

  async get(key: string): Promise<string | null> {
    if (!this.client || !this.connected) {
      console.warn('Redis not connected, returning null')
      return null
    }

    try {
      return await this.client.get(key)
    } catch (error) {
      console.error('Redis GET error:', error)
      return null
    }
  }

  async set(key: string, value: string, expirySeconds?: number): Promise<boolean> {
    if (!this.client || !this.connected) {
      console.warn('Redis not connected, skipping SET')
      return false
    }

    try {
      if (expirySeconds) {
        await this.client.setEx(key, expirySeconds, value)
      } else {
        await this.client.set(key, value)
      }
      return true
    } catch (error) {
      console.error('Redis SET error:', error)
      return false
    }
  }

  async del(key: string): Promise<boolean> {
    if (!this.client || !this.connected) {
      console.warn('Redis not connected, skipping DEL')
      return false
    }

    try {
      await this.client.del(key)
      return true
    } catch (error) {
      console.error('Redis DEL error:', error)
      return false
    }
  }

  isConnected(): boolean {
    return this.connected
  }

  async disconnect(): Promise<void> {
    if (this.client) {
      await this.client.quit()
      this.connected = false
    }
  }
}
