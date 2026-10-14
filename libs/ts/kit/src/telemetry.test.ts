import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { afterEach, describe, expect, it } from 'vitest';
import { loadRequirePatchedModules, markValkey, REQUIRE_PATCHED_MODULES } from './telemetry.js';

describe('Valkey client spans', () => {
  it('name the store valkey and keep the protocol as db.system=redis', () => {
    const set: Record<string, unknown> = {};
    markValkey({
      setAttribute(key, value) {
        set[key] = value;
        return this as never;
      },
    });
    expect(set).toEqual({ 'db.system': 'redis', 'db.system.name': 'valkey' });
  });
});

describe('loading the require-patched modules', () => {
  let dir: string | null = null;
  afterEach(() => {
    if (dir !== null) rmSync(dir, { recursive: true, force: true });
    dir = null;
  });

  it('covers ioredis', () => {
    expect(REQUIRE_PATCHED_MODULES).toContain('ioredis');
  });

  it('skips a module the service does not depend on', () => {
    dir = mkdtempSync(path.join(tmpdir(), 'kit-no-ioredis-'));
    expect(loadRequirePatchedModules(path.join(dir, 'index.js'))).toEqual([]);
  });

  it('requires the module from the service directory when it is there', () => {
    dir = mkdtempSync(path.join(tmpdir(), 'kit-with-ioredis-'));
    const pkg = path.join(dir, 'node_modules', 'ioredis');
    mkdirSync(pkg, { recursive: true });
    writeFileSync(path.join(pkg, 'package.json'), '{"name":"ioredis","main":"index.js"}');
    writeFileSync(path.join(pkg, 'index.js'), 'globalThis.__kitFakeIoredisLoaded = true;');
    expect(loadRequirePatchedModules(path.join(dir, 'index.js'))).toEqual(['ioredis']);
    expect((globalThis as { __kitFakeIoredisLoaded?: boolean }).__kitFakeIoredisLoaded).toBe(true);
  });
});
