<script setup lang="ts">
defineProps<{
  open: () => void
}>()
</script>

<template>
  <UTooltip
    :content="{
      side: 'top'
    }"
    text="File uploads are not wired up in the desktop static build yet"
  >
    <UButton
      icon="i-lucide-paperclip"
      variant="ghost"
      color="neutral"
      size="sm"
      disabled
    />
  </UTooltip>
</template>
