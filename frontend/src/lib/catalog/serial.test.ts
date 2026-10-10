import { describe, expect, it } from 'vitest';
import { latestRunner, serialQueue } from './serial';

function gate() {
  let open!: () => void;
  const wait = new Promise<void>((resolve) => { open = resolve; });
  return { wait, open };
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

describe('latestRunner', () => {
  it('ends a burst of triggers on the state seen by one follow-up run', async () => {
    let state = 'Action';
    const seen: string[] = [];
    const first = gate();
    let calls = 0;
    const trigger = latestRunner(async () => {
      calls++;
      seen.push(state);
      if (calls === 1) await first.wait;
    });

    const done = trigger();
    for (const genre of ['Adventure', 'Arcade', 'Indie', 'Racing']) {
      state = genre;
      void trigger();
    }
    first.open();
    await done;

    expect(seen).toEqual(['Action', 'Racing']);
  });

  it('runs again for a trigger that arrives after the previous run finished', async () => {
    let calls = 0;
    const trigger = latestRunner(async () => { calls++; });
    await trigger();
    await trigger();
    expect(calls).toBe(2);
  });

  it('drops the follow-up after a failed run and accepts the next trigger', async () => {
    let calls = 0;
    const first = gate();
    const trigger = latestRunner(async () => {
      calls++;
      if (calls === 1) {
        await first.wait;
        throw new Error('save failed');
      }
    });
    const failed = trigger();
    void trigger();
    first.open();
    await expect(failed).rejects.toThrow('save failed');
    expect(calls).toBe(1);
    await trigger();
    expect(calls).toBe(2);
  });
});

describe('serialQueue', () => {
  it('never overlaps tasks and keeps call order across a rejection', async () => {
    const queue = serialQueue();
    const log: string[] = [];
    const slow = gate();
    const a = queue(async () => { log.push('a start'); await slow.wait; log.push('a end'); throw new Error('a'); });
    const b = queue(async () => { log.push('b'); return 2; });
    await tick();
    expect(log).toEqual(['a start']);
    slow.open();
    await expect(a).rejects.toThrow('a');
    await expect(b).resolves.toBe(2);
    expect(log).toEqual(['a start', 'a end', 'b']);
  });
});
