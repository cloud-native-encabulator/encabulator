import { KubernetesService } from './kubernetes'
import * as k8s from '@kubernetes/client-node'

jest.mock('@kubernetes/client-node')

describe('KubernetesService', () => {
  let service: KubernetesService
  let mockK8sApi: jest.Mocked<k8s.AppsV1Api>
  let mockCoreApi: jest.Mocked<k8s.CoreV1Api>
  let mockKubeConfig: jest.Mocked<k8s.KubeConfig>

  beforeEach(() => {
    mockK8sApi = {
      listNamespacedDeployment: jest.fn(),
      readNamespacedDeployment: jest.fn(),
      patchNamespacedDeploymentScale: jest.fn(),
    } as any

    mockCoreApi = {
      listNamespace: jest.fn(),
    } as any

    mockKubeConfig = {
      loadFromCluster: jest.fn(),
      loadFromDefault: jest.fn(),
      makeApiClient: jest.fn((apiType) => {
        if (apiType === k8s.AppsV1Api) return mockK8sApi
        if (apiType === k8s.CoreV1Api) return mockCoreApi
        return {} as any
      }),
    } as any

    jest.mocked(k8s.KubeConfig).mockImplementation(() => mockKubeConfig)

    service = new KubernetesService()
  })

  afterEach(() => {
    jest.clearAllMocks()
  })

  describe('constructor', () => {
    it('loads config from cluster when available', () => {
      expect(mockKubeConfig.loadFromCluster).toHaveBeenCalled()
    })

    it('falls back to default config when cluster config fails', () => {
      mockKubeConfig.loadFromCluster.mockImplementation(() => {
        throw new Error('Not in cluster')
      })

      new KubernetesService()

      expect(mockKubeConfig.loadFromDefault).toHaveBeenCalled()
    })

    it('creates API clients', () => {
      expect(mockKubeConfig.makeApiClient).toHaveBeenCalledWith(k8s.AppsV1Api)
      expect(mockKubeConfig.makeApiClient).toHaveBeenCalledWith(k8s.CoreV1Api)
    })
  })

  describe('checkConnection', () => {
    it('successfully checks connection', async () => {
      mockCoreApi.listNamespace.mockResolvedValue({} as any)

      await expect(service.checkConnection()).resolves.not.toThrow()
      expect(mockCoreApi.listNamespace).toHaveBeenCalled()
    })

    it('throws error when connection fails', async () => {
      mockCoreApi.listNamespace.mockRejectedValue(new Error('Connection failed'))

      await expect(service.checkConnection()).rejects.toThrow('Connection failed')
    })
  })

  describe('getSystemStatus', () => {
    it('returns system status with healthy services', async () => {
      const mockDeployments = {
        body: {
          items: [
            {
              metadata: { name: 'panametric-fam-processor' },
              status: { replicas: 3, readyReplicas: 3 },
            },
            {
              metadata: { name: 'dingle-arm-api-gateway' },
              status: { replicas: 2, readyReplicas: 2 },
            },
          ],
        },
      }

      mockK8sApi.listNamespacedDeployment.mockResolvedValue(mockDeployments as any)

      const status = await service.getSystemStatus()

      expect(status.services['panametric-fam-processor']).toEqual({
        status: 'healthy',
        replicas: 3,
      })
      expect(status.services['dingle-arm-api-gateway']).toEqual({
        status: 'healthy',
        replicas: 2,
      })
    })

    it('returns degraded status when replicas are not ready', async () => {
      const mockDeployments = {
        body: {
          items: [
            {
              metadata: { name: 'test-service' },
              status: { replicas: 3, readyReplicas: 1 },
            },
          ],
        },
      }

      mockK8sApi.listNamespacedDeployment.mockResolvedValue(mockDeployments as any)

      const status = await service.getSystemStatus()

      expect(status.services['test-service']).toEqual({
        status: 'degraded',
        replicas: 3,
      })
    })

    it('handles missing metadata gracefully', async () => {
      const mockDeployments = {
        body: {
          items: [
            {
              metadata: undefined,
              status: { replicas: 1, readyReplicas: 1 },
            },
          ],
        },
      }

      mockK8sApi.listNamespacedDeployment.mockResolvedValue(mockDeployments as any)

      const status = await service.getSystemStatus()

      expect(status.services['unknown']).toEqual({
        status: 'healthy',
        replicas: 1,
      })
    })

    it('returns empty services on error', async () => {
      mockK8sApi.listNamespacedDeployment.mockRejectedValue(new Error('API error'))

      const status = await service.getSystemStatus()

      expect(status).toEqual({ services: {} })
    })
  })

  describe('scaleService', () => {
    it('scales service up successfully', async () => {
      const mockDeployment = {
        body: {
          spec: { replicas: 2 },
        },
      }

      mockK8sApi.readNamespacedDeployment.mockResolvedValue(mockDeployment as any)
      mockK8sApi.patchNamespacedDeploymentScale.mockResolvedValue({} as any)

      await service.scaleService('test-service', 'up')

      expect(mockK8sApi.patchNamespacedDeploymentScale).toHaveBeenCalledWith(
        'test-service',
        expect.any(String),
        { spec: { replicas: 3 } },
        undefined,
        undefined,
        undefined,
        undefined,
        undefined,
        { headers: { 'Content-Type': 'application/merge-patch+json' } }
      )
    })

    it('scales service down successfully', async () => {
      const mockDeployment = {
        body: {
          spec: { replicas: 3 },
        },
      }

      mockK8sApi.readNamespacedDeployment.mockResolvedValue(mockDeployment as any)
      mockK8sApi.patchNamespacedDeploymentScale.mockResolvedValue({} as any)

      await service.scaleService('test-service', 'down')

      expect(mockK8sApi.patchNamespacedDeploymentScale).toHaveBeenCalledWith(
        'test-service',
        expect.any(String),
        { spec: { replicas: 2 } },
        undefined,
        undefined,
        undefined,
        undefined,
        undefined,
        { headers: { 'Content-Type': 'application/merge-patch+json' } }
      )
    })

    it('does not scale below 1 replica', async () => {
      const mockDeployment = {
        body: {
          spec: { replicas: 1 },
        },
      }

      mockK8sApi.readNamespacedDeployment.mockResolvedValue(mockDeployment as any)
      mockK8sApi.patchNamespacedDeploymentScale.mockResolvedValue({} as any)

      await service.scaleService('test-service', 'down')

      expect(mockK8sApi.patchNamespacedDeploymentScale).toHaveBeenCalledWith(
        'test-service',
        expect.any(String),
        { spec: { replicas: 1 } },
        undefined,
        undefined,
        undefined,
        undefined,
        undefined,
        { headers: { 'Content-Type': 'application/merge-patch+json' } }
      )
    })

    it('throws error when service not found', async () => {
      mockK8sApi.readNamespacedDeployment.mockRejectedValue(new Error('Not found'))

      await expect(service.scaleService('nonexistent-service', 'up')).rejects.toThrow(
        'Service nonexistent-service not found'
      )
    })
  })

  describe('injectFailure', () => {
    it('logs failure injection', async () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation()

      await service.injectFailure('test-service')

      expect(consoleSpy).toHaveBeenCalledWith('Injecting failure into test-service')

      consoleSpy.mockRestore()
    })
  })

  describe('runScenario', () => {
    it('logs scenario execution', async () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation()

      await service.runScenario('startup')

      expect(consoleSpy).toHaveBeenCalledWith('Running scenario: startup')

      consoleSpy.mockRestore()
    })
  })
})
