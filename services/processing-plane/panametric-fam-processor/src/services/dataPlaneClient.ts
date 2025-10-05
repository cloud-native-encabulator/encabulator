import axios, { AxiosInstance } from 'axios';
import { logger } from '../utils/logger';

export class DataPlaneClient {
  private hydrocopticeClient: AxiosInstance;
  private cacheClient: AxiosInstance;
  private sinusoidalClient: AxiosInstance;

  constructor() {
    const hydrocopticeUrl = process.env.HYDROCOPTIC_URL || 'http://hydrocoptic-datastore:8080';
    const cacheUrl = process.env.CACHE_URL || 'http://prefabulated-cache:8080';
    const sinusoidalUrl = process.env.SINUSOIDAL_URL || 'http://sinusoidal-database:8080';

    this.hydrocopticeClient = axios.create({
      baseURL: hydrocopticeUrl,
      timeout: 5000,
    });

    this.cacheClient = axios.create({
      baseURL: cacheUrl,
      timeout: 2000,
    });

    this.sinusoidalClient = axios.create({
      baseURL: sinusoidalUrl,
      timeout: 3000,
    });
  }

  async createEncabulation(data: any): Promise<any> {
    try {
      const response = await this.hydrocopticeClient.post('/api/v1/encabulations', data);
      logger.info('Created encabulation', { id: response.data.id });
      return response.data;
    } catch (error: any) {
      logger.error('Failed to create encabulation', { error: error.message });
      throw error;
    }
  }

  async getEncabulation(id: string): Promise<any> {
    try {
      const response = await this.hydrocopticeClient.get(`/api/v1/encabulations/${id}`);
      return response.data;
    } catch (error: any) {
      logger.error('Failed to get encabulation', { id, error: error.message });
      throw error;
    }
  }

  async getCached(key: string): Promise<any | null> {
    try {
      const response = await this.cacheClient.get(`/api/v1/cache/${key}`);
      if (response.data.found) {
        logger.debug('Cache hit', { key });
        return response.data.value;
      }
      logger.debug('Cache miss', { key });
      return null;
    } catch (error: any) {
      logger.warn('Cache lookup failed', { key, error: error.message });
      return null;
    }
  }

  async setCache(key: string, value: any, ttl: number = 3600): Promise<void> {
    try {
      await this.cacheClient.post('/api/v1/cache', { key, value, ttl });
      logger.debug('Cache set', { key, ttl });
    } catch (error: any) {
      logger.warn('Cache set failed', { key, error: error.message });
    }
  }

  async writeMetrics(points: any[]): Promise<void> {
    try {
      await this.sinusoidalClient.post('/api/v1/metrics', { points });
      logger.debug('Metrics written', { count: points.length });
    } catch (error: any) {
      logger.warn('Metrics write failed', { error: error.message });
    }
  }

  async checkHealth(): Promise<{ [key: string]: boolean }> {
    const health: { [key: string]: boolean } = {};

    try {
      await this.hydrocopticeClient.get('/ready');
      health.hydrocoptic = true;
    } catch {
      health.hydrocoptic = false;
    }

    try {
      await this.cacheClient.get('/ready');
      health.cache = true;
    } catch {
      health.cache = false;
    }

    try {
      await this.sinusoidalClient.get('/ready');
      health.sinusoidal = true;
    } catch {
      health.sinusoidal = false;
    }

    return health;
  }
}
