import { Flags } from '@wailsio/runtime'

function hasNativeDesktopBridge() {
  if (!import.meta.client) {
    return false
  }

  const runtimeWindow = window as typeof window & {
    chrome?: {
      webview?: {
        postMessage?: (message: unknown) => void
      }
    }
    webkit?: {
      messageHandlers?: {
        external?: {
          postMessage?: (message: unknown) => void
        }
      }
    }
    wails?: {
      invoke?: (message: unknown) => void
    }
  }

  return Boolean(
    runtimeWindow.chrome?.webview?.postMessage
    || runtimeWindow.webkit?.messageHandlers?.external?.postMessage
    || runtimeWindow.wails?.invoke
  )
}

export function useRuntimeMode() {
  const ready = useState('runtime-mode-ready', () => false)
  const isServerMode = useState('runtime-server-mode', () => false)
  const supportsPicker = useState('runtime-native-workspace-picker', () => true)

  if (import.meta.client && !ready.value) {
    const nativeDesktopBridge = hasNativeDesktopBridge()

    try {
      isServerMode.value = Boolean(Flags.GetFlag('server'))
      const pickerFlag = Flags.GetFlag('supportsNativeWorkspacePicker')
      supportsPicker.value = typeof pickerFlag === 'boolean' ? pickerFlag : !isServerMode.value
    } catch {
      isServerMode.value = !nativeDesktopBridge
      supportsPicker.value = nativeDesktopBridge
    }

    if (!nativeDesktopBridge) {
      supportsPicker.value = false
    }

    ready.value = true
  }

  return {
    isServerMode: computed(() => isServerMode.value),
    supportsNativeWorkspacePicker: computed(() => supportsPicker.value)
  }
}
