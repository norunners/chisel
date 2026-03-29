<script setup lang="ts">
interface ProjectItem {
  label: string
  path: string
}

const props = withDefaults(defineProps<{
  selectedPath?: string
  items?: ProjectItem[]
  supportsNativeWorkspacePicker?: boolean
  disabled?: boolean
}>(), {
  selectedPath: '',
  items: () => [],
  supportsNativeWorkspacePicker: true,
  disabled: false
})

const emit = defineEmits<{
  select: [path: string]
  addProject: []
  addProjectByPath: [path: string]
}>()

const open = ref(false)
const query = ref('')
const manualMode = ref(false)
const manualPath = ref('')

const filteredItems = computed(() => {
  const normalizedQuery = query.value.trim().toLowerCase()
  if (!normalizedQuery) {
    return props.items
  }

  return props.items.filter((item) => {
    return item.label.toLowerCase().includes(normalizedQuery) || item.path.toLowerCase().includes(normalizedQuery)
  })
})

function currentProjectLabel(path: string) {
  const trimmed = path.replace(/[\\/]+$/, '')
  const segments = trimmed.split(/[\\/]/).filter(Boolean)
  return segments.at(-1) || path || 'New project'
}

const selectedLabel = computed(() => {
  const selectedItem = props.items.find(item => item.path === props.selectedPath)
  return selectedItem?.label || currentProjectLabel(props.selectedPath)
})

function resetMenuState() {
  query.value = ''
  manualMode.value = false
  manualPath.value = ''
}

watch(() => open.value, (isOpen) => {
  if (!isOpen) {
    resetMenuState()
  }
})

function chooseProject(path: string) {
  emit('select', path)
  open.value = false
}

function handleAddProject() {
  if (props.supportsNativeWorkspacePicker) {
    open.value = false
    emit('addProject')
    return
  }

  manualMode.value = true
}

function submitManualProject() {
  const nextPath = manualPath.value.trim()
  if (!nextPath) {
    return
  }

  emit('addProjectByPath', nextPath)
  open.value = false
}
</script>

<template>
  <UPopover
    v-model:open="open"
    :content="{ side: 'bottom', align: 'center', sideOffset: 12 }"
  >
    <button
      type="button"
      class="inline-flex items-center gap-1 rounded-full px-2 py-1 text-xl font-semibold text-highlighted transition hover:bg-elevated/60 disabled:cursor-not-allowed disabled:opacity-60 sm:text-2xl"
      :disabled="disabled"
    >
      <span>{{ selectedLabel }}</span>
      <UIcon name="i-lucide-chevron-down" class="size-4 text-muted" />
    </button>

    <template #content>
      <div class="w-80 max-w-[calc(100vw-2rem)] rounded-2xl border border-default bg-default/95 p-3 shadow-lg backdrop-blur">
        <div class="space-y-3">
          <UInput
            v-model="query"
            icon="i-lucide-search"
            placeholder="Search projects"
            autocomplete="off"
          />

          <div class="max-h-64 space-y-1 overflow-y-auto">
            <button
              v-for="item in filteredItems"
              :key="item.path"
              type="button"
              class="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left transition hover:bg-elevated"
              @click="chooseProject(item.path)"
            >
              <UIcon name="i-lucide-folder" class="size-4 shrink-0 text-muted" />

              <div class="min-w-0 flex-1">
                <p class="truncate text-sm font-medium text-highlighted">
                  {{ item.label }}
                </p>
                <p class="truncate text-xs text-muted">
                  {{ item.path }}
                </p>
              </div>

              <UIcon
                v-if="item.path === selectedPath"
                name="i-lucide-check"
                class="size-4 shrink-0 text-primary"
              />
            </button>

            <p v-if="filteredItems.length === 0" class="px-3 py-4 text-sm text-muted">
              No projects match that search yet.
            </p>
          </div>

          <div class="border-t border-default pt-3">
            <button
              type="button"
              class="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left text-sm font-medium text-highlighted transition hover:bg-elevated"
              @click="handleAddProject"
            >
              <UIcon name="i-lucide-folder-plus" class="size-4 shrink-0 text-muted" />
              <span>New project</span>
            </button>

            <div v-if="manualMode && !supportsNativeWorkspacePicker" class="space-y-2 px-3 pt-3">
              <UInput
                v-model="manualPath"
                placeholder="/path/to/project"
                autocomplete="off"
                @keyup.enter="submitManualProject"
              />

              <div class="flex justify-end gap-2">
                <UButton
                  label="Cancel"
                  color="neutral"
                  variant="ghost"
                  size="sm"
                  @click="manualMode = false"
                />
                <UButton
                  label="Use project"
                  color="neutral"
                  size="sm"
                  @click="submitManualProject"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </UPopover>
</template>
