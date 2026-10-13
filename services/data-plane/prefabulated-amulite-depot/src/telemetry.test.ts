import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

const serviceDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

// Run under plain Node ESM, as the image runs dist/index.js. Vitest's own module loader goes
// through `require` for CommonJS packages and would hide the bug this guards: an ESM `import`
// of ioredis is never seen by require-in-the-middle, so without the kit's fix the
// instrumentation patches nothing and the depot's Valkey calls produce no client spans.
const SCRIPT = `
import { startTelemetry } from '@encabulator/kit/telemetry';
startTelemetry({ serviceName: 'ioredis-patch-probe', endpoint: 'http://127.0.0.1:9' });
const { Redis } = await import('ioredis');
process.stdout.write(String(Boolean(Redis.prototype.sendCommand.__wrapped)));
process.exit(0);
`;

describe('telemetry in an ESM service', () => {
  it('patches the ioredis the service imports, so Valkey calls produce client spans', () => {
    const out = execFileSync(process.execPath, ['--input-type=module', '-e', SCRIPT], {
      cwd: serviceDir,
      encoding: 'utf8',
      timeout: 30_000,
    });
    expect(out.trim()).toBe('true');
  });
});
