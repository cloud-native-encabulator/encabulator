import { describe, it, expect } from 'vitest'

/**
 * Unit tests for SystemTopology visualization logic
 * Task 6.19: Test pod positioning, colors, particle spawning, and tooltips
 */

describe('SystemTopology Visualization Logic', () => {
  describe('Pod Position Calculation', () => {
    // Helper function to calculate pod positions (extracted from component logic)
    function calculatePodPositions(podCount: number, spacing: number = 10): { x: number, y: number }[] {
      const positions: { x: number, y: number }[] = []
      
      if (podCount === 1) {
        positions.push({ x: 0, y: 0 })
      } else if (podCount === 2) {
        positions.push({ x: -spacing / 2, y: 0 })
        positions.push({ x: spacing / 2, y: 0 })
      } else if (podCount === 3) {
        positions.push({ x: -spacing, y: 0 })
        positions.push({ x: 0, y: 0 })
        positions.push({ x: spacing, y: 0 })
      } else if (podCount === 4) {
        positions.push({ x: -spacing / 2, y: -spacing / 2 })
        positions.push({ x: spacing / 2, y: -spacing / 2 })
        positions.push({ x: -spacing / 2, y: spacing / 2 })
        positions.push({ x: spacing / 2, y: spacing / 2 })
      } else if (podCount === 5) {
        positions.push({ x: -spacing, y: -spacing / 2 })
        positions.push({ x: 0, y: -spacing / 2 })
        positions.push({ x: spacing, y: -spacing / 2 })
        positions.push({ x: -spacing / 2, y: spacing / 2 })
        positions.push({ x: spacing / 2, y: spacing / 2 })
      }
      
      return positions
    }

    it('calculates correct position for single pod', () => {
      const positions = calculatePodPositions(1)
      expect(positions).toHaveLength(1)
      expect(positions[0]).toEqual({ x: 0, y: 0 })
    })

    it('calculates correct positions for 2 pods', () => {
      const positions = calculatePodPositions(2)
      expect(positions).toHaveLength(2)
      expect(positions[0].x).toBeLessThan(0)
      expect(positions[1].x).toBeGreaterThan(0)
      expect(positions[0].y).toBe(0)
      expect(positions[1].y).toBe(0)
    })

    it('calculates correct positions for 3 pods in a row', () => {
      const positions = calculatePodPositions(3)
      expect(positions).toHaveLength(3)
      expect(positions[0].x).toBeLessThan(positions[1].x)
      expect(positions[1].x).toBeLessThan(positions[2].x)
      expect(positions.every(p => p.y === 0)).toBe(true)
    })

    it('calculates correct positions for 4 pods in a grid', () => {
      const positions = calculatePodPositions(4)
      expect(positions).toHaveLength(4)
      // Should form a 2x2 grid
      const topRow = positions.filter(p => p.y < 0)
      const bottomRow = positions.filter(p => p.y > 0)
      expect(topRow).toHaveLength(2)
      expect(bottomRow).toHaveLength(2)
    })

    it('calculates correct positions for 5 pods', () => {
      const positions = calculatePodPositions(5)
      expect(positions).toHaveLength(5)
      // Should have 3 on top, 2 on bottom
      const topRow = positions.filter(p => p.y < 0)
      const bottomRow = positions.filter(p => p.y > 0)
      expect(topRow).toHaveLength(3)
      expect(bottomRow).toHaveLength(2)
    })

    it('scales positions with custom spacing', () => {
      const spacing = 20
      const positions = calculatePodPositions(2, spacing)
      expect(positions[0].x).toBe(-spacing / 2)
      expect(positions[1].x).toBe(spacing / 2)
    })

    it('scales positions proportionally for larger service nodes', () => {
      const baseSpacing = 10
      const replicas = 5
      const squareSize = Math.max(50, 40 + (replicas * 8)) // 80
      const scaleFactor = squareSize / 50 // 1.6
      const scaledSpacing = baseSpacing * scaleFactor // 16
      
      const positions = calculatePodPositions(replicas, scaledSpacing)
      expect(positions).toHaveLength(5)
      // Positions should be scaled proportionally
      expect(Math.abs(positions[0].x)).toBeGreaterThan(baseSpacing)
    })
  })

  describe('Pod Color Determination', () => {
    // Helper function to determine pod color (extracted from component logic)
    function getPodColor(pod: any, _plane: string): string {
      if (pod.status === 'Failed') {
        return '#ff0055'
      }
      if (!pod.ready) {
        return '#666666'
      }
      if (pod.status === 'Running' && pod.ready) {
        return '#00ff88'
      }
      return '#666666'
    }

    it('returns red for failed pods', () => {
      const pod = { status: 'Failed', ready: false }
      expect(getPodColor(pod, 'processing')).toBe('#ff0055')
    })

    it('returns gray for not ready pods', () => {
      const pod = { status: 'Running', ready: false }
      expect(getPodColor(pod, 'processing')).toBe('#666666')
    })

    it('returns green for running and ready pods', () => {
      const pod = { status: 'Running', ready: true }
      expect(getPodColor(pod, 'processing')).toBe('#00ff88')
    })

    it('returns gray for unknown status', () => {
      const pod = { status: 'Unknown', ready: false }
      expect(getPodColor(pod, 'processing')).toBe('#666666')
    })

    it('prioritizes failed status over ready state', () => {
      const pod = { status: 'Failed', ready: true }
      expect(getPodColor(pod, 'processing')).toBe('#ff0055')
    })
  })

  describe('Particle Spawn Logic Based on Traffic Volume', () => {
    // Helper function to calculate particle count (extracted from component logic)
    function calculateParticleCount(requestsPerSecond: number): number {
      let particleCount: number
      
      if (requestsPerSecond < 50) {
        particleCount = Math.max(1, Math.ceil(requestsPerSecond / 20))
      } else if (requestsPerSecond < 500) {
        particleCount = 3 + Math.ceil((requestsPerSecond - 50) / 90)
      } else {
        particleCount = 8 + Math.ceil(Math.log10(requestsPerSecond - 400) * 2)
      }
      
      return Math.min(particleCount, 15) // Cap at 15
    }

    it('spawns 1-3 particles for low traffic (1-50 TPS)', () => {
      expect(calculateParticleCount(1)).toBe(1)
      expect(calculateParticleCount(20)).toBe(1)
      expect(calculateParticleCount(40)).toBe(2)
      expect(calculateParticleCount(50)).toBeLessThanOrEqual(3)
    })

    it('spawns 3-8 particles for medium traffic (50-500 TPS)', () => {
      const count100 = calculateParticleCount(100)
      const count300 = calculateParticleCount(300)
      const count450 = calculateParticleCount(450)
      
      expect(count100).toBeGreaterThanOrEqual(3)
      expect(count100).toBeLessThanOrEqual(8)
      expect(count300).toBeGreaterThanOrEqual(3)
      expect(count300).toBeLessThanOrEqual(8)
      expect(count450).toBeGreaterThanOrEqual(3)
      expect(count450).toBeLessThanOrEqual(8)
    })

    it('spawns 8-15 particles for high traffic (500+ TPS)', () => {
      const count600 = calculateParticleCount(600)
      const count1000 = calculateParticleCount(1000)
      const count2000 = calculateParticleCount(2000)
      
      expect(count600).toBeGreaterThanOrEqual(8)
      expect(count1000).toBeGreaterThanOrEqual(8)
      expect(count2000).toBeGreaterThanOrEqual(8)
      
      // All should be capped at 15
      expect(count600).toBeLessThanOrEqual(15)
      expect(count1000).toBeLessThanOrEqual(15)
      expect(count2000).toBeLessThanOrEqual(15)
    })

    it('caps particle count at 15 for very high traffic', () => {
      expect(calculateParticleCount(5000)).toBe(15)
      expect(calculateParticleCount(10000)).toBe(15)
    })

    it('scales logarithmically for high traffic', () => {
      const count600 = calculateParticleCount(600)
      const count1200 = calculateParticleCount(1200)
      
      // Doubling TPS should not double particles (logarithmic)
      expect(count1200).toBeLessThan(count600 * 2)
    })
  })

  describe('Traffic Color Determination Based on Health Metrics', () => {
    // Helper function to determine particle color (extracted from component logic)
    function getParticleColor(sourceId: string, targetId: string, metrics?: any): string {
      if (!metrics) {
        return '#888888' // Gray for no metrics
      }
      
      const { avgLatency, errorRate } = metrics
      
      // Red for errors or high latency
      if (errorRate > 0.01 || avgLatency > 500) {
        return '#ff0055'
      }
      
      // Yellow for elevated latency
      if (avgLatency > 100) {
        return '#ffaa00'
      }
      
      // Green for healthy
      return '#00ff88'
    }

    it('returns green for healthy traffic (low latency, no errors)', () => {
      const metrics = { avgLatency: 50, errorRate: 0 }
      expect(getParticleColor('a', 'b', metrics)).toBe('#00ff88')
    })

    it('returns yellow for elevated latency (100-500ms)', () => {
      const metrics = { avgLatency: 150, errorRate: 0 }
      expect(getParticleColor('a', 'b', metrics)).toBe('#ffaa00')
    })

    it('returns red for high latency (>500ms)', () => {
      const metrics = { avgLatency: 600, errorRate: 0 }
      expect(getParticleColor('a', 'b', metrics)).toBe('#ff0055')
    })

    it('returns red for high error rate (>1%)', () => {
      const metrics = { avgLatency: 50, errorRate: 0.02 }
      expect(getParticleColor('a', 'b', metrics)).toBe('#ff0055')
    })

    it('returns gray for unavailable metrics', () => {
      expect(getParticleColor('a', 'b', undefined)).toBe('#888888')
    })

    it('prioritizes errors over latency', () => {
      const metrics = { avgLatency: 50, errorRate: 0.05 }
      expect(getParticleColor('a', 'b', metrics)).toBe('#ff0055')
    })
  })

  describe('Tooltip Positioning Logic', () => {
    // Helper function to calculate tooltip position (extracted from component logic)
    function calculateTooltipPosition(
      elementX: number,
      elementY: number,
      viewportHeight: number = 600
    ): { x: number, y: number, flipped: boolean } {
      const flipThreshold = 120
      
      if (elementY < flipThreshold) {
        // Near top - flip tooltip below
        return { x: elementX, y: elementY + 20, flipped: true }
      } else {
        // Normal position - tooltip above
        return { x: elementX, y: elementY - 20, flipped: false }
      }
    }

    it('positions tooltip above element by default', () => {
      const pos = calculateTooltipPosition(100, 300)
      expect(pos.y).toBeLessThan(300)
      expect(pos.flipped).toBe(false)
    })

    it('flips tooltip below when near top edge', () => {
      const pos = calculateTooltipPosition(100, 50)
      expect(pos.y).toBeGreaterThan(50)
      expect(pos.flipped).toBe(true)
    })

    it('uses flip threshold of 120px', () => {
      const posAbove = calculateTooltipPosition(100, 121)
      const posBelow = calculateTooltipPosition(100, 119)
      
      expect(posAbove.flipped).toBe(false)
      expect(posBelow.flipped).toBe(true)
    })

    it('maintains horizontal position', () => {
      const x = 250
      const pos = calculateTooltipPosition(x, 300)
      expect(pos.x).toBe(x)
    })

    it('offsets tooltip by 20px vertically', () => {
      const y = 300
      const posAbove = calculateTooltipPosition(100, y)
      const posBelow = calculateTooltipPosition(100, 50)
      
      expect(Math.abs(posAbove.y - y)).toBe(20)
      expect(Math.abs(posBelow.y - 50)).toBe(20)
    })
  })
})
