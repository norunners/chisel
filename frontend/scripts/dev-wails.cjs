const net = require('node:net')
const { spawn } = require('node:child_process')

const rawPort = process.argv[2]
const port = Number.parseInt(rawPort || '', 10)

if (!Number.isInteger(port) || port <= 0) {
  console.error(`Invalid port: ${rawPort || '(missing)'}`)
  process.exit(1)
}

const probe = net.createServer()

probe.once('error', (error) => {
  if (error && error.code === 'EADDRINUSE') {
    console.error(`Port ${port} is already in use. Stop the existing frontend dev server and retry.`)
    process.exit(1)
  }

  console.error(error)
  process.exit(1)
})

probe.once('listening', () => {
  probe.close(() => {
    const command = process.platform === 'win32' ? 'pnpm.cmd' : 'pnpm'
    const child = spawn(command, ['exec', 'nuxt', 'dev', '--port', String(port)], {
      cwd: process.cwd(),
      stdio: 'inherit'
    })

    child.on('exit', (code, signal) => {
      if (signal) {
        process.kill(process.pid, signal)
        return
      }

      process.exit(code ?? 0)
    })
  })
})

probe.listen(port)
