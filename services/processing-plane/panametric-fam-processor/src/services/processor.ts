import { v4 as uuidv4 } from 'uuid';
import { EncabulationRequest, EncabulationResult } from '../types';
import { DataPlaneClient } from './dataPlaneClient';
import { logger } from '../utils/logger';

export class EncabulationProcessor {
  private dataPlane: DataPlaneClient;

  constructor(dataPlane: DataPlaneClient) {
    this.dataPlane = dataPlane;
  }

  async process(request: EncabulationRequest): Promise<EncabulationResult> {
    const startTime = Date.now();
    const requestId = uuidv4();

    logger.info('Processing encabulation request', { requestId, request });

    try {
      // Check cache first
      const cacheKey = this.getCacheKey(request);
      const cached = await this.dataPlane.getCached(cacheKey);
      
      if (cached) {
        logger.info('Returning cached result', { requestId });
        return {
          ...cached,
          requestId,
          processingTime: Date.now() - startTime,
        };
      }

      // Perform calculations
      const result = this.calculate(request);

      // Store in database
      const stored = await this.dataPlane.createEncabulation({
        grammeters: request.grammeters,
        prefabulation: request.prefabulation,
        sinusoidalMode: request.sinusoidalMode,
      });

      // Cache the result
      await this.dataPlane.setCache(cacheKey, result, 3600);

      // Write metrics
      await this.writeMetrics(request, result, Date.now() - startTime);

      const finalResult: EncabulationResult = {
        id: stored.id,
        requestId,
        status: 'SUCCESS',
        encabulationFactor: result.encabulationFactor,
        dingleArmPosition: result.dingleArmPosition,
        prefabulationRate: result.prefabulationRate,
        processingTime: Date.now() - startTime,
        timestamp: new Date().toISOString(),
      };

      logger.info('Encabulation processed successfully', { 
        requestId, 
        processingTime: finalResult.processingTime 
      });

      return finalResult;
    } catch (error: any) {
      logger.error('Encabulation processing failed', { requestId, error: error.message });
      throw error;
    }
  }

  private calculate(request: EncabulationRequest): any {
    // Business logic calculations
    const baseMultiplier = request.sinusoidalMode === 'LOGARITHMIC' ? 2.0 : 1.5;
    const prefabMultiplier = request.prefabulation ? 2.5 : 1.0;

    return {
      encabulationFactor: request.grammeters * baseMultiplier,
      dingleArmPosition: request.grammeters * 0.75,
      prefabulationRate: request.grammeters * prefabMultiplier,
    };
  }

  private getCacheKey(request: EncabulationRequest): string {
    return `encab:${request.grammeters}:${request.prefabulation}:${request.sinusoidalMode}`;
  }

  private async writeMetrics(request: EncabulationRequest, result: any, processingTime: number): Promise<void> {
    const points = [
      {
        measurement: 'encabulation_processing',
        tags: {
          sinusoidalMode: request.sinusoidalMode,
          prefabulation: request.prefabulation.toString(),
        },
        fields: {
          processingTime,
          grammeters: request.grammeters,
          encabulationFactor: result.encabulationFactor,
        },
        timestamp: new Date().toISOString(),
      },
    ];

    await this.dataPlane.writeMetrics(points);
  }
}
