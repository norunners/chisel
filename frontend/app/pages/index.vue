<script setup lang="ts">
import { findModelOption } from '../composables/useChiselSessions'

const toast = useChiselToast()
const loading = ref(false)
const input = ref('')
const selectedAgent = useCookie<string>('chisel-agent', { default: () => 'Kiro CLI' })
const { isServerMode, supportsNativeWorkspacePicker } = useRuntimeMode()
const newThreadToken = useState<number>('new-thread-token', () => 0)

const { agents, discoveringModes, refreshSettings, discoverAgentModes } = useChiselSettings()
const {
  recentWorkspaces,
  knownModesByAgent,
  preferredModesByAgent,
  preferredModelsByAgent,
  refreshSessions,
  createSession,
  sendPrompt,
  setConfigOption,
  setMode,
  pickWorkspace,
  selectedWorkspace,
  setSelectedWorkspace,
  setPreferredModeForAgent,
  setPreferredModelForAgent
} = useChiselSessions()

const agentOptions = computed(() => agents.value.map(agent => ({
  label: agent.name,
  value: agent.name,
  description: `${agent.command} ${agent.args.join(' ')}`
})))

function encodeProviderValue(agentName: string) {
  return `provider:${encodeURIComponent(agentName)}`
}

function encodeModeValue(agentName: string, modeId: string) {
  return `mode:${encodeURIComponent(agentName)}:${encodeURIComponent(modeId)}`
}

function parseProviderSelection(value: string) {
  if (value.startsWith('provider:')) {
    return {
      kind: 'provider' as const,
      agentName: decodeURIComponent(value.slice('provider:'.length))
    }
  }

  if (value.startsWith('mode:')) {
    const segments = value.split(':')
    return {
      kind: 'mode' as const,
      agentName: decodeURIComponent(segments[1] || ''),
      modeId: decodeURIComponent(segments.slice(2).join(':'))
    }
  }

  return null
}

const agentDescriptorsByName = computed(() => new Map(agents.value.map(agent => [agent.name, agent])))
const selectedAgentDescriptor = computed(() => agentDescriptorsByName.value.get(selectedAgent.value) || null)
const selectedModelOption = computed(() => selectedAgentDescriptor.value?.modelOption || null)

function cachedModesForAgent(agentName: string) {
  const liveModes = knownModesByAgent.value[agentName] || []
  if (liveModes.length > 0) {
    return liveModes
  }

  return agentDescriptorsByName.value.get(agentName)?.modes || []
}

function isAgentDiscovering(agentName: string) {
  return Boolean(discoveringModes.value[agentName])
}

async function ensureAgentModes(agentName: string, interactive = false) {
  if (!agentName || cachedModesForAgent(agentName).length > 0 || isAgentDiscovering(agentName)) {
    return
  }

  try {
    const descriptor = await discoverAgentModes(agentName, selectedWorkspace.value || latestWorkspacePath.value || '')
    const defaultModeId = descriptor?.currentModeId || descriptor?.modes?.[0]?.id || ''
    if (defaultModeId && !preferredModesByAgent.value[agentName]) {
      setPreferredModeForAgent(agentName, defaultModeId)
    }
    const defaultModelId = descriptor?.modelOption?.currentValue || ''
    if (defaultModelId && !preferredModelsByAgent.value[agentName]) {
      setPreferredModelForAgent(agentName, defaultModelId)
    }
  } catch (error) {
    if (!interactive) {
      return
    }

    toast.add({
      title: 'Unable to load agent modes',
      description: error instanceof Error ? error.message : 'The provider could not be queried for its configured agents.',
      color: 'error'
    })
  }
}

function syncProviderDefaults(agentName: string, descriptor: typeof selectedAgentDescriptor.value) {
  const defaultModeId = descriptor?.currentModeId || descriptor?.modes?.[0]?.id || ''
  const defaultModelId = descriptor?.modelOption?.currentValue || ''

  setPreferredModeForAgent(agentName, defaultModeId)
  setPreferredModelForAgent(agentName, defaultModelId)
}

async function refreshProviderDefaults(agentName: string, interactive = false) {
  try {
    const descriptor = await discoverAgentModes(agentName, selectedWorkspace.value || latestWorkspacePath.value || '', true)
    syncProviderDefaults(agentName, descriptor)
    return descriptor
  } catch (error) {
    if (!interactive) {
      return null
    }

    toast.add({
      title: 'Unable to refresh provider',
      description: error instanceof Error ? error.message : 'The provider could not refresh its agent and model defaults.',
      color: 'error'
    })
    return null
  }
}

function defaultModeIDForAgent(agentName: string) {
  const descriptor = agentDescriptorsByName.value.get(agentName)
  return descriptor?.currentModeId || cachedModesForAgent(agentName)[0]?.id || ''
}

