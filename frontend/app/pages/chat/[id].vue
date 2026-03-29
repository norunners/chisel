<script setup lang="ts">
import type { DefineComponent } from 'vue'
import { findModelOption } from '../../composables/useChiselSessions'
import type { TimelineMessageItem } from '../../composables/useChiselSessions'
import { useClipboard } from '@vueuse/core'
import ProseStreamPre from '../../components/prose/PreStream.vue'

const components = {
  pre: ProseStreamPre as unknown as DefineComponent
}

const route = useRoute()
const toast = useChiselToast()
const clipboard = useClipboard()
const selectedAgent = useCookie<string>('chisel-agent', { default: () => 'Kiro CLI' })
const sessionId = computed(() => route.params.id as string)

const { agents, discoveringModes, refreshSettings, discoverAgentModes } = useChiselSettings()
const {
  active,
  knownModesByAgent,
  preferredModesByAgent,
  preferredModelsByAgent,
  selectedWorkspace,
  refreshSessions,
  refreshPermissions,
  loadSession,
  sendPrompt,
  cancelPrompt,
  setConfigOption,
  setMode,
  resolvePermission,
  setSelectedWorkspace,
  setPreferredModeForAgent,
  setPreferredModelForAgent,
  timelineFor,
  statusFor
} = useChiselSessions()

const input = ref('')
const copiedMessageId = ref('')
const loadingSession = ref(true)

const timeline = computed(() => timelineFor(sessionId.value))
const sessionStatus = computed(() => statusFor(sessionId.value))
const currentSession = computed(() => active.value?.session.sessionId === sessionId.value ? active.value : null)
const chatStatus = computed(() => sessionStatus.value === 'streaming' ? 'streaming' : 'ready')

const scrollMessages = computed(() => timeline.value
  .map(item => ({
    id: item.id,
    role: item.kind === 'message' ? item.role : 'assistant',
    parts: [{
      type: 'text' as const,
      text: item.timestamp
    }]
  })))

const agentOptions = computed(() => agents.value.map(agent => ({
  label: agent.name,
  value: agent.name,
  description: `${agent.command} ${agent.args.join(' ')}`
})))

const agentDescriptorsByName = computed(() => new Map(agents.value.map(agent => [agent.name, agent])))

const modelOption = computed(() => {
  return findModelOption(currentSession.value?.configOptions)
})

const modelItems = computed(() => {
  if (!modelOption.value) {
    return []
  }

  return modelOption.value.values.map(value => ({
    label: value.groupName ? `${value.groupName} / ${value.name}` : value.name,
    value: value.value
  }))
})

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

