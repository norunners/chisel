export function useFileUploadWithStatus(chatId: string) {
  void chatId

  const dropzoneRef = ref<HTMLElement | null>(null)
  const isDragging = ref(false)
  const files = ref<FileWithStatus[]>([])
  const isUploading = computed(() => false)
  const uploadedFiles = computed(() => [] as Array<{ type: 'file', mediaType: string, url: string }>)

  function open() {}
  function removeFile(id: string) {
    void id
  }
  function clearFiles() {
    files.value = []
  }

  return {
    dropzoneRef,
    isDragging,
    open,
    files,
    isUploading,
    uploadedFiles,
    addFiles: async () => {},
    removeFile,
    clearFiles
  }
}
