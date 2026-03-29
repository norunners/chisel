// @ts-check
import withNuxt from './.nuxt/eslint.config.mjs'

const config = withNuxt({
  rules: {
    'vue/multi-word-component-names': 'off',
    'vue/max-attributes-per-line': ['error', { singleline: 3 }]
  }
})

config.ignores = [
  '**/bindings/**'
]

export default config
