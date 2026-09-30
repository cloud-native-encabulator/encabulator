/**
 * Testcontainers helper. Integration tests skip cleanly without a container runtime
 * (design section 7, "Testing"). Locally, `source dev-env.sh` points DOCKER_HOST at Finch.
 */

import { createServer } from 'node:net';
import Dockerode from 'dockerode';
import { GenericContainer, type StartedTestContainer, Wait } from 'testcontainers';
import { describe } from 'vitest';

export const containersAvailable = Boolean(process.env.DOCKER_HOST || process.env.CI_DOCKER);

/**
 * Finch reports `Labels: null` for containers it did not create, and Testcontainers reads
 * `container.Labels[...]` unconditionally while it hunts for a reaper. One unrelated
 * container in the VM would otherwise fail every integration test in the repository, so
 * normalise the list response once, here.
 */
let labelsPatched = false;
function patchDockerodeLabels(): void {
  if (labelsPatched) return;
  labelsPatched = true;
  const proto = Dockerode.prototype as unknown as {
    listContainers: (...args: unknown[]) => Promise<Array<{ Labels?: unknown }>>;
  };
  const original = proto.listContainers;
  proto.listContainers = async function patched(...args: unknown[]) {
    const containers = await original.apply(this, args);
    for (const container of containers ?? []) {
      if (container.Labels == null) container.Labels = {};
    }
    return containers;
  };
}

export const describeWithContainers: typeof describe.skip = containersAvailable
  ? describe
  : describe.skip;

/**
 * Pick a concrete free host port: Finch's CNI portmap plugin rejects the host-port-0 that
 * Testcontainers would otherwise ask the runtime for.
 */
export async function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      const port = typeof address === 'object' && address ? address.port : 0;
      server.close(() => (port ? resolve(port) : reject(new Error('no free port'))));
    });
  });
}

export interface StartedNats {
  url: string;
  container: StartedTestContainer;
}

/** NATS 2.12 with JetStream, the version in the design's backing-store table. */
export async function startNats(): Promise<StartedNats> {
  patchDockerodeLabels();
  const host = await freePort();
  const container = await new GenericContainer('nats:2.12')
    .withCommand(['-js', '-sd', '/tmp/nats'])
    .withExposedPorts({ container: 4222, host })
    .withWaitStrategy(Wait.forListeningPorts())
    .withStartupTimeout(90_000)
    .start();
  return { url: `nats://127.0.0.1:${host}`, container };
}
