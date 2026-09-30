import { Writable } from 'node:stream';
import { describe, expect, it } from 'vitest';
import { createLogger } from './logger.js';

function capture(): { stream: Writable; lines: () => Record<string, unknown>[] } {
  const chunks: string[] = [];
  const stream = new Writable({
    write(chunk, _encoding, callback) {
      chunks.push(String(chunk));
      callback();
    },
  });
  return {
    stream,
    lines: () =>
      chunks
        .join('')
        .split('\n')
        .filter((l) => l.trim().length > 0)
        .map((l) => JSON.parse(l) as Record<string, unknown>),
  };
}

describe('createLogger', () => {
  it('emits the required fields in the required formats', () => {
    const sink = capture();
    const logger = createLogger({ service: 'panametric-fam-processor', destination: sink.stream });
    logger.info({ orderId: 'ord_1' }, 'order accepted');

    const line = sink.lines()[0];
    expect(line).toBeDefined();
    expect(line?.level).toBe('info');
    expect(line?.msg).toBe('order accepted');
    expect(line?.service).toBe('panametric-fam-processor');
    expect(line?.orderId).toBe('ord_1');
    // RFC 3339 with milliseconds.
    expect(String(line?.time)).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/);
  });

  it('writes level names rather than numbers', () => {
    const sink = capture();
    const logger = createLogger({ service: 's', level: 'debug', destination: sink.stream });
    logger.warn('side fumbling detected');
    logger.debug('grammeters synchronized');
    expect(sink.lines().map((l) => l.level)).toEqual(['warn', 'debug']);
  });

  it('respects the level', () => {
    const sink = capture();
    const logger = createLogger({ service: 's', level: 'warn', destination: sink.stream });
    logger.info('not emitted');
    logger.error('emitted');
    expect(sink.lines()).toHaveLength(1);
  });

  it('redacts secrets rather than logging them', () => {
    const sink = capture();
    const logger = createLogger({ service: 's', destination: sink.stream });
    logger.info({ password: 'hunter2', accessToken: 'eyJ...' }, 'login');
    const line = sink.lines()[0];
    expect(line?.password).toBe('[redacted]');
    expect(line?.accessToken).toBe('[redacted]');
  });

  it('omits trace ids when no span is active', () => {
    const sink = capture();
    const logger = createLogger({ service: 's', destination: sink.stream });
    logger.info('no span here');
    expect(sink.lines()[0]).not.toHaveProperty('trace_id');
  });
});
