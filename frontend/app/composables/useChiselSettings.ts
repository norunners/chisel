import { GetSettings, ListAgents, SaveSettings } from '../../bindings/github.com/norunners/chisel/settingsservice'
import { DiscoverAgentModes, TestAgent } from '../../bindings/github.com/norunners/chisel/acpservice'
import * as models from '../../bindings/github.com/norunners/chisel/models'

export function useChiselSettings() {
  const settings = useState<models.ChiselSettings>('chisel-settings', () => new models.ChiselSettings())
  const agents = useState<models.AgentDescriptor[]>('chisel-agents', () => [])
  const loading = useState<boolean>('chisel-settings-loading', () => false)
  const testResults = useState<Record<string, models.AgentTestResult>>('chisel-agent-tests', () => ({}))
  const discoveringModes = useState<Record<string, boolean>>('chisel-agent-mode-discovery', () => ({}))

  function upsertAgent(descriptor: models.AgentDescriptor) {
    const existingIndex = agents.value.findIndex(agent => agent.name === descriptor.name)
    if (existingIndex >= 0) {
      agents.value = agents.value.map((agent, index) => index === existingIndex ? descriptor : agent)
      return
    }

    agents.value = [...agents.value, descriptor].sort((left, right) => left.name.localeCompare(right.name))
  }

  async function refreshSettings() {
    loading.value = true
    try {
      settings.value = await GetSettings()
      agents.value = await ListAgents()
    } finally {
      loading.value = false
    }
  }

  async function saveSettings(nextSettings: models.ChiselSettings) {
    await SaveSettings(nextSettings)
    settings.value = nextSettings
    agents.value = await ListAgents()
  }

  async function testAgent(agentName: string) {
    const result = await TestAgent(agentName)
    testResults.value = {
      ...testResults.value,
      [agentName]: result
    }
    return result
  }

  async function discoverAgentModes(agentName: string, cwd = '', force = false) {
    const existing = agents.value.find(agent => agent.name === agentName)
    if (!force && (existing?.modes?.length || existing?.modelOption?.values?.length)) {
      return existing
    }

    if (discoveringModes.value[agentName]) {
      return existing || null
    }

    discoveringModes.value = {
      ...discoveringModes.value,
      [agentName]: true
    }

    try {
      const descriptor = await DiscoverAgentModes(agentName, cwd, force)
      upsertAgent(descriptor)
      return descriptor
    } finally {
      const { [agentName]: _discovering, ...remaining } = discoveringModes.value
      discoveringModes.value = remaining
    }
  }

  return {
    settings,
    agents,
    loading,
    testResults,
    discoveringModes,
    refreshSettings,
    saveSettings,
    testAgent,
    discoverAgentModes
  }
}
