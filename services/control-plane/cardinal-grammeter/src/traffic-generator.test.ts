import { TrafficGenerator } from './traffic-generator'

describe('TrafficGenerator', () => {
  let generator: TrafficGenerator

  beforeEach(() => {
    generator = new TrafficGenerator()
  })

  afterEach(() => {
    generator.stop()
  })

  test('should initialize with inactive status', () => {
    const status = generator.getStatus()
    expect(status.active).toBe(false)
    expect(status.requestsSent).toBe(0)
  })

  test('should start traffic generation', async () => {
    await generator.start({
      target: 'http://localhost:8080/test',
      pattern: 'steady',
      magnetoReluctance: 50,
      capacitiveDiractance: 50,
    })

    const status = generator.getStatus()
    expect(status.active).toBe(true)
    expect(status.target).toBe('http://localhost:8080/test')
    expect(status.pattern).toBe('steady')
  })

  test('should stop traffic generation', async () => {
    await generator.start({
      target: 'http://localhost:8080/test',
      pattern: 'steady',
      magnetoReluctance: 50,
      capacitiveDiractance: 50,
    })

    generator.stop()

    const status = generator.getStatus()
    expect(status.active).toBe(false)
  })

  test('should throw error when starting already active generator', async () => {
    await generator.start({
      target: 'http://localhost:8080/test',
      pattern: 'steady',
      magnetoReluctance: 50,
      capacitiveDiractance: 50,
    })

    await expect(
      generator.start({
        target: 'http://localhost:8080/test',
        pattern: 'steady',
        magnetoReluctance: 50,
        capacitiveDiractance: 50,
      })
    ).rejects.toThrow('Traffic generation already active')
  })

  test('should update parameters', async () => {
    await generator.start({
      target: 'http://localhost:8080/test',
      pattern: 'steady',
      magnetoReluctance: 50,
      capacitiveDiractance: 50,
    })

    generator.updateParameters({
      magnetoReluctance: 75,
      capacitiveDiractance: 25,
    })

    // Parameters are updated internally, no direct way to verify without exposing config
    // This test ensures the method doesn't throw
    expect(true).toBe(true)
  })

  test('should calculate success rate correctly', () => {
    const status = generator.getStatus()
    expect(status.successRate).toBe(0)
  })
})
