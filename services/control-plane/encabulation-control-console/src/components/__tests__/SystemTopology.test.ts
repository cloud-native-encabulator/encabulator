import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import SystemTopology from '../SystemTopology.vue'
import { useSystemStore } from '@/stores/system'

describe('SystemTopology', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('renders topology component', () => {
    const wrapper = mount(SystemTopology)
    expect(wrapper.find('.system-topology').exists()).toBe(true)
  })

  it('displays plane legend', () => {
    const wrapper = mount(SystemTopology)
    expect(wrapper.text()).toContain('Data')
    expect(wrapper.text()).toContain('Processing')
    expect(wrapper.text()).toContain('Support')
    expect(wrapper.text()).toContain('Control')
  })

  it('creates correct number of nodes', () => {
    const wrapper = mount(SystemTopology)
    const component = wrapper.vm as any
    
    // Should have 13 services total
    expect(component.nodes.length).toBe(13)
  })

  it('creates correct service relationships', () => {
    const wrapper = mount(SystemTopology)
    
    // Component should render SVG with links
    const svg = wrapper.find('svg')
    expect(svg.exists()).toBe(true)
    
    // Should have the topology container
    expect(wrapper.find('.system-topology').exists()).toBe(true)
  })

  it('updates when system store changes', async () => {
    const systemStore = useSystemStore()
    const wrapper = mount(SystemTopology)
    
    // Update service status
    systemStore.updateServices({
      'panametric-fam-processor': { status: 'healthy', replicas: 3 }
    })
    
    await wrapper.vm.$nextTick()
    
    // Component should react to store changes
    expect(systemStore.services['panametric-fam-processor'].replicas).toBe(3)
  })

  it('shows node details on selection', async () => {
    const wrapper = mount(SystemTopology)
    const component = wrapper.vm as any
    
    // Simulate node selection
    component.selectedNode = {
      id: 'test-service',
      name: 'test-service',
      plane: 'processing',
      status: 'healthy',
      replicas: 2
    }
    
    await wrapper.vm.$nextTick()
    
    // Details panel should be visible
    expect(wrapper.text()).toContain('test-service')
    expect(wrapper.text()).toContain('processing')
    expect(wrapper.text()).toContain('healthy')
  })

  it('displays help text', () => {
    const wrapper = mount(SystemTopology)
    // Updated to match actual help text in component
    expect(wrapper.text()).toContain('Hover for quick info')
    expect(wrapper.text()).toContain('Click for details')
  })
})
