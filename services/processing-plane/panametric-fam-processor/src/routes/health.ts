import { Router, Request, Response } from 'express';
import { DataPlaneClient } from '../services/dataPlaneClient';
import { HealthCheck } from '../types';

export function createHealthRouter(dataPlane: DataPlaneClient): Router {
  const router = Router();

  router.get('/health', (req: Request, res: Response) => {
    const health: HealthCheck = {
      status: 'healthy',
      timestamp: new Date().toISOString(),
      checks: {},
    };

    res.json(health);
  });

  router.get('/ready', async (req: Request, res: Response) => {
    try {
      const checks = await dataPlane.checkHealth();
      const allHealthy = Object.values(checks).every(v => v === true);

      const health: HealthCheck = {
        status: allHealthy ? 'healthy' : 'unhealthy',
        timestamp: new Date().toISOString(),
        checks: Object.entries(checks).reduce((acc, [key, value]) => {
          acc[key] = value ? 'ok' : 'unavailable';
          return acc;
        }, {} as { [key: string]: string }),
      };

      res.status(allHealthy ? 200 : 503).json(health);
    } catch (error) {
      res.status(503).json({
        status: 'unhealthy',
        timestamp: new Date().toISOString(),
        checks: { error: 'health check failed' },
      });
    }
  });

  return router;
}
