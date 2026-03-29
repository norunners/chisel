import { Events } from '@wailsio/runtime'
import type { Ref } from 'vue'
import { ListPendingPermissions, ResolvePermission } from '../../bindings/github.com/norunners/chisel/permissionservice'
import {
  CancelPrompt,
  CreateSession,
  ForgetSession,
  ListRecentWorkspaces,
  ListSessions,
  LoadSession,
  PickWorkspace,
  SendPrompt,
  SetConfigOption,
  SetMode
} from '../../bindings/github.com/norunners/chisel/sessionservice'
import * as models from '../../bindings/github.com/norunners/chisel/models'

export type SessionStatus = 'idle' | 'loading' | 'streaming' | 'error'
export type PermissionTimelineStatus = 'pending' | 'approved' | 'cancelled' | 'resolved'

interface TimelineBase {
  id: string
  kind: 'message' | 'thought' | 'tool' | 'plan' | 'permission' | 'error'
  timestamp: string
}

export interface TimelineMessageItem extends TimelineBase {
  kind: 'message'
  role: 'user' | 'assistant'
  text: string
}

export interface TimelineThoughtItem extends TimelineBase {
  kind: 'thought'
  text: string
  isStreaming: boolean
}

export interface TimelineToolItem extends TimelineBase {
  kind: 'tool'
  toolCallId: string
  toolCall: models.ToolCallRecord
}

export interface TimelinePlanItem extends TimelineBase {
  kind: 'plan'
  entries: models.PlanEntry[]
}

export interface TimelinePermissionItem extends TimelineBase {
  kind: 'permission'
  requestId: string
  request: models.PermissionRequest
  status: PermissionTimelineStatus
}

export interface TimelineErrorItem extends TimelineBase {
  kind: 'error'
  text: string
}

export type SessionTimelineItem = TimelineMessageItem
  | TimelineThoughtItem
  | TimelineToolItem
  | TimelinePlanItem
  | TimelinePermissionItem
  | TimelineErrorItem

let subscribed = false

function timelineID(prefix: string) {
  return `${prefix}-${crypto.randomUUID()}`
}

export function findModelOption(configOptions?: models.SessionConfigOption[] | null) {
  if (!configOptions?.length) {
    return null
  }

  return configOptions.find((option) => {
    const label = `${option.category || ''} ${option.id} ${option.name}`.toLowerCase()
    return label.includes('model')
  }) || null
}

