<script setup lang="ts">
import type * as models from '../../bindings/github.com/norunners/chisel/models'

const { toolCall } = defineProps<{
  toolCall: models.ToolCallRecord
}>()

const open = ref(false)

const statusLabel = computed(() => toolCall.status || 'running')
const hasContent = computed(() => toolCall.content.some(content => content.output || content.text || content.path))

watch(() => toolCall.status, (status) => {
  const normalized = (status || '').toLowerCase()
  open.value = normalized === '' || normalized === 'running' || normalized === 'in_progress'
}, { immediate: true })

const statusIcon = computed(() => {
  const normalized = statusLabel.value.toLowerCase()
  if (normalized === 'completed' || normalized === 'done' || normalized === 'succeeded') {
    return 'i-lucide-circle-check-big'
  }
  if (normalized === 'failed' || normalized === 'error' || normalized === 'cancelled') {
    return 'i-lucide-circle-alert'
  }
  return 'i-lucide-loader-circle'
})
</script>

<template>
  <UCollapsible v-model:open="open" class="max-w-3xl">
    <UButton
      color="neutral"
      variant="ghost"
      block
      class="justify-between rounded-2xl border border-default bg-default/80 px-4 py-3 shadow-sm"
      :ui="{
        base: 'justify-between'
      }"
    >
      <template #leading>
        <div class="flex items-center gap-2">
          <UIcon
            :name="statusIcon"
            class="size-4"
            :class="statusLabel === 'running' || statusLabel === 'in_progress' ? 'animate-spin text-primary' : statusLabel === 'failed' ? 'text-error' : 'text-primary'"
          />
          <span class="text-sm font-medium text-highlighted">
            {{ toolCall.title || toolCall.kind || 'Tool activity' }}
          </span>
        </div>
      </template>

      <template #trailing>
        <div class="flex items-center gap-2">
          <UBadge
            :label="statusLabel.replaceAll('_', ' ')"
            color="neutral"
            variant="subtle"
          />
          <UIcon name="i-lucide-chevron-down" class="size-4 text-muted group-data-[state=open]:rotate-180 transition-transform duration-200" />
        </div>
      </template>
    </UButton>

    <template #content>
      <div class="space-y-3 rounded-b-2xl border border-default border-t-0 bg-default/80 px-4 py-4 shadow-sm">
        <p v-if="!hasContent" class="text-sm text-muted">
          The agent is still preparing this tool call.
        </p>

        <template v-for="(content, index) in toolCall.content" :key="`${toolCall.toolCallId}-${content.type}-${index}`">
          <pre
            v-if="content.type === 'terminal' && content.output"
            class="overflow-x-auto rounded-xl bg-neutral-950 p-3 text-xs text-neutral-100 whitespace-pre-wrap"
          >{{ content.output }}</pre>

          <div
            v-else-if="content.text || content.path || content.description"
            class="rounded-xl border border-default/70 bg-default/60 px-3 py-2 space-y-1"
          >
            <p v-if="content.description" class="text-xs uppercase tracking-[0.16em] text-muted">
              {{ content.description }}
            </p>
            <p v-if="content.text" class="text-sm text-muted whitespace-pre-wrap">
              {{ content.text }}
            </p>
            <p v-if="content.path" class="text-xs font-mono text-muted break-all">
              {{ content.path }}
            </p>
          </div>
        </template>
      </div>
    </template>
  </UCollapsible>
</template>
