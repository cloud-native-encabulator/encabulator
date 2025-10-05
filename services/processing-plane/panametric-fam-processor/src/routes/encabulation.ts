import { Router, Request, Response } from 'express';
import { EncabulationProcessor } from '../services/processor';
import { EncabulationRequest } from '../types';
import { logger } from '../utils/logger';
import { encabulationsProcessed, encabulationProcessingDuration } from '../utils/metrics';

export function createEncabulationRouter(processor: EncabulationProcessor): Router {
  const router = Router();

  router.post('/', async (req: Request, res: Response) => {
    const end = encabulationProcessingDuration.startTimer();
    
    try {
      const request: EncabulationRequest = req.body;

      // Validate request
      if (!request.grammeters || request.grammeters <= 0) {
        return res.status(400).json({
          error: 'Bad Request',
          code: 'INVALID_GRAMMETERS',
          message: 'Grammeters must be a positive number',
        });
      }

      if (!['STANDARD', 'INVERSE', 'LOGARITHMIC'].includes(request.sinusoidalMode)) {
        return res.status(400).json({
          error: 'Bad Request',
          code: 'INVALID_MODE',
          message: 'Invalid sinusoidal mode',
        });
      }

      const result = await processor.process(request);
      
      encabulationsProcessed.inc({ status: 'success' });
      end();

      res.status(201).json(result);
    } catch (error: any) {
      logger.error('Encabulation request failed', { error: error.message });
      encabulationsProcessed.inc({ status: 'error' });
      end();

      res.status(500).json({
        error: 'Internal Server Error',
        code: 'PROCESSING_ERROR',
        message: 'Failed to process encabulation',
      });
    }
  });

  return router;
}
