/**
 * DEV ONLY. Starts the fixture server, the BFF and Vite together, and stops all
 * three on Ctrl-C. Never copied into the image.
 *
 *   npm run dev      ->  http://127.0.0.1:5173
 */

import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const root = resolve(here, '..')

const parts = [
  ['fixtures', 'npm', ['run', 'dev:fixtures']],
  ['bff     ', 'npm', ['run', 'dev:bff']],
  ['vite    ', 'npm', ['run', 'dev:web']],
]

const children = parts.map(([label, command, args]) => {
  const child = spawn(command, args, { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] })
  const prefix = (line) => `[${label}] ${line}`
  for (const stream of [child.stdout, child.stderr]) {
    stream.setEncoding('utf8')
    let buffer = ''
    stream.on('data', (chunk) => {
      buffer += chunk
      const lines = buffer.split('\n')
      buffer = lines.pop() ?? ''
      for (const line of lines) process.stdout.write(prefix(`${line}\n`))
    })
  }
  return child
})

let stopping = false
const stop = () => {
  if (stopping) return
  stopping = true
  for (const child of children) child.kill('SIGTERM')
  setTimeout(() => process.exit(0), 500).unref()
}

process.on('SIGINT', stop)
process.on('SIGTERM', stop)
for (const child of children) child.on('exit', stop)

process.stdout.write(
  '\nEncabulation Control Console, development mode.\n' +
    '  SPA        http://127.0.0.1:5173   (Vite, proxying /bff to the BFF)\n' +
    '  BFF        http://127.0.0.1:8080\n' +
    '  fixtures   http://127.0.0.1:9099\n' +
    '  sign in    dev / dev  (admin)\n\n' +
    'The fixtures are constants in dev/fixture-server.ts, not a simulation, and\n' +
    'nothing under dev/ enters the production image.\n\n',
)
