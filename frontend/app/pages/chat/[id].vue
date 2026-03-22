<script setup lang="ts">
import type { DefineComponent } from 'vue'
import type { UIMessage } from 'ai'
import { useClipboard } from '@vueuse/core'
import { getTextFromMessage } from '@nuxt/ui/utils/ai'
import ProseStreamPre from '../../components/prose/PreStream.vue'

const components = {
  pre: ProseStreamPre as unknown as DefineComponent
}

const route = useRoute()
const toast = useToast()
const clipboard = useClipboard()
const { model } = useModels()
const {
  chats,
  getChat,
  appendUserMessage,
  generateAssistantMessage,
  regenerateAssistantMessage
} = useDesktopChats()

function getFileName(url: string): string {
  try {
    const urlObj = new URL(url)
    const pathname = urlObj.pathname
    const filename = pathname.split('/').pop() || 'file'
    return decodeURIComponent(filename)
  } catch {
    return 'file'
  }
}

const {
  dropzoneRef,
  isDragging,
  open,
  files,
  isUploading,
  removeFile,
  clearFiles
} = useFileUploadWithStatus(route.params.id as string)

const currentChat = computed(() => getChat(route.params.id as string))

if (!currentChat.value) {
  throw createError({ statusCode: 404, statusMessage: 'Chat not found' })
}

const input = ref('')
const status = ref<'ready' | 'streaming'>('ready')
const error = ref<Error | undefined>()
const messages = computed<UIMessage[]>(() => {
  const chat = chats.value.find(item => item.id === route.params.id)
  return chat?.messages || []
})

async function runAssistantReply() {
  status.value = 'streaming'
  error.value = undefined

  try {
    await generateAssistantMessage(route.params.id as string, model.value)
  } catch (responseError) {
    error.value = responseError instanceof Error ? responseError : new Error('Unable to generate a local reply.')
    toast.add({
      description: error.value.message,
      icon: 'i-lucide-alert-circle',
      color: 'error',
      duration: 0
    })
  } finally {
    status.value = 'ready'
  }
}

async function handleSubmit(e: Event) {
  e.preventDefault()
  if (input.value.trim() && !isUploading.value) {
    appendUserMessage(route.params.id as string, input.value.trim())
    input.value = ''
    clearFiles()
    await runAssistantReply()
  }
}

const copied = ref(false)

function copy(e: MouseEvent, message: UIMessage) {
  clipboard.copy(getTextFromMessage(message))

  copied.value = true

  setTimeout(() => {
    copied.value = false
  }, 2000)
}

onMounted(() => {
  if (messages.value.length === 1) {
    runAssistantReply()
  }
})
</script>

<template>
  <UDashboardPanel
    id="chat"
    class="relative min-h-0"
    :ui="{ body: 'p-0 sm:p-0 overscroll-none' }"
  >
    <template #header>
      <DashboardNavbar />
    </template>

    <template #body>
      <div ref="dropzoneRef" class="flex flex-1">
        <DragDropOverlay :show="isDragging" />

        <UContainer class="flex-1 flex flex-col gap-4 sm:gap-6">
          <UChatMessages
            should-auto-scroll
            :messages="messages"
            :status="status"
            :assistant="status !== 'streaming' ? { actions: [{ label: 'Copy', icon: copied ? 'i-lucide-copy-check' : 'i-lucide-copy', onClick: copy }] } : { actions: [] }"
            :spacing-offset="160"
            class="lg:pt-(--ui-header-height) pb-4 sm:pb-6"
          >
            <template #content="{ message }">
              <template v-for="(part, index) in message.parts" :key="`${message.id}-${part.type}-${index}${'state' in part ? `-${part.state}` : ''}`">
                <Reasoning
                  v-if="part.type === 'reasoning'"
                  :text="part.text"
                  :is-streaming="part.state !== 'done'"
                />
                <!-- Only render markdown for assistant messages to prevent XSS from user input -->
                <MDCCached
                  v-else-if="part.type === 'text' && message.role === 'assistant'"
                  :value="part.text"
                  :cache-key="`${message.id}-${index}`"
                  :components="components"
                  :parser-options="{ highlight: false }"
                  class="*:first:mt-0 *:last:mb-0"
                />
                <!-- User messages are rendered as plain text (safely escaped by Vue) -->
                <p v-else-if="part.type === 'text' && message.role === 'user'" class="whitespace-pre-wrap">
                  {{ part.text }}
                </p>
                <FileAvatar
                  v-else-if="part.type === 'file'"
                  :name="getFileName(part.url)"
                  :type="part.mediaType"
                  :preview-url="part.url"
                  class="inline-flex"
                />
              </template>
            </template>
          </UChatMessages>

          <UChatPrompt
            v-model="input"
            :error="error"
            :disabled="isUploading"
            variant="subtle"
            class="sticky bottom-0 [view-transition-name:chat-prompt] rounded-b-none z-10"
            :ui="{ base: 'px-1.5' }"
            @submit="handleSubmit"
          >
            <template v-if="files.length > 0" #header>
              <div class="flex flex-wrap gap-2">
                <FileAvatar
                  v-for="fileWithStatus in files"
                  :key="fileWithStatus.id"
                  :name="fileWithStatus.file.name"
                  :type="fileWithStatus.file.type"
                  :preview-url="fileWithStatus.previewUrl"
                  :status="fileWithStatus.status"
                  :error="fileWithStatus.error"
                  removable
                  @remove="removeFile(fileWithStatus.id)"
                />
              </div>
            </template>

            <template #footer>
              <div class="flex items-center gap-1">
                <FileUploadButton :open="open" />

                <ModelSelect />
              </div>

              <UChatPromptSubmit
                :status="status"
                :disabled="isUploading"
                color="neutral"
                size="sm"
                @stop="status = 'ready'"
                @reload="regenerateAssistantMessage(route.params.id as string, model)"
              />
            </template>
          </UChatPrompt>
        </UContainer>
      </div>
    </template>
  </UDashboardPanel>
</template>
