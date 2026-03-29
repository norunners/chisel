export default defineAppConfig({
  ui: {
    colors: {
      primary: 'blue',
      neutral: 'zinc'
    },
    toast: {
      slots: {
        title: 'text-sm font-medium text-highlighted select-text',
        description: 'text-sm text-muted select-text whitespace-pre-wrap break-words'
      }
    }
  }
})