function modesForAgent(agentName: string) {
  if (agentName === currentSession.value?.session.agentName) {
    return currentSession.value?.modes || []
  }

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
  if (!agentName || modesForAgent(agentName).length > 0 || isAgentDiscovering(agentName)) {
    return
  }

  try {
    const descriptor = await discoverAgentModes(agentName, currentSession.value?.session.cwd || selectedWorkspace.value || '')
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

const providerAgentItems = computed(() => {
  const items: Array<{ label: string, value: string, disabled?: boolean }> = []

  for (const agent of agentOptions.value) {
    items.push({
      label: agent.label,
      value: encodeProviderValue(agent.value)
    })

    const modes = modesForAgent(agent.value)

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

function defaultModeIDForAgent(agentName: string) {
  const descriptor = agentDescriptorsByName.value.get(agentName)
  return descriptor?.currentModeId || modesForAgent(agentName)[0]?.id || ''
}

const providerAgentValue = computed(() => {
  if (currentSession.value?.session.agentName) {
    const agentName = currentSession.value.session.agentName
    const sessionModeID = currentSession.value.currentModeId || ''
    const defaultModeId = defaultModeIDForAgent(agentName)

    if (sessionModeID && sessionModeID !== defaultModeId) {
      return encodeModeValue(agentName, sessionModeID)
    }

    if (agentName) {
      return encodeProviderValue(agentName)
    }
  }

  if (selectedAgent.value) {
    const defaultModeId = defaultModeIDForAgent(selectedAgent.value)
    const preferredModeId = preferredModesByAgent.value[selectedAgent.value]
    if (
      preferredModeId
      && preferredModeId !== defaultModeId
      && modesForAgent(selectedAgent.value).some(mode => mode.id === preferredModeId)
    ) {
      return encodeModeValue(selectedAgent.value, preferredModeId)
    }
  }

  return encodeProviderValue(selectedAgent.value)
})

function syncProviderDefaults(agentName: string, descriptor: Awaited<ReturnType<typeof discoverAgentModes>>) {
  const defaultModeId = descriptor?.currentModeId || descriptor?.modes?.[0]?.id || ''
  const defaultModelId = descriptor?.modelOption?.currentValue || ''

  setPreferredModeForAgent(agentName, defaultModeId)
  setPreferredModelForAgent(agentName, defaultModelId)

  return {
    defaultModeId,
    defaultModelId
  }
}

async function refreshProviderDefaults(agentName: string, interactive = false) {
  try {
    const descriptor = await discoverAgentModes(agentName, currentSession.value?.session.cwd || selectedWorkspace.value || '', true)
    return {
      descriptor,
      ...syncProviderDefaults(agentName, descriptor)
    }
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

async function openSession() {
  loadingSession.value = true
  try {
    await Promise.all([
      loadSession(sessionId.value),
      refreshSessions(),
      refreshPermissions(),
      refreshSettings()
    ])
  } catch (error) {
    throw createError({
      statusCode: 500,
      statusMessage: error instanceof Error ? error.message : 'Unable to load session.'
    })
  } finally {
    loadingSession.value = false
  }
}

watch(sessionId, () => {
  openSession()
}, { immediate: true })

async function copyMessage(message: TimelineMessageItem) {
  await clipboard.copy(message.text)
  copiedMessageId.value = message.id
  window.setTimeout(() => {
    copiedMessageId.value = ''
  }, 2000)
}

function messageActions(message: TimelineMessageItem) {
  if (message.role !== 'assistant' || chatStatus.value === 'streaming') {
    return []
  }

  return [{
    label: 'Copy',
    icon: copiedMessageId.value === message.id ? 'i-lucide-copy-check' : 'i-lucide-copy',
    onClick: () => copyMessage(message)
  }]
}

async function handleSubmit() {
  const prompt = input.value.trim()
  if (!prompt) {
    return
  }

  input.value = ''

  try {
    await sendPrompt(sessionId.value, prompt)
  } catch (error) {
    toast.add({
      title: 'Unable to send prompt',
      description: error instanceof Error ? error.message : 'An unexpected ACP error occurred.',
      color: 'error'
    })
  }
}

async function handleCancel() {
  try {
    await cancelPrompt(sessionId.value)
  } catch (error) {
    toast.add({
      title: 'Unable to cancel turn',
      description: error instanceof Error ? error.message : 'An unexpected ACP error occurred.',
      color: 'error'
    })
  }
}

async function updateConfig(optionId: string, value: string) {
  try {
    await setConfigOption(sessionId.value, optionId, value)
  } catch (error) {
    toast.add({
      title: 'Unable to update option',
      description: error instanceof Error ? error.message : 'The agent rejected the config update.',
      color: 'error'
    })
  }
}

async function updateModel(value: string) {
  if (!modelOption.value || value === modelOption.value.currentValue) {
    return
  }

  await updateConfig(modelOption.value.id, value)
  if (currentSession.value?.session.agentName) {
    setPreferredModelForAgent(currentSession.value.session.agentName, value)
  }
}

async function updateMode(modeId: string) {
  if (!currentSession.value || modeId === currentSession.value.currentModeId) {
    return
  }

  try {
    await setMode(sessionId.value, modeId)
    setPreferredModeForAgent(currentSession.value.session.agentName, modeId)
  } catch (error) {
    toast.add({
      title: 'Unable to switch mode',
      description: error instanceof Error ? error.message : 'The agent rejected the mode change.',
      color: 'error'
    })
  }
}

async function handleSessionMenuAgent(agentName: string) {
  if (!currentSession.value || agentName === currentSession.value.session.agentName) {
    return
  }

  selectedAgent.value = agentName
  setSelectedWorkspace(currentSession.value.session.cwd || selectedWorkspace.value)

  toast.add({
    title: 'Ready for a new session',
    description: `${agentName} is selected for the next session in ${currentSession.value.session.cwd}.`,
    icon: 'i-lucide-bot'
  })

  await navigateTo('/')
}

async function handleProviderAgentSelection(value: string) {
  const selection = parseProviderSelection(value)
  if (!selection) {
    return
  }

  if (selection.kind === 'mode') {
    if (selection.agentName === currentSession.value?.session.agentName) {
      await updateMode(selection.modeId)
      return
    }

    setPreferredModeForAgent(selection.agentName, selection.modeId)
    await handleSessionMenuAgent(selection.agentName)
    return
  }

  const refreshed = await refreshProviderDefaults(selection.agentName, true)
  if (!refreshed) {
    return
  }

  if (selection.agentName === currentSession.value?.session.agentName) {
    if (refreshed.defaultModeId && refreshed.defaultModeId !== currentSession.value.currentModeId) {
      await updateMode(refreshed.defaultModeId)
    }

    const currentModel = findModelOption(currentSession.value.configOptions)
    if (
      refreshed.defaultModelId
      && currentModel
      && currentModel.currentValue !== refreshed.defaultModelId
      && currentModel.values.some(value => value.value === refreshed.defaultModelId)
    ) {
      await updateConfig(currentModel.id, refreshed.defaultModelId)
      setPreferredModelForAgent(selection.agentName, refreshed.defaultModelId)
    }

    toast.add({
      title: 'Provider defaults refreshed',
      description: `${selection.agentName} has been refreshed and reset to its default agent and model settings.`,
      color: 'neutral'
    })
    return
  }

  selectedAgent.value = selection.agentName
  setSelectedWorkspace(currentSession.value?.session.cwd || selectedWorkspace.value)
  await handleSessionMenuAgent(selection.agentName)
}

watch(() => currentSession.value?.session.agentName, (agentName) => {
  if (!agentName) {
    return
  }

  void ensureAgentModes(agentName)
}, { immediate: true })

async function choosePermission(requestId: string, optionId: string) {
  try {
    await resolvePermission(requestId, optionId, false)
  } catch (error) {
    toast.add({
      title: 'Unable to resolve permission',
      description: error instanceof Error ? error.message : 'The permission response could not be sent.',
      color: 'error'
    })
  }
}

async function cancelPermission(requestId: string) {
  try {
    await resolvePermission(requestId, '', true)
  } catch (error) {
    toast.add({
      title: 'Unable to cancel permission',
      description: error instanceof Error ? error.message : 'The permission response could not be sent.',
      color: 'error'
    })
  }
}
</script>

<template>
  <UDashboardPanel
    id="chat"
    class="relative min-h-0"
    :ui="{ body: 'p-0 sm:p-0 overscroll-none' }"
  >
    <template #header>
      <DashboardNavbar />
    </template>

    <template #body>
      <div v-if="loadingSession" class="flex flex-1 items-center justify-center">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-muted" />
      </div>

      <UContainer v-else class="flex h-full flex-col py-6 sm:py-8">
        <div class="mx-auto flex h-full w-full max-w-4xl flex-col">
          <div class="space-y-1">
            <p class="text-xs font-medium text-muted">
              {{ currentSession?.session.title || 'Session' }}
            </p>
            <p v-if="currentSession?.session.cwd" class="text-xs text-muted break-all">
              {{ currentSession.session.cwd }}
            </p>
          </div>

          <UChatMessages
            should-auto-scroll
            :messages="scrollMessages as any"
            :status="chatStatus"
            :spacing-offset="0"
            class="flex-1 py-6"
          >
            <template #default>
              <template v-if="timeline.length > 0">
                <template v-for="item in timeline" :key="item.id">
                  <UChatMessage
                    v-if="item.kind === 'message'"
                    :id="item.id"
                    :role="item.role"
                    :parts="[{ type: 'text', text: item.text }] as any"
                    :side="item.role === 'user' ? 'right' : 'left'"
                    :variant="item.role === 'user' ? 'soft' : 'naked'"
                    :actions="messageActions(item)"
                  >
                    <template #content>
                      <MDCCached
                        v-if="item.role === 'assistant'"
                        :value="item.text"
                        :cache-key="item.id"
                        :components="components"
                        :parser-options="{ highlight: false }"
                        class="*:first:mt-0 *:last:mb-0"
                      />
                      <p v-else class="whitespace-pre-wrap">
                        {{ item.text }}
                      </p>
                    </template>
                  </UChatMessage>

                  <div v-else-if="item.kind === 'thought'" class="max-w-3xl">
                    <Reasoning :text="item.text" :is-streaming="item.isStreaming" />
                  </div>

                  <ChatTimelinePlan
                    v-else-if="item.kind === 'plan'"
                    :entries="item.entries"
                  />

                  <ChatTimelineToolCall
                    v-else-if="item.kind === 'tool'"
                    :tool-call="item.toolCall"
                  />

                  <ChatTimelinePermission
                    v-else-if="item.kind === 'permission'"
                    :request="item.request"
                    :status="item.status"
                    @choose="choosePermission"
                    @cancel="cancelPermission"
                  />

                  <div
                    v-else-if="item.kind === 'error'"
                    class="max-w-3xl rounded-2xl border border-error/40 bg-error/10 p-4 text-sm text-error shadow-sm"
                  >
                    {{ item.text }}
                  </div>
                </template>
              </template>

              <div
                v-else
                class="flex h-full min-h-0 items-center justify-center py-12"
              >
                <div class="max-w-sm space-y-3 text-center">
                  <div class="mx-auto flex size-10 items-center justify-center rounded-full border border-default bg-default/70 shadow-sm">
                    <UIcon name="i-lucide-pickaxe" class="size-4 text-highlighted" />
                  </div>
                  <p class="text-sm text-muted">
                    This session is ready. Ask about the current project and Chisel will stream plans, tools, and approvals directly in the conversation.
                  </p>
                </div>
              </div>
            </template>
          </UChatMessages>

          <div class="pt-4">
            <UChatPrompt
              v-model="input"
              variant="subtle"
              class="[view-transition-name:chat-prompt]"
              :ui="{ base: 'px-1.5' }"
              @submit="handleSubmit"
            >
              <template #footer>
                <div class="flex min-w-0 items-center gap-1 overflow-x-auto">
                  <FileUploadButton />
                  <ComposerSelect
                    aria-label="Select agent"
                    :items="providerAgentItems"
                    :selected="providerAgentValue"
                    @update:selected="handleProviderAgentSelection"
                  />
                  <ComposerSelect
                    aria-label="Select model"
                    :items="modelItems"
                    :selected="modelOption?.currentValue || ''"
                    :disabled="modelItems.length === 0"
                    empty-label="Default"
                    @update:selected="updateModel"
                  />
                </div>

                <div class="flex items-center gap-2">
                  <UButton
                    v-if="sessionStatus === 'streaming'"
                    icon="i-lucide-square"
                    color="neutral"
                    size="sm"
                    variant="outline"
                    @click="handleCancel"
                  />

                  <UTooltip :content="{ side: 'top' }" text="Send">
                    <UChatPromptSubmit
                      color="neutral"
                      size="sm"
                      :status="chatStatus"
                    />
                  </UTooltip>
                </div>
              </template>
            </UChatPrompt>
          </div>
        </div>
      </UContainer>
    </template>
  </UDashboardPanel>
</template>