export function useChiselSessions() {
  const sessions = useState<models.SessionSummary[]>('chisel-sessions', () => [])
  const recentWorkspaces = useState<models.WorkspaceSummary[]>('chisel-recent-workspaces', () => [])
  const selectedWorkspace = useState<string>('chisel-selected-workspace', () => '')
  const active = useState<models.SessionLoadResult | null>('chisel-active-session', () => null)
  const knownModesByAgent = useState<Record<string, models.SessionMode[]>>('chisel-known-modes-by-agent', () => ({}))
  const preferredModesByAgent = useState<Record<string, string>>('chisel-preferred-modes-by-agent', () => ({}))
  const preferredModelsByAgent = useState<Record<string, string>>('chisel-preferred-models-by-agent', () => ({}))
  const timelineBySession = useState<Record<string, SessionTimelineItem[]>>('chisel-timeline', () => ({}))
  const statuses = useState<Record<string, SessionStatus>>('chisel-statuses', () => ({}))
  const errors = useState<Record<string, string>>('chisel-errors', () => ({}))
  const pendingPermissions = useState<models.PermissionRequest[]>('chisel-permissions', () => [])
  const activeMessageItemIDs = useState<Record<string, string>>('chisel-active-message-item-ids', () => ({}))
  const activeMessageRoles = useState<Record<string, 'user' | 'assistant' | ''>>('chisel-active-message-roles', () => ({}))
  const activeThoughtItemIDs = useState<Record<string, string>>('chisel-active-thought-item-ids', () => ({}))
  const activePlanItemIDs = useState<Record<string, string>>('chisel-active-plan-item-ids', () => ({}))
  const resolvedPermissionStatuses = useState<Record<string, PermissionTimelineStatus>>('chisel-resolved-permission-statuses', () => ({}))

  function ensureSubscription() {
    if (!import.meta.client || subscribed) {
      return
    }

    Events.On('acp:event', ({ data }: { data: unknown }) => {
      const event = models.SessionEvent.createFrom(data)
      handleSessionEvent(event)
    })
    subscribed = true
  }

  function updateSessionState<T>(state: Ref<Record<string, T>>, sessionId: string, value: T) {
    state.value = {
      ...state.value,
      [sessionId]: value
    }
  }

  function rememberModes(agentName: string, modes: models.SessionMode[]) {
    if (!agentName || modes.length === 0) {
      return
    }

    knownModesByAgent.value = {
      ...knownModesByAgent.value,
      [agentName]: modes.map(mode => new models.SessionMode(mode))
    }
  }

  function setPreferredModeForAgent(agentName: string, modeId: string) {
    if (!agentName) {
      return
    }

    if (!modeId) {
      const { [agentName]: _removed, ...remaining } = preferredModesByAgent.value
      preferredModesByAgent.value = remaining
      return
    }

    preferredModesByAgent.value = {
      ...preferredModesByAgent.value,
      [agentName]: modeId
    }
  }

  function setPreferredModelForAgent(agentName: string, modelId: string) {
    if (!agentName) {
      return
    }

    if (!modelId) {
      const { [agentName]: _removed, ...remaining } = preferredModelsByAgent.value
      preferredModelsByAgent.value = remaining
      return
    }

    preferredModelsByAgent.value = {
      ...preferredModelsByAgent.value,
      [agentName]: modelId
    }
  }

  function syncPreferredModelForAgent(agentName: string, configOptions: models.SessionConfigOption[]) {
    const modelOption = findModelOption(configOptions)
    if (!modelOption?.currentValue) {
      return
    }

    setPreferredModelForAgent(agentName, modelOption.currentValue)
  }

  function clearMessageTarget(sessionId: string) {
    updateSessionState(activeMessageItemIDs, sessionId, '')
    updateSessionState(activeMessageRoles, sessionId, '')
  }

  function clearThoughtTarget(sessionId: string) {
    updateSessionState(activeThoughtItemIDs, sessionId, '')
  }

  function clearPlanTarget(sessionId: string) {
    updateSessionState(activePlanItemIDs, sessionId, '')
  }

  function clearTurnTargets(sessionId: string) {
    clearMessageTarget(sessionId)
    clearThoughtTarget(sessionId)
    clearPlanTarget(sessionId)
  }

  function replaceTimeline(sessionId: string, items: SessionTimelineItem[]) {
    timelineBySession.value = {
      ...timelineBySession.value,
      [sessionId]: items
    }
  }

  function appendTimelineItem(sessionId: string, item: SessionTimelineItem) {
    replaceTimeline(sessionId, [...(timelineBySession.value[sessionId] || []), item])
  }

  function updateTimelineItem(sessionId: string, itemId: string, updater: (item: SessionTimelineItem) => SessionTimelineItem) {
    const items = timelineBySession.value[sessionId] || []
    replaceTimeline(sessionId, items.map(item => item.id === itemId ? updater(item) : item))
  }

  function updateSessionSummary(sessionId: string, patch: Partial<models.SessionSummary>) {
    sessions.value = sessions.value.map((session) => {
      if (session.sessionId !== sessionId) {
        return session
      }

      return new models.SessionSummary({
        ...session,
        ...patch
      })
    })

    if (active.value?.session.sessionId === sessionId) {
      active.value = new models.SessionLoadResult({
        ...active.value,
        session: new models.SessionSummary({
          ...active.value.session,
          ...patch
        })
      })
    }
  }

  function clearSessionState(sessionId: string) {
    replaceTimeline(sessionId, [])
    updateSessionState(errors, sessionId, '')
    pendingPermissions.value = pendingPermissions.value.filter(item => item.sessionId !== sessionId)
    clearTurnTargets(sessionId)
  }

  function appendMessageChunk(sessionId: string, role: 'user' | 'assistant', text: string, timestamp: string) {
    const activeMessageItemID = activeMessageItemIDs.value[sessionId]
    const activeRole = activeMessageRoles.value[sessionId]

    if (activeMessageItemID && activeRole === role) {
      updateTimelineItem(sessionId, activeMessageItemID, (item) => {
        if (item.kind !== 'message') {
          return item
        }

        return {
          ...item,
          text: `${item.text}${text}`,
          timestamp: timestamp || item.timestamp
        }
      })
      return
    }

    if (!text) {
      return
    }

    const item: TimelineMessageItem = {
      id: timelineID(role),
      kind: 'message',
      role,
      text,
      timestamp
    }

    appendTimelineItem(sessionId, item)
    updateSessionState(activeMessageItemIDs, sessionId, item.id)
    updateSessionState(activeMessageRoles, sessionId, role)
  }

  function appendThoughtChunk(sessionId: string, text: string, timestamp: string) {
    clearMessageTarget(sessionId)
    const activeThoughtItemID = activeThoughtItemIDs.value[sessionId]

    if (activeThoughtItemID) {
      updateTimelineItem(sessionId, activeThoughtItemID, (item) => {
        if (item.kind !== 'thought') {
          return item
        }

        return {
          ...item,
          text: `${item.text}${text}`,
          timestamp: timestamp || item.timestamp,
          isStreaming: true
        }
      })
      return
    }

    if (!text) {
      return
    }

    const item: TimelineThoughtItem = {
      id: timelineID('thought'),
      kind: 'thought',
      text,
      timestamp,
      isStreaming: true
    }

    appendTimelineItem(sessionId, item)
    updateSessionState(activeThoughtItemIDs, sessionId, item.id)
  }

  function upsertToolCall(sessionId: string, toolCall: models.ToolCallRecord, timestamp: string) {
    clearMessageTarget(sessionId)
    clearThoughtTarget(sessionId)

    const items = timelineBySession.value[sessionId] || []
    const existing = items.find(item => item.kind === 'tool' && item.toolCallId === toolCall.toolCallId)

    if (existing) {
      updateTimelineItem(sessionId, existing.id, (item) => {
        if (item.kind !== 'tool') {
          return item
        }

        return {
          ...item,
          timestamp: timestamp || item.timestamp,
          toolCall: new models.ToolCallRecord({
            ...item.toolCall,
            ...toolCall,
            content: toolCall.content?.length ? toolCall.content : item.toolCall.content
          })
        }
      })
      return
    }

    appendTimelineItem(sessionId, {
      id: timelineID('tool'),
      kind: 'tool',
      toolCallId: toolCall.toolCallId,
      toolCall,
      timestamp
    })
  }

  function upsertPlan(sessionId: string, entries: models.PlanEntry[], timestamp: string) {
    if (!entries.length) {
      return
    }

    clearMessageTarget(sessionId)
    clearThoughtTarget(sessionId)

    const activePlanItemID = activePlanItemIDs.value[sessionId]
    if (activePlanItemID) {
      updateTimelineItem(sessionId, activePlanItemID, (item) => {
        if (item.kind !== 'plan') {
          return item
        }

        return {
          ...item,
          entries,
          timestamp: timestamp || item.timestamp
        }
      })
      return
    }

    const item: TimelinePlanItem = {
      id: timelineID('plan'),
      kind: 'plan',
      entries,
      timestamp
    }

    appendTimelineItem(sessionId, item)
    updateSessionState(activePlanItemIDs, sessionId, item.id)
  }

  function upsertPermission(sessionId: string, request: models.PermissionRequest, status: PermissionTimelineStatus, timestamp: string) {
    clearMessageTarget(sessionId)
    clearThoughtTarget(sessionId)

    const items = timelineBySession.value[sessionId] || []
    const existing = items.find(item => item.kind === 'permission' && item.requestId === request.requestId)

    if (existing) {
      updateTimelineItem(sessionId, existing.id, (item) => {
        if (item.kind !== 'permission') {
          return item
        }

        return {
          ...item,
          request,
          status,
          timestamp: timestamp || item.timestamp
        }
      })
      return
    }

    appendTimelineItem(sessionId, {
      id: timelineID('permission'),
      kind: 'permission',
      requestId: request.requestId,
      request,
      status,
      timestamp
    })
  }

  function resolveTimelinePermission(sessionId: string, requestId: string, timestamp: string) {
    const status = resolvedPermissionStatuses.value[requestId] || 'resolved'
    const items = timelineBySession.value[sessionId] || []
    const existing = items.find(item => item.kind === 'permission' && item.requestId === requestId)

    if (existing) {
      updateTimelineItem(sessionId, existing.id, (item) => {
        if (item.kind !== 'permission') {
          return item
        }

        return {
          ...item,
          status,
          timestamp: timestamp || item.timestamp
        }
      })
    }

    const { [requestId]: _resolvedStatus, ...remainingStatuses } = resolvedPermissionStatuses.value
    resolvedPermissionStatuses.value = remainingStatuses
  }

  function appendError(sessionId: string, text: string, timestamp: string) {
    clearTurnTargets(sessionId)

    const items = timelineBySession.value[sessionId] || []
    const lastItem = items.at(-1)
    if (lastItem?.kind === 'error' && lastItem.text === text) {
      updateTimelineItem(sessionId, lastItem.id, item => ({
        ...item,
        timestamp
      }))
      return
    }

    appendTimelineItem(sessionId, {
      id: timelineID('error'),
      kind: 'error',
      text,
      timestamp
    })
  }

  function finalizeTurn(sessionId: string) {
    const activeThoughtItemID = activeThoughtItemIDs.value[sessionId]
    if (activeThoughtItemID) {
      updateTimelineItem(sessionId, activeThoughtItemID, (item) => {
        if (item.kind !== 'thought') {
          return item
        }

        return {
          ...item,
          isStreaming: false
        }
      })
    }

    clearTurnTargets(sessionId)
  }

  function syncPendingPermissionsTimeline(requests: models.PermissionRequest[]) {
    for (const request of requests) {
      upsertPermission(
        request.sessionId,
        request,
        'pending',
        request.createdAt || new Date().toISOString()
      )
    }
  }

  function handleSessionEvent(event: models.SessionEvent) {
    const sessionId = event.sessionId
    if (!sessionId) {
      return
    }

    const timestamp = event.timestamp || new Date().toISOString()

    switch (event.kind) {
      case 'history_reset':
        clearSessionState(sessionId)
        updateSessionState(statuses, sessionId, 'loading')
        return
      case 'message_chunk':
        if (event.messageRole === 'user' || event.messageRole === 'assistant') {
          appendMessageChunk(sessionId, event.messageRole, event.text || '', timestamp)
        }
        return
      case 'thought_chunk':
        appendThoughtChunk(sessionId, event.thought || '', timestamp)
        return
      case 'tool_call':
      case 'tool_call_update':
        if (event.toolCall) {
          upsertToolCall(sessionId, event.toolCall, timestamp)
        }
        return
      case 'plan_update':
        upsertPlan(sessionId, event.plan || [], timestamp)
        return
      case 'permission_request':
        if (event.permission) {
          pendingPermissions.value = [
            ...pendingPermissions.value.filter(item => item.requestId !== event.permission?.requestId),
            event.permission
          ]
          upsertPermission(sessionId, event.permission, 'pending', timestamp)
        }
        return
      case 'permission_resolved':
        pendingPermissions.value = pendingPermissions.value.filter(item => item.requestId !== event.permission?.requestId)
        if (event.permission?.requestId) {
          resolveTimelinePermission(sessionId, event.permission.requestId, timestamp)
        }
        return
      case 'session_info_update':
        if (event.title) {
          updateSessionSummary(sessionId, { title: event.title, updatedAt: event.timestamp })
        }
        return
      case 'config_options_update':
        if (active.value?.session.sessionId === sessionId) {
          const configOptions = event.configOptions || active.value.configOptions
          syncPreferredModelForAgent(active.value.session.agentName, configOptions)
          active.value = new models.SessionLoadResult({
            ...active.value,
            configOptions
          })
        }
        return
      case 'mode_update':
        if (active.value?.session.sessionId === sessionId) {
          rememberModes(active.value.session.agentName, event.modes || active.value.modes)
          if (event.currentModeId) {
            setPreferredModeForAgent(active.value.session.agentName, event.currentModeId)
          }
          active.value = new models.SessionLoadResult({
            ...active.value,
            modes: event.modes || active.value.modes,
            currentModeId: event.currentModeId || active.value.currentModeId
          })
        }
        return
      case 'turn_end':
        finalizeTurn(sessionId)
        updateSessionState(statuses, sessionId, 'idle')
        updateSessionSummary(sessionId, { updatedAt: event.timestamp, lastOpenedAt: event.timestamp })
        return
      case 'error':
        errors.value = { ...errors.value, [sessionId]: event.error || 'Unknown ACP error' }
        appendError(sessionId, event.error || 'Unknown ACP error', timestamp)
        updateSessionState(statuses, sessionId, 'error')
        return
      default:
        return
    }
  }

  async function refreshSessions() {
    ensureSubscription()
    sessions.value = await ListSessions()
    recentWorkspaces.value = await ListRecentWorkspaces()
  }

  async function refreshPermissions() {
    pendingPermissions.value = await ListPendingPermissions()
    syncPendingPermissionsTimeline(pendingPermissions.value)
  }

  function hydrateActive(result: models.SessionLoadResult) {
    rememberModes(result.session.agentName, result.modes)
    setPreferredModeForAgent(result.session.agentName, result.currentModeId)
    syncPreferredModelForAgent(result.session.agentName, result.configOptions)
    active.value = result
    updateSessionState(statuses, result.session.sessionId, 'idle')
    selectedWorkspace.value = result.session.cwd
    updateSessionSummary(result.session.sessionId, {
      title: result.session.title,
      updatedAt: result.session.updatedAt,
      lastOpenedAt: result.session.lastOpenedAt
    })
  }

  async function createSession(options: { agentName: string, cwd: string, titleHint: string }) {
    ensureSubscription()
    const result = await CreateSession(new models.CreateSessionRequest(options))
    clearSessionState(result.session.sessionId)
    hydrateActive(result)
    await refreshSessions()
    await refreshPermissions()
    return result
  }

  async function loadSession(sessionId: string) {
    ensureSubscription()

    if (active.value?.session.sessionId === sessionId) {
      updateSessionState(statuses, sessionId, statuses.value[sessionId] || 'idle')
      await refreshSessions()
      await refreshPermissions()
      return active.value
    }

    clearSessionState(sessionId)
    updateSessionState(statuses, sessionId, 'loading')
    const result = await LoadSession(sessionId)
    hydrateActive(result)
    await refreshSessions()
    await refreshPermissions()
    if (statuses.value[sessionId] === 'loading') {
      updateSessionState(statuses, sessionId, 'idle')
    }
    return result
  }

  async function sendPrompt(sessionId: string, prompt: string) {
    ensureSubscription()

    const previousTimeline = [...(timelineBySession.value[sessionId] || [])]
    const previousStatus = statuses.value[sessionId] || 'idle'
    const previousError = errors.value[sessionId] || ''
    const previousMessageItemID = activeMessageItemIDs.value[sessionId] || ''
    const previousMessageRole = activeMessageRoles.value[sessionId] || ''
    const previousThoughtItemID = activeThoughtItemIDs.value[sessionId] || ''
    const previousPlanItemID = activePlanItemIDs.value[sessionId] || ''

    clearThoughtTarget(sessionId)
    clearPlanTarget(sessionId)
    appendMessageChunk(sessionId, 'user', prompt, new Date().toISOString())
    clearMessageTarget(sessionId)
    updateSessionState(statuses, sessionId, 'streaming')
    updateSessionState(errors, sessionId, '')

    try {
      await SendPrompt(sessionId, prompt)
    } catch (error) {
      replaceTimeline(sessionId, previousTimeline)
      updateSessionState(statuses, sessionId, previousStatus)
      updateSessionState(errors, sessionId, previousError)
      updateSessionState(activeMessageItemIDs, sessionId, previousMessageItemID)
      updateSessionState(activeMessageRoles, sessionId, previousMessageRole as 'user' | 'assistant' | '')
      updateSessionState(activeThoughtItemIDs, sessionId, previousThoughtItemID)
      updateSessionState(activePlanItemIDs, sessionId, previousPlanItemID)
      throw error
    }

    await refreshSessions()
  }

  async function cancelPrompt(sessionId: string) {
    await CancelPrompt(sessionId)
    finalizeTurn(sessionId)
    updateSessionState(statuses, sessionId, 'idle')
  }

  async function forgetSession(sessionId: string) {
    await ForgetSession(sessionId)
    const { [sessionId]: _timeline, ...remainingTimeline } = timelineBySession.value
    const { [sessionId]: _status, ...remainingStatuses } = statuses.value
    const { [sessionId]: _error, ...remainingErrors } = errors.value
    const { [sessionId]: _messageItemID, ...remainingMessageItemIDs } = activeMessageItemIDs.value
    const { [sessionId]: _messageRole, ...remainingMessageRoles } = activeMessageRoles.value
    const { [sessionId]: _thoughtItemID, ...remainingThoughtItemIDs } = activeThoughtItemIDs.value
    const { [sessionId]: _planItemID, ...remainingPlanItemIDs } = activePlanItemIDs.value

    timelineBySession.value = remainingTimeline
    statuses.value = remainingStatuses
    errors.value = remainingErrors
    activeMessageItemIDs.value = remainingMessageItemIDs
    activeMessageRoles.value = remainingMessageRoles
    activeThoughtItemIDs.value = remainingThoughtItemIDs
    activePlanItemIDs.value = remainingPlanItemIDs
    pendingPermissions.value = pendingPermissions.value.filter(item => item.sessionId !== sessionId)

    if (active.value?.session.sessionId === sessionId) {
      active.value = null
    }

    await refreshSessions()
    await refreshPermissions()
  }

  async function setConfigOption(sessionId: string, optionId: string, value: string) {
    const result = await SetConfigOption(sessionId, optionId, value)
    hydrateActive(result)
    return result
  }

  async function setMode(sessionId: string, modeId: string) {
    const result = await SetMode(sessionId, modeId)
    hydrateActive(result)
    return result
  }

  async function resolvePermission(requestId: string, optionId = '', cancelled = false) {
    resolvedPermissionStatuses.value = {
      ...resolvedPermissionStatuses.value,
      [requestId]: cancelled ? 'cancelled' : optionId ? 'approved' : 'resolved'
    }

    await ResolvePermission(requestId, optionId, cancelled)
    pendingPermissions.value = pendingPermissions.value.filter(item => item.requestId !== requestId)
  }

  async function pickWorkspace(initialDir = '') {
    return PickWorkspace(initialDir)
  }

  function setSelectedWorkspace(path: string) {
    selectedWorkspace.value = path
  }

  function timelineFor(sessionId: string) {
    return timelineBySession.value[sessionId] || []
  }

  function statusFor(sessionId: string) {
    return statuses.value[sessionId] || 'idle'
  }

  function errorFor(sessionId: string) {
    return errors.value[sessionId] || ''
  }

  return {
    sessions,
    recentWorkspaces,
    selectedWorkspace,
    active,
    knownModesByAgent,
    preferredModesByAgent,
    preferredModelsByAgent,
    pendingPermissions,
    refreshSessions,
    refreshPermissions,
    createSession,
    loadSession,
    sendPrompt,
    cancelPrompt,
    forgetSession,
    setConfigOption,
    setMode,
    resolvePermission,
    pickWorkspace,
    setSelectedWorkspace,
    setPreferredModeForAgent,
    setPreferredModelForAgent,
    timelineFor,
    statusFor,
    errorFor
  }
}
