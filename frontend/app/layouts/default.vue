<script setup lang="ts">
import { LazyModalConfirm } from '#components'

const route = useRoute()
const toast = useChiselToast()
const overlay = useOverlay()
const newThreadToken = useState<number>('new-thread-token', () => 0)
const { sessions, recentWorkspaces, forgetSession, refreshSessions, pickWorkspace, selectedWorkspace, setSelectedWorkspace } = useChiselSessions()
const { supportsNativeWorkspacePicker } = useRuntimeMode()
const settingsReturnPath = useState<string>('settings-return-path', () => '/')

onMounted(() => {
  refreshSessions()
})

const open = ref(false)
const pickingProject = ref(false)

interface SessionNavigationItem {
  id: string
  label: string
  to: string
  createdAt: string
  class?: string
}

interface SessionProjectGroup {
  id: string
  label: string
  items: SessionNavigationItem[]
}

const forgetModal = overlay.create(LazyModalConfirm, {
  props: {
    title: 'Forget session',
    description: 'Remove this session from Chisel’s sidebar? The backend-owned session data will not be deleted.'
  }
})

function getProjectName(cwd: string) {
  const trimmed = cwd.replace(/[\\/]+$/, '')
  const segments = trimmed.split(/[\\/]/).filter(Boolean)
  return segments.at(-1) || trimmed || 'Workspace'
}

const sortedSessions = computed(() => [...sessions.value].sort((a, b) => {
  const left = new Date(b.lastOpenedAt || b.updatedAt || b.createdAt).getTime()
  const right = new Date(a.lastOpenedAt || a.updatedAt || a.createdAt).getTime()
  return left - right
}))

const latestWorkspacePath = computed(() => {
  const sortedRecent = [...recentWorkspaces.value].sort((left, right) => {
    return new Date(right.lastOpenedAt || 0).getTime() - new Date(left.lastOpenedAt || 0).getTime()
  })

  return sortedRecent[0]?.path || sortedSessions.value[0]?.cwd || selectedWorkspace.value
})

const groups = computed(() => {
  const result: SessionProjectGroup[] = []
  const byProject = new Map<string, SessionProjectGroup>()

  for (const session of sortedSessions.value) {
    const projectName = getProjectName(session.cwd)

    if (!byProject.has(session.cwd)) {
      const group = {
        id: session.cwd,
        label: projectName,
        items: []
      }
      byProject.set(session.cwd, group)
      result.push(group)
    }

    byProject.get(session.cwd)?.items.push({
      id: session.sessionId,
      label: session.title || 'Untitled Session',
      to: `/chat/${session.sessionId}`,
      createdAt: session.createdAt,
      class: session.title ? 'ps-4' : 'ps-4 text-muted'
    })
  }

  return result
})

const items = computed(() => groups.value.flatMap(group => [{
  label: group.label,
  type: 'label' as const
}, ...group.items.map(item => ({
  ...item,
  slot: 'session' as const,
  icon: undefined
}))]))

const isSettingsRoute = computed(() => route.path === '/settings')
const settingsToggleTarget = computed(() => isSettingsRoute.value ? settingsReturnPath.value || '/' : '/settings')

watch(() => route.fullPath, (fullPath) => {
  if (route.path !== '/settings') {
    settingsReturnPath.value = fullPath
  }
}, { immediate: true })

async function addProject() {
  open.value = false

  if (!supportsNativeWorkspacePicker.value) {
    toast.add({
      title: 'Project picker unavailable',
      description: 'Enter an absolute workspace path from the home screen when running in browser mode.',
      color: 'warning'
    })
    await navigateTo('/')
    return
  }

  pickingProject.value = true
  try {
    const selection = await pickWorkspace(selectedWorkspace.value)
    if (!selection) {
      return
    }

    setSelectedWorkspace(selection)
    await navigateTo('/')
  } catch (error) {
    toast.add({
      title: 'Unable to choose project',
      description: error instanceof Error ? error.message : 'The folder picker could not open.',
      color: 'error'
    })
  } finally {
    pickingProject.value = false
  }
}

async function openNewThread() {
  if (latestWorkspacePath.value) {
    setSelectedWorkspace(latestWorkspacePath.value)
  }

  newThreadToken.value += 1
  open.value = false
  await navigateTo('/')
}

async function forget(id: string) {
  const instance = forgetModal.open()
  const result = await instance.result
  if (!result) {
    return
  }

  await forgetSession(id)
  toast.add({
    title: 'Session removed',
    description: 'The session has been removed from Chisel’s local index.',
    icon: 'i-lucide-trash'
  })

  if (route.params.id === id) {
    await navigateTo('/')
  }
}