const providerItems = computed(() => {
  const items: Array<{ label: string, value: string, disabled?: boolean }> = []

  for (const agent of agentOptions.value) {
    items.push({
      label: agent.label,
      value: encodeProviderValue(agent.value)
    })

    const modes = cachedModesForAgent(agent.value)

    if (modes.length === 0 && isAgentDiscovering(agent.value)) {
      items.push({
        label: '\u00A0\u00A0Loading agents…',
        value: `loading:${encodeURIComponent(agent.value)}`,
        disabled: true
      })
    }

    for (const mode of modes) {
      items.push({
        label: `\u00A0\u00A0${mode.name}`,
        value: encodeModeValue(agent.value, mode.id)
      })
    }
  }

  return items
})

const providerValue = computed(() => {
  const preferredModeId = preferredModesByAgent.value[selectedAgent.value] || agentDescriptorsByName.value.get(selectedAgent.value)?.currentModeId
  const modes = cachedModesForAgent(selectedAgent.value)
  const defaultModeId = defaultModeIDForAgent(selectedAgent.value)

  if (
    preferredModeId
    && preferredModeId !== defaultModeId
    && modes.some(mode => mode.id === preferredModeId)
  ) {
    return encodeModeValue(selectedAgent.value, preferredModeId)
  }

  return encodeProviderValue(selectedAgent.value)
})

const modelItems = computed(() => {
  if (!selectedModelOption.value) {
    return []
  }

  return selectedModelOption.value.values.map(value => ({
    label: value.groupName ? `${value.groupName} / ${value.name}` : value.name,
    value: value.value
  }))
})

const modelValue = computed(() => {
  const preferredModelId = preferredModelsByAgent.value[selectedAgent.value]
  if (preferredModelId && selectedModelOption.value?.values.some(value => value.value === preferredModelId)) {
    return preferredModelId
  }

  return selectedModelOption.value?.currentValue || ''
})

function workspaceName(path: string) {
  const trimmed = path.replace(/[\\/]+$/, '')
  const segments = trimmed.split(/[\\/]/).filter(Boolean)
  return segments.at(-1) || path || 'New project'
}

const recentWorkspaceItems = computed(() => {
  const seen = new Set<string>()
  const items = []

  if (selectedWorkspace.value && !seen.has(selectedWorkspace.value)) {
    seen.add(selectedWorkspace.value)
    items.push({
      label: workspaceName(selectedWorkspace.value),
      path: selectedWorkspace.value
    })
  }

  const sortedRecent = [...recentWorkspaces.value].sort((left, right) => {
    return new Date(right.lastOpenedAt || 0).getTime() - new Date(left.lastOpenedAt || 0).getTime()
  })

  for (const workspace of sortedRecent) {
    if (seen.has(workspace.path)) {
      continue
    }

    seen.add(workspace.path)
    items.push({
      label: workspaceName(workspace.path),
      path: workspace.path
    })
  }

  return items
})

const latestWorkspacePath = computed(() => {
  const sortedRecent = [...recentWorkspaces.value].sort((left, right) => {
    return new Date(right.lastOpenedAt || 0).getTime() - new Date(left.lastOpenedAt || 0).getTime()
  })

  return sortedRecent[0]?.path || ''
})

function resetNewThreadState() {
  input.value = ''

  if (latestWorkspacePath.value) {
    setSelectedWorkspace(latestWorkspacePath.value)
  }
}

onMounted(async () => {
  await Promise.all([
    refreshSettings(),
    refreshSessions()
  ])

  if (!agentOptions.value.find(option => option.value === selectedAgent.value)) {
    selectedAgent.value = agentOptions.value[0]?.value || 'Kiro CLI'
  }

  resetNewThreadState()
  void ensureAgentModes(selectedAgent.value)
})

async function browseWorkspace() {
  if (!supportsNativeWorkspacePicker.value) {
    toast.add({
      title: 'Workspace picker unavailable',
      description: 'Browser mode cannot open native folder pickers. Enter an absolute workspace path directly.',
      color: 'warning'
    })
    return
  }

  const selection = await pickWorkspace(selectedWorkspace.value)
  if (selection) {
    setSelectedWorkspace(selection)
  }
}

async function handleAddProject() {
  await browseWorkspace()
}

function handleProjectSelection(path: string) {
  setSelectedWorkspace(path)
}

function handleProjectPathSelection(path: string) {
  const nextPath = path.trim()
  if (!nextPath) {
    return
  }

  if (!nextPath.startsWith('/')) {
    toast.add({
      title: 'Absolute path required',
      description: 'Enter a full workspace path such as /Users/you/project.',
      color: 'error'
    })
    return
  }

  setSelectedWorkspace(nextPath)
}

