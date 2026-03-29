<script setup lang="ts">
import type * as models from '../../bindings/github.com/norunners/chisel/models'
import type { PermissionTimelineStatus } from '../composables/useChiselSessions'

const props = defineProps<{
  request: models.PermissionRequest
  status: PermissionTimelineStatus
}>()

const emit = defineEmits<{
  choose: [requestId: string, optionId: string]
  cancel: [requestId: string]
}>()

const statusLabel = computed(() => {
  switch (props.status) {
    case 'approved':
      return 'Approved'
    case 'cancelled':
      return 'Cancelled'
    case 'resolved':
      return 'Resolved'
    default:
      return 'Waiting'
  }
})
</script>

<template>
  <div class="max-w-3xl rounded-2xl border border-amber-500/30 bg-amber-500/8 p-4 shadow-sm space-y-3">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="space-y-1">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-shield-alert" class="size-4 text-amber-500" />
          <p class="text-sm font-semibold text-highlighted">
            {{ request.title || 'Permission request' }}
          </p>
        </div>
        <p class="text-sm text-muted whitespace-pre-wrap">
          {{ request.description || 'Choose how Chisel should respond.' }}
        </p>
      </div>

      <UBadge :label="statusLabel" color="warning" variant="subtle" />
    </div>

    <div v-if="status === 'pending'" class="flex flex-wrap gap-2">
      <UButton
        v-for="option in request.options"
        :key="option.id"
        :label="option.name"
        color="neutral"
        variant="soft"
        @click="emit('choose', request.requestId, option.id)"
      />
      <UButton
        label="Cancel"
        color="neutral"
        variant="ghost"
        @click="emit('cancel', request.requestId)"
      />
    </div>

    <p v-else class="text-sm text-muted">
      This request has already been answered, so the agent can continue.
    </p>
  </div>
</template>
