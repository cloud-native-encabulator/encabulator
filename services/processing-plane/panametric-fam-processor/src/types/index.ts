export interface EncabulationRequest {
  grammeters: number;
  prefabulation: boolean;
  sinusoidalMode: 'STANDARD' | 'INVERSE' | 'LOGARITHMIC';
  priority?: 'LOW' | 'MEDIUM' | 'HIGH';
}

export interface EncabulationResult {
  id: string;
  requestId: string;
  status: 'SUCCESS' | 'PARTIAL' | 'FAILED';
  encabulationFactor: number;
  dingleArmPosition: number;
  prefabulationRate: number;
  processingTime: number;
  timestamp: string;
}

export interface ProcessingMetrics {
  requestsProcessed: number;
  averageProcessingTime: number;
  successRate: number;
  cacheHitRate: number;
}

export interface HealthCheck {
  status: 'healthy' | 'unhealthy';
  timestamp: string;
  checks: {
    [key: string]: string;
  };
}

export interface ErrorResponse {
  error: string;
  code: string;
  message: string;
}
