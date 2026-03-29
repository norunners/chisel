const fs = require('node:fs')
const path = require('node:path')

const rootDir = path.resolve(__dirname, '..')
const publicDir = path.join(rootDir, '.output', 'public')
const distDir = path.join(rootDir, 'dist')

if (!fs.existsSync(publicDir)) {
  throw new Error(`Nuxt public output was not found at ${publicDir}`)
}

fs.rmSync(distDir, { recursive: true, force: true })
fs.mkdirSync(distDir, { recursive: true })
fs.cpSync(publicDir, distDir, { recursive: true })
