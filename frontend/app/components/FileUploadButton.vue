<script setup lang="ts">
const toast = useChiselToast()

function showComingSoon() {
  toast.add({
    title: 'Attachments coming soon',
    description: 'The + button is reserved for ACP attachments. That flow is not wired up yet.',
    color: 'neutral'
  })
}
</script>

<template>
  <UTooltip
    :content="{
      side: 'top'
    }"
    text="Attachments are not wired up yet"
  >
    <UButton
      icon="i-lucide-plus"
      variant="ghost"
      color="neutral"
      size="sm"
      @click="showComingSoon"
    />
  </UTooltip>
</template>
