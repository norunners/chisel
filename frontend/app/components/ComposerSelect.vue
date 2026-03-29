<script setup lang="ts">
interface SelectItem {
  label: string
  value: string
  disabled?: boolean
}

withDefaults(defineProps<{
  items?: SelectItem[]
  selected?: string
  ariaLabel?: string
  emptyLabel?: string
  disabled?: boolean
}>(), {
  items: () => [],
  selected: '',
  ariaLabel: 'Select an option',
  emptyLabel: 'No options available',
  disabled: false
})

const emit = defineEmits<{
  'update:selected': [value: string]
}>()

function handleChange(event: Event) {
  emit('update:selected', (event.target as HTMLSelectElement).value)
}
</script>

<template>
  <UTooltip
    :content="{
      side: 'top'
    }"
    :text="ariaLabel"
  >
    <label class="relative block min-w-0">
      <span class="sr-only">{{ ariaLabel }}</span>

      <select
        :value="selected"
        :disabled="disabled"
        :title="ariaLabel"
        class="max-w-56 min-w-0 appearance-none rounded-full border border-default bg-default/85 px-3 py-1.5 pr-8 text-xs text-highlighted shadow-sm backdrop-blur outline-none transition disabled:cursor-not-allowed disabled:opacity-60 sm:text-sm"
        @change="handleChange"
      >
        <option v-if="items.length === 0" value="">
          {{ emptyLabel }}
        </option>

        <option
          v-for="item in items"
          :key="item.value"
          :value="item.value"
          :disabled="item.disabled"
        >
          {{ item.label }}
        </option>
      </select>

      <UIcon
        name="i-lucide-chevron-down"
        class="pointer-events-none absolute right-2 top-1/2 size-3.5 -translate-y-1/2 text-muted"
      />
    </label>
  </UTooltip>
</template>