async function createNewSession() {
  if (!selectedAgent.value) {
    toast.add({
      title: 'No agent selected',
      description: 'Choose a configured ACP agent before starting a session.',
      color: 'error'
    })
    return
  }

  if (!selectedWorkspace.value) {
    if (supportsNativeWorkspacePicker.value) {
      await browseWorkspace()
    } else {
      toast.add({
        title: 'Workspace path required',
        description: 'Enter an absolute workspace path before starting a browser session.',
        color: 'error'
      })
    }
  }

  if (!selectedWorkspace.value) {
    return
  }

  loading.value = true
  try {
    let result = await createSession({
      agentName: selectedAgent.value,
      cwd: selectedWorkspace.value,
      titleHint: input.value
    })

    const preferredModeId = preferredModesByAgent.value[selectedAgent.value]
    if (preferredModeId && result.modes.some(mode => mode.id === preferredModeId) && result.currentModeId !== preferredModeId) {
      try {
        result = await setMode(result.session.sessionId, preferredModeId)
      } catch (error) {
        toast.add({
          title: 'Unable to set agent mode',
          description: error instanceof Error ? error.message : 'The session started with the provider default mode instead.',
          color: 'warning'
        })
      }
    }

    const preferredModelId = preferredModelsByAgent.value[selectedAgent.value]
    const resultModelOption = findModelOption(result.configOptions)
    if (preferredModelId && resultModelOption?.values.some(value => value.value === preferredModelId) && resultModelOption.currentValue !== preferredModelId) {
      try {
        result = await setConfigOption(result.session.sessionId, resultModelOption.id, preferredModelId)
      } catch (error) {
        toast.add({
          title: 'Unable to set model',
          description: error instanceof Error ? error.message : 'The session started with the provider default model instead.',
          color: 'warning'
        })
      }
    }

    await navigateTo(`/chat/${result.session.sessionId}`)
    if (input.value.trim()) {
      const firstPrompt = input.value.trim()
      input.value = ''
      await sendPrompt(result.session.sessionId, firstPrompt)
    }
  } catch (error) {
    toast.add({
      title: 'Unable to start session',
      description: error instanceof Error ? error.message : 'An unexpected error occurred.',
      color: 'error'
    })
  } finally {
    loading.value = false
  }
}

async function onSubmit() {
  await createNewSession()
}

async function handleProviderSelection(value: string) {
  const selection = parseProviderSelection(value)
  if (!selection) {
    return
  }

  selectedAgent.value = selection.agentName

  if (selection.kind === 'provider') {
    await refreshProviderDefaults(selection.agentName, true)
    return
  }

  setPreferredModeForAgent(selection.agentName, selection.modeId)
}

function handleModelSelection(value: string) {
  if (!selectedAgent.value) {
    return
  }

  setPreferredModelForAgent(selectedAgent.value, value)
}

watch(() => newThreadToken.value, () => {
  resetNewThreadState()
})

watch([selectedAgent, selectedWorkspace, latestWorkspacePath], ([agentName]) => {
  void ensureAgentModes(agentName)
})
</script>

<template>
  <UDashboardPanel
    id="home"
    class="min-h-0"
    :ui="{ body: 'p-0 sm:p-0' }"
  >
    <template #header>
      <DashboardNavbar />
    </template>

    <template #body>
      <UContainer class="flex h-full flex-col py-6 sm:py-8">
        <p class="text-xs font-medium text-muted">
          New session
        </p>

        <div class="flex flex-1 flex-col items-center justify-center gap-4 text-center">
          <div class="flex size-12 items-center justify-center rounded-full border border-default bg-default/70 shadow-sm">
            <UIcon name="i-lucide-pickaxe" class="size-5 text-highlighted" />
          </div>

          <div class="space-y-1">
            <h1 class="text-3xl font-semibold text-highlighted sm:text-4xl">
              Let's build
            </h1>

            <ProjectSwitcherMenu
              :selected-path="selectedWorkspace"
              :items="recentWorkspaceItems"
              :supports-native-workspace-picker="supportsNativeWorkspacePicker"
              @select="handleProjectSelection"
              @add-project="handleAddProject"
              @add-project-by-path="handleProjectPathSelection"
            />
          </div>

          <p v-if="isServerMode" class="max-w-sm text-sm text-muted">
            Browser mode can switch between recent projects, and new projects can be added with an absolute path from the selector.
          </p>
        </div>

        <div class="mx-auto w-full max-w-4xl pt-6">
          <UChatPrompt
            v-model="input"
            :status="loading ? 'streaming' : 'ready'"
            variant="subtle"
            class="[view-transition-name:chat-prompt]"
            :ui="{ base: 'px-1.5' }"
            @submit="onSubmit"
          >
            <template #footer>
              <div class="flex items-center gap-1 overflow-x-auto">
                <FileUploadButton />
                <ComposerSelect
                  aria-label="Select agent"
                  :items="providerItems"
                  :selected="providerValue"
                  @update:selected="handleProviderSelection"
                />
                <ComposerSelect
                  aria-label="Select model"
                  :items="modelItems"
                  :selected="modelValue"
                  :disabled="modelItems.length === 0"
                  empty-label="Default"
                  @update:selected="handleModelSelection"
                />
              </div>

              <UTooltip :content="{ side: 'top' }" text="Send">
                <UChatPromptSubmit color="neutral" size="sm" :status="loading ? 'streaming' : 'ready'" />
              </UTooltip>
            </template>
          </UChatPrompt>
        </div>
      </UContainer>
    </template>
  </UDashboardPanel>
</template>
