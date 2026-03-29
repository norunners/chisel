<script setup lang="ts">
import type * as models from '../../bindings/github.com/norunners/chisel/models'

const { entries } = defineProps<{
  entries: models.PlanEntry[]
}>()

function iconForStatus(status: string) {
  switch (status.toLowerCase()) {
    case 'completed':
    case 'done':
      return 'i-lucide-circle-check-big'
    case 'in_progress':
    case 'in-progress':
    case 'active':
      return 'i-lucide-loader-circle'
    default:
      return 'i-lucide-circle'
  }
}

function statusLabel(status: string) {
  return status.replaceAll('_', ' ') || 'pending'
}
</script>

<template>
  <div class="max-w-3xl rounded-2xl border border-default bg-default/80 p-4 shadow-sm space-y-3">
    <div class="flex items-center gap-2">
      <UIcon name="i-lucide-list-todo" class="size-4 text-primary" />
      <p class="text-sm font-semibold text-highlighted">
        Plan
      </p>
    </div>

    <div class="space-y-2">
      <div
        v-for="(entry, index) in entries"
        :key="`${entry.content}-${index}`"
        class="flex items-start gap-3 rounded-xl border border-default/70 bg-default/60 px-3 py-2"
      >
        <UIcon
          :name="iconForStatus(entry.status || '')"
          class="size-4 mt-0.5 shrink-0"
          :class="entry.status === 'completed' ? 'text-primary' : entry.status === 'in_progress' ? 'text-amber-500 animate-spin' : 'text-muted'"
        />

        <div class="min-w-0 space-y-1">
          <p class="text-sm text-highlighted whitespace-pre-wrap">
            {{ entry.content }}
          </p>
          <p class="text-[11px] uppercase tracking-[0.16em] text-muted">
            {{ statusLabel(entry.status || '') }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
