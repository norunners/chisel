interface ToastTextLike {
  title?: unknown
  description?: unknown
}

function getToastCopyText(toast: ToastTextLike) {
  const parts: string[] = []

  if (typeof toast.title === 'string' && toast.title.trim()) {
    parts.push(toast.title.trim())
  }

  if (typeof toast.description === 'string' && toast.description.trim()) {
    parts.push(toast.description.trim())
  }

  return parts.join('\n\n').trim()
}

async function copyToastText(text: string) {
  if (!import.meta.client || !text) {
    return
  }

  await navigator.clipboard.writeText(text)
}

export function useChiselToast() {
  const toast = useToast()
  type ToastInput = Parameters<typeof toast.add>[0]
  type ToastAction = NonNullable<ToastInput['actions']>[number]

  function add(options: ToastInput) {
    const isErrorToast = options.color === 'error'
    const copyText = getToastCopyText(options)

    if (!isErrorToast || !copyText) {
      return toast.add(options)
    }

    const actions = [...(options.actions || [])]

    if (!actions.some(action => action.label === 'Copy')) {
      actions.push({
        label: 'Copy',
        icon: 'i-lucide-copy',
        color: 'neutral',
        variant: 'outline',
        onClick: async () => {
          await copyToastText(copyText)
        }
      } as ToastAction)
    }

    return toast.add({
      ...options,
      duration: options.duration ?? 0,
      actions
    })
  }

  return {
    ...toast,
    add
  }
}
