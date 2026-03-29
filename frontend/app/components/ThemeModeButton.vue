<script setup lang="ts">
const colorMode = useColorMode()

const modeOrder = ['system', 'light', 'dark'] as const

const modeMeta = {
  system: {
    icon: 'i-lucide-monitor',
    label: 'System'
  },
  light: {
    icon: 'i-lucide-sun',
    label: 'Light'
  },
  dark: {
    icon: 'i-lucide-moon',
    label: 'Dark'
  }
} as const

const currentPreference = computed(() => {
  const preference = colorMode.preference
  return preference === 'light' || preference === 'dark' ? preference : 'system'
})

const currentMeta = computed(() => modeMeta[currentPreference.value])
const tooltipText = computed(() => `Theme: ${currentMeta.value.label}`)

function cycleTheme() {
  const currentIndex = modeOrder.indexOf(currentPreference.value)
  const nextMode = modeOrder[(currentIndex + 1) % modeOrder.length] || 'system'
  colorMode.preference = nextMode
}
</script>

<template>
  <UTooltip
    :content="{
      side: 'top'
    }"
    :text="tooltipText"
  >
    <UButton
      :icon="currentMeta.icon"
      color="neutral"
      variant="ghost"
      size="sm"
      :aria-label="tooltipText"
      :title="tooltipText"
      @click="cycleTheme"
    />
  </UTooltip>
</template>