defineShortcuts({
  c: () => navigateTo('/'),
  s: () => navigateTo('/settings')
})
</script>

<template>
  <UDashboardGroup unit="rem">
    <UDashboardSidebar
      id="default"
      v-model:open="open"
      :min-size="14"
      collapsible
      resizable
      class="border-r-0 py-4"
    >
      <template #header="{ collapsed }">
        <NuxtLink to="/" class="flex items-end gap-2">
          <UIcon name="i-lucide-pickaxe" class="h-7 w-7 shrink-0 text-primary" />
          <span v-if="!collapsed" class="text-xl font-bold text-highlighted">Chisel</span>
        </NuxtLink>

        <div v-if="!collapsed" class="flex items-center gap-1.5 ms-auto">
          <UDashboardSearchButton collapsed />
        </div>
      </template>

      <template #default="{ collapsed }">
        <div v-if="collapsed" class="flex flex-col gap-1.5">
          <UDashboardSearchButton collapsed />

          <UButton
            icon="i-lucide-square-pen"
            variant="soft"
            block
            title="New session"
            aria-label="New session"
            @click="openNewThread"
          />

          <UTooltip
            :content="{ side: 'right' }"
            text="New project"
          >
            <UButton
              icon="i-lucide-folder-plus"
              variant="soft"
              block
              :loading="pickingProject"
              title="New project"
              aria-label="New project"
              @click="addProject"
            />
          </UTooltip>
        </div>

        <div v-else class="space-y-3 px-2 pb-2">
          <UButton
            icon="i-lucide-square-pen"
            label="New session"
            color="neutral"
            variant="soft"
            block
            class="justify-start"
            @click="openNewThread"
          />

          <div class="flex items-center justify-between gap-2">
            <p class="text-xs font-medium uppercase tracking-[0.18em] text-muted">
              Sessions
            </p>

            <UTooltip
              :content="{ side: 'top' }"
              text="New project"
            >
              <UButton
                color="neutral"
                variant="ghost"
                icon="i-lucide-folder-plus"
                size="sm"
                :loading="pickingProject"
                title="New project"
                aria-label="New project"
                @click="addProject"
              />
            </UTooltip>
          </div>
        </div>

        <UNavigationMenu
          v-if="!collapsed"
          :items="items"
          orientation="vertical"
          :ui="{ link: 'overflow-hidden', label: 'text-[11px] uppercase tracking-[0.16em] text-muted' }"
        >
          <template #session-trailing="{ item }">
            <div class="flex -mr-1.25 translate-x-full group-hover:translate-x-0 transition-transform">
              <UButton
                icon="i-lucide-x"
                color="neutral"
                variant="ghost"
                size="xs"
                class="text-muted hover:text-primary hover:bg-accented/50 focus-visible:bg-accented/50 p-0.5"
                tabindex="-1"
                @click.stop.prevent="forget((item as any).id)"
              />
            </div>
          </template>
        </UNavigationMenu>
      </template>

      <template #footer="{ collapsed }">
        <div
          class="flex w-full items-center gap-2"
          :class="collapsed ? 'flex-col justify-center' : ''"
        >
          <UButton
            v-if="!collapsed"
            color="neutral"
            variant="ghost"
            icon="i-lucide-settings"
            label="Settings"
            :to="settingsToggleTarget"
            class="-ml-2 text-muted"
            @click="open = false"
          />
          <div :class="collapsed ? '' : 'ms-auto'">
            <ThemeModeButton />
          </div>
          <UButton
            v-if="collapsed"
            color="neutral"
            variant="ghost"
            icon="i-lucide-settings"
            :to="settingsToggleTarget"
            title="Settings"
            aria-label="Settings"
            @click="open = false"
          />
        </div>
      </template>
    </UDashboardSidebar>

    <UDashboardSearch
      placeholder="Search sessions..."
      :groups="[{
        id: 'links',
        items: [
          {
            label: 'New session',
            to: '/',
            icon: 'i-lucide-square-pen'
          },
          {
            label: 'Settings',
            to: '/settings',
            icon: 'i-lucide-settings'
          }
        ]
      }, ...groups]"
    />

    <div class="flex-1 flex m-4 lg:ml-0 rounded-lg ring ring-default bg-default/75 shadow min-w-0">
      <slot />
    </div>
  </UDashboardGroup>
</template>
