import { EncabulationProcessor } from './processor';
import { DataPlaneClient } from './dataPlaneClient';
import { EncabulationRequest } from '../types';

// Mock the DataPlaneClient
jest.mock('./dataPlaneClient');

describe('EncabulationProcessor', () => {
  let processor: EncabulationProcessor;
  let mockDataPlane: jest.Mocked<DataPlaneClient>;

  beforeEach(() => {
    mockDataPlane = new DataPlaneClient() as jest.Mocked<DataPlaneClient>;
    processor = new EncabulationProcessor(mockDataPlane);

    // Setup default mocks
    mockDataPlane.getCached = jest.fn().mockResolvedValue(null);
    mockDataPlane.createEncabulation = jest.fn().mockResolvedValue({ id: 'test-id' });
    mockDataPlane.setCache = jest.fn().mockResolvedValue(undefined);
    mockDataPlane.writeMetrics = jest.fn().mockResolvedValue(undefined);
  });

  it('should process encabulation request successfully', async () => {
    const request: EncabulationRequest = {
      grammeters: 10,
      prefabulation: true,
      sinusoidalMode: 'STANDARD',
    };

    const result = await processor.process(request);

    expect(result.status).toBe('SUCCESS');
    expect(result.encabulationFactor).toBe(15); // 10 * 1.5
    expect(result.dingleArmPosition).toBe(7.5); // 10 * 0.75
    expect(result.prefabulationRate).toBe(25); // 10 * 2.5
    expect(mockDataPlane.createEncabulation).toHaveBeenCalled();
  });

  it('should use logarithmic multiplier for LOGARITHMIC mode', async () => {
    const request: EncabulationRequest = {
      grammeters: 10,
      prefabulation: false,
      sinusoidalMode: 'LOGARITHMIC',
    };

    const result = await processor.process(request);

    expect(result.encabulationFactor).toBe(20); // 10 * 2.0
    expect(result.prefabulationRate).toBe(10); // 10 * 1.0 (no prefabulation)
  });

  it('should return cached result if available', async () => {
    const cachedResult = {
      encabulationFactor: 15,
      dingleArmPosition: 7.5,
      prefabulationRate: 25,
    };

    mockDataPlane.getCached = jest.fn().mockResolvedValue(cachedResult);

    const request: EncabulationRequest = {
      grammeters: 10,
      prefabulation: true,
      sinusoidalMode: 'STANDARD',
    };

    const result = await processor.process(request);

    expect(result.encabulationFactor).toBe(15);
    expect(mockDataPlane.createEncabulation).not.toHaveBeenCalled();
  });
});
