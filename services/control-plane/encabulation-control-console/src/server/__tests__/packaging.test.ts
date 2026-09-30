/**
 * Packaging guards.
 *
 * The brief puts two hard constraints on the image: the dev-only fixtures under
 * `dev/` must never enter it, and no source maps may ship. Both are properties
 * of the Dockerfile and the build config, so both are asserted here rather than
 * left to a reviewer noticing.
 */

import { readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const ROOT = resolve(process.cwd())
const dockerfile = readFileSync(join(ROOT, 'Dockerfile'), 'utf8')
const packageJson = JSON.parse(readFileSync(join(ROOT, 'package.json'), 'utf8')) as {
  dependencies: Record<string, string>
  devDependencies: Record<string, string>
  scripts: Record<string, string>
}

/** Every path the Dockerfile copies out of the build context. */
function copiedPaths(): string[] {
  const out: string[] = []
  // Join continuation lines so a multi-line COPY is read as one instruction.
  const joined = dockerfile.replace(/\\\r?\n\s*/g, ' ')
  for (const line of joined.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed.startsWith('COPY ')) continue
    // Skip stage-to-stage copies: those move build output, not source.
    if (/--from=/.test(trimmed)) continue
    const args = trimmed
      .slice('COPY '.length)
      .split(/\s+/)
      .filter((a) => a !== '' && !a.startsWith('--'))
    // The last argument is the destination.
    out.push(...args.slice(0, -1))
  }
  return out
}

describe('the Dockerfile', () => {
  it('copies explicit paths, never the whole service directory', () => {
    const paths = copiedPaths()
    expect(paths.length).toBeGreaterThan(4)
    for (const path of paths) {
      expect(path, 'a bare "." or "./" copy would drag dev/ in').not.toMatch(/^\.\/?$/)
    }
  })

  it('never copies anything under dev/', () => {
    const offenders = copiedPaths().filter((p) => /(^|\/)dev\//.test(p))
    expect(offenders).toEqual([])
  })

  it('never copies the test, lint or vitest configuration', () => {
    const offenders = copiedPaths().filter((p) =>
      /(eslint\.config|vitest\.config|tsconfig\.dev|tsconfig\.test)/.test(p),
    )
    expect(offenders).toEqual([])
  })

  it('builds from the repository root, so the shared kit can be copied in', () => {
    expect(dockerfile).toContain('libs/ts/kit')
    expect(dockerfile).toContain('services/control-plane/encabulation-control-console/package.json')
  })

  it('runs as UID and GID 65532', () => {
    expect(dockerfile).toMatch(/^USER 65532:65532$/m)
  })

  it('declares no HEALTHCHECK, because Kubernetes probes own health', () => {
    expect(dockerfile).not.toMatch(/^HEALTHCHECK/m)
  })

  it('deletes any source map the build produced', () => {
    expect(dockerfile).toContain("find dist -name '*.map' -delete")
  })

  it('runs prune so the runtime tree carries no build tooling', () => {
    expect(dockerfile).toContain('npm prune --omit=dev')
  })
})

describe('the build configuration', () => {
  it('turns Vite source maps off', () => {
    const vite = readFileSync(join(ROOT, 'vite.config.ts'), 'utf8')
    expect(vite).toMatch(/sourcemap:\s*false/)
  })

  it('turns TypeScript source maps and declarations off', () => {
    const base = readFileSync(join(ROOT, 'tsconfig.json'), 'utf8')
    expect(base).toMatch(/"sourceMap":\s*false/)
    expect(base).toMatch(/"declaration":\s*false/)
  })

  it('keeps the SPA toolchain out of runtime dependencies', () => {
    // vue, vite, pinia, d3 and friends are bundled at build time; shipping them
    // in the image would be dead weight after `npm prune --omit=dev`.
    for (const build of ['vue', 'vite', 'pinia', 'vue-router', 'd3', 'tailwindcss', 'typescript']) {
      expect(Object.keys(packageJson.dependencies), build).not.toContain(build)
      expect(Object.keys(packageJson.devDependencies), build).toContain(build)
    }
  })

  it('depends on the shared kit as a file: dependency, as the design requires', () => {
    expect(packageJson.dependencies['@encabulator/kit']).toBe('file:../../../libs/ts/kit')
  })

  it('pins @kubernetes/client-node to the 1.x object-style API', () => {
    expect(packageJson.dependencies['@kubernetes/client-node']).toMatch(/^\^?1\./)
  })

  it('fails lint on warnings, so "lint clean" means clean', () => {
    expect(packageJson.scripts['lint']).toContain('--max-warnings 0')
  })
})
