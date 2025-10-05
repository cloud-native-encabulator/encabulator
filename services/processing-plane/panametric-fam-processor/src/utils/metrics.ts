import { Registry, Counter, Histogram, Gauge } from 'prom-client';

export const register = new Registry();

export const httpRequestDuration = new Histogram({
  name: 'http_request_duration_seconds',
  help: 'Duration of HTTP requests in seconds',
  labelNames: ['method', 'route', 'status_code'],
  registers: [register],
});

export const httpRequestTotal = new Counter({
  name: 'http_requests_total',
  help: 'Total number of HTTP requests',
  labelNames: ['method', 'route', 'status_code'],
  registers: [register],
});

export const encabulationsProcessed = new Counter({
  name: 'encabulations_processed_total',
  help: 'Total number of encabulations processed',
  labelNames: ['status'],
  registers: [register],
});

export const encabulationProcessingDuration = new Histogram({
  name: 'encabulation_processing_duration_seconds',
  help: 'Duration of encabulation processing in seconds',
  buckets: [0.1, 0.5, 1, 2, 5],
  registers: [register],
});

export const cacheHitRate = new Gauge({
  name: 'cache_hit_rate',
  help: 'Cache hit rate',
  registers: [register],
});
