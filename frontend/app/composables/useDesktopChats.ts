import type { UIMessage } from 'ai'

export interface DesktopChat {
  id: string
  title: string
  createdAt: string
  updatedAt: string
  messages: UIMessage[]
}

const STORAGE_KEY = 'chisel.desktop.chats'

function createTextMessage(role: 'user' | 'assistant', text: string): UIMessage {
  return {
    id: crypto.randomUUID(),
    role,
    parts: [
      {
        type: 'text',
        text
      }
    ]
  }
}

function getMessageText(message: UIMessage | undefined): string {
  if (!message) {
    return ''
  }

  return message.parts.reduce((text, part) => {
    if (part.type !== 'text') {
      return text
    }

    return [...text, part.text]
  }, [] as string[]).join('\n').trim()
}

function makeChatTitle(prompt: string): string {
  const normalized = prompt.trim().replace(/\s+/g, ' ')
  if (!normalized) {
    return 'Untitled'
  }

  return normalized.length > 48 ? `${normalized.slice(0, 45)}...` : normalized
}

function buildAssistantReply(prompt: string, model: string): string {
  const normalized = prompt.trim()
  const lower = normalized.toLowerCase()

  if (lower.includes('vue') || lower.includes('nuxt')) {
    return [
      `Here is a solid starting point for **${normalized}**.`,
      '',
      '1. Start with the smallest component or composable that proves the flow.',
      '2. Keep state close to the feature until multiple screens need it.',
      '3. Prefer typed props and typed return values so the template stays easy to extend.',
      '',
      `The current desktop build is running in local mode, so this reply is generated from the **${model}** preset rather than a live model backend.`
    ].join('\n')
  }

  if (lower.includes('weather') || lower.includes('chart')) {
    return [
      `I can help sketch out **${normalized}**, but the desktop scaffold is not connected to a live data source yet.`,
      '',
      'A good next step would be:',
      '- wire the chat request to a Wails Go service',
      '- fetch the data there',
      '- stream or return the result back to the Nuxt UI',
      '',
      `For now, this local response is using the **${model}** preset as a placeholder.`
    ].join('\n')
  }

  return [
    `You asked: **${normalized || 'Untitled prompt'}**`,
    '',
    'This Nuxt chat UI is installed and running in a desktop-friendly local mode.',
    '',
    'Next steps you could wire in from here:',
    '- connect prompt handling to a Wails Go service',
    '- persist chats in Go instead of browser storage',
    '- re-enable uploads and auth with desktop-native flows',
    '',
    `Selected model preset: **${model}**`
  ].join('\n')
}

export function useDesktopChats() {
  const chats = useState<DesktopChat[]>('desktop-chats', () => [])
  const loaded = useState<boolean>('desktop-chats-loaded', () => false)

  function sortChats() {
    chats.value = [...chats.value].sort((a, b) => {
      return new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()
    })
  }

  function load() {
    if (loaded.value || !import.meta.client) {
      return
    }

    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (raw) {
      try {
        chats.value = JSON.parse(raw) as DesktopChat[]
      } catch {
        chats.value = []
      }
    }

    sortChats()
    loaded.value = true
  }

  function persist() {
    if (!import.meta.client) {
      return
    }

    sortChats()
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(chats.value))
  }

  function getChat(id: string) {
    load()
    return chats.value.find(chat => chat.id === id)
  }

  function createChat(prompt: string) {
    load()

    const timestamp = new Date().toISOString()
    const chat: DesktopChat = {
      id: crypto.randomUUID(),
      title: makeChatTitle(prompt),
      createdAt: timestamp,
      updatedAt: timestamp,
      messages: [createTextMessage('user', prompt)]
    }

    chats.value = [chat, ...chats.value]
    persist()
    return chat
  }

  function appendUserMessage(chatId: string, prompt: string) {
    load()

    const chat = getChat(chatId)
    if (!chat) {
      return
    }

    chat.messages.push(createTextMessage('user', prompt))
    chat.title = chat.title === 'Untitled' ? makeChatTitle(prompt) : chat.title
    chat.updatedAt = new Date().toISOString()
    persist()
  }

  async function generateAssistantMessage(chatId: string, model: string) {
    load()

    const chat = getChat(chatId)
    if (!chat) {
      return
    }

    const lastUserMessage = [...chat.messages].reverse().find(message => message.role === 'user')
    const prompt = getMessageText(lastUserMessage)
    if (!prompt) {
      return
    }

    await new Promise(resolve => window.setTimeout(resolve, 450))

    chat.messages.push(createTextMessage('assistant', buildAssistantReply(prompt, model)))
    chat.updatedAt = new Date().toISOString()
    persist()
  }

  async function regenerateAssistantMessage(chatId: string, model: string) {
    load()

    const chat = getChat(chatId)
    if (!chat) {
      return
    }

    const lastMessage = chat.messages.at(-1)
    if (lastMessage?.role === 'assistant') {
      chat.messages.pop()
    }

    persist()
    await generateAssistantMessage(chatId, model)
  }

  function deleteChat(chatId: string) {
    load()
    chats.value = chats.value.filter(chat => chat.id !== chatId)
    persist()
  }

  load()

  return {
    chats,
    getChat,
    createChat,
    appendUserMessage,
    generateAssistantMessage,
    regenerateAssistantMessage,
    deleteChat
  }
}
