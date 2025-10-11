import * as k8s from '@kubernetes/client-node'

export class KubernetesService {
  private kc: k8s.KubeConfig
  private k8sApi: k8s.AppsV1Api
  private coreApi: k8s.CoreV1Api

  constructor() {
    this.kc = new k8s.KubeConfig()
    
    // Load config from cluster or local kubeconfig
    try {
      this.kc.loadFromCluster()
    } catch {
      this.kc.loadFromDefault()
    }

    this.k8sApi = this.kc.makeApiClient(k8s.AppsV1Api)
    this.coreApi = this.kc.makeApiClient(k8s.CoreV1Api)
  }

  async checkConnection(): Promise<void> {
    await this.coreApi.listNamespace()
  }

  async getSystemStatus(): Promise<any> {
    try {
      const namespaces = [
        'encabulator-system', 
        'encabulator-data', 
        'encabulator-control',
        'encabulator',
        'encabulator-support'
      ]
      const services: Record<string, any> = {}

      for (const namespace of namespaces) {
        try {
          const deployments = await this.k8sApi.listNamespacedDeployment(namespace)
          
          for (const deployment of deployments.body.items) {
            const name = deployment.metadata?.name || 'unknown'
            const replicas = deployment.status?.replicas || 0
            const readyReplicas = deployment.status?.readyReplicas || 0
            
            // Fetch individual pod data for this deployment
            const pods = await this.getPodsForDeployment(namespace, name)
            
            services[name] = {
              status: readyReplicas === replicas && replicas > 0 ? 'healthy' : 
                      replicas > 0 ? 'degraded' : 'unhealthy',
              replicas: replicas,
              cpu: pods.length > 0 ? pods[0].cpu : 'N/A',
              memory: pods.length > 0 ? pods[0].memory : 'N/A',
              pods: pods
            }
          }
        } catch (error) {
          console.error(`Error fetching deployments from ${namespace}:`, error)
        }
      }

      return { services }
    } catch (error) {
      console.error('Error getting system status:', error)
      return { services: {} }
    }
  }

  private async getPodsForDeployment(namespace: string, deploymentName: string): Promise<any[]> {
    try {
      // Get pods for this deployment
      const pods = await this.coreApi.listNamespacedPod(
        namespace,
        undefined,
        undefined,
        undefined,
        undefined,
        `app.kubernetes.io/name=${deploymentName}`
      )

      if (pods.body.items.length === 0) {
        return []
      }

      const metricsClient = new k8s.Metrics(this.kc)
      const podData = []

      for (const pod of pods.body.items) {
        const podName = pod.metadata?.name || 'unknown'
        const podStatus = pod.status?.phase || 'Unknown'
        const ready = pod.status?.conditions?.find(c => c.type === 'Ready')?.status === 'True'

        let cpu = 'N/A'
        let memory = 'N/A'

        // Try to get metrics for this pod
        try {
          const metricsResponse = await metricsClient.getPodMetrics(namespace, podName)
          const containers = metricsResponse.containers || []
          
          if (containers.length > 0) {
            const cpuUsage = containers[0].usage?.cpu || '0'
            const memUsage = containers[0].usage?.memory || '0'
            
            cpu = this.formatCpu(cpuUsage)
            memory = this.formatMemory(memUsage)
          }
        } catch (metricsError) {
          // Metrics not available for this pod
          console.debug(`Metrics not available for pod ${podName}`)
        }

        podData.push({
          name: podName,
          status: podStatus,
          ready: ready,
          cpu: cpu,
          memory: memory
        })
      }

      return podData
    } catch (error) {
      console.error(`Error fetching pods for ${deploymentName}:`, error)
      return []
    }
  }

  private formatCpu(cpuString: string): string {
    // CPU is in nanocores (n) or millicores (m)
    if (cpuString.endsWith('n')) {
      const nanocores = parseInt(cpuString.slice(0, -1))
      const millicores = Math.round(nanocores / 1000000)
      return `${millicores}m`
    }
    return cpuString
  }

  private formatMemory(memString: string): string {
    // Memory is in Ki, Mi, Gi, etc.
    if (memString.endsWith('Ki')) {
      const kb = parseInt(memString.slice(0, -2))
      const mb = Math.round(kb / 1024)
      return `${mb}Mi`
    }
    return memString
  }

  async scaleService(serviceName: string, direction: 'up' | 'down'): Promise<void> {
    const namespaces = ['encabulator-system', 'encabulator-data', 'encabulator-control']
    
    for (const namespace of namespaces) {
      try {
        const deployment = await this.k8sApi.readNamespacedDeployment(serviceName, namespace)
        const currentReplicas = deployment.body.spec?.replicas || 1
        const newReplicas = direction === 'up' ? currentReplicas + 1 : Math.max(1, currentReplicas - 1)

        await this.k8sApi.patchNamespacedDeploymentScale(
          serviceName,
          namespace,
          { spec: { replicas: newReplicas } },
          undefined,
          undefined,
          undefined,
          undefined,
          undefined,
          { headers: { 'Content-Type': 'application/merge-patch+json' } }
        )
        
        console.log(`Scaled ${serviceName} to ${newReplicas} replicas`)
        return
      } catch (error) {
        // Try next namespace
        continue
      }
    }
    
    throw new Error(`Service ${serviceName} not found`)
  }

  async injectFailure(serviceName: string): Promise<void> {
    // This would integrate with the Malaise Injection Engine
    // For now, just log the action
    console.log(`Injecting failure into ${serviceName}`)
  }

  async runScenario(scenario: string): Promise<void> {
    // This would integrate with the Demo Scenario Orchestrator
    // For now, just log the action
    console.log(`Running scenario: ${scenario}`)
  }
}
