import { get } from 'svelte/store';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: vi.fn((name: string, cb: (event: { data: unknown }) => void) => {
      handlers[name] = cb;
      return () => delete handlers[name];
    }),
  },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/selfupdate')>()),
  getHandoffInfo: vi.fn(),
  handoffRendered: vi.fn(),
}));

// The successor's side of a seamless restart, as Go drives it: HandoffInfo says this launcher took
// over, HandoffRendered tells Go the first frame is on screen, launcher:handoff_done says the old
// launcher is gone. The show is held until that and the shell's first data, then finishes.

const PHASE = 1_700_000_000_000;
const ACTIVE = { active: true, phaseStart: PHASE, version: '1.5.0', background: '#101418' };
const DONE = 'launcher:handoff_done';

async function load() {
  vi.resetModules();
  for (const key of Object.keys(handlers)) delete handlers[key];
  const service = await import('../services/selfupdate');
  const handoff = await import('./handoff');
  const show = await import('./updateShow');
  const store = await import('./selfupdate');
  vi.mocked(service.handoffRendered).mockResolvedValue(undefined);
  return { service, handoff, show, store };
}

/** Animation frames the test runs by hand. */
function frames() {
  const queue: Array<() => void> = [];
  return {
    frame: (next: () => void) => queue.push(next),
    step: () => queue.splice(0).forEach((next) => next()),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('an ordinary start', () => {
  it('takes over nothing, holds no show and never tells Go a hand-over frame was drawn', async () => {
    const { service, handoff, show } = await load();
    vi.mocked(service.getHandoffInfo).mockResolvedValue({ active: false, phaseStart: 0, version: '', background: '' });

    await expect(handoff.loadHandoff()).resolves.toBeNull();
    const { frame, step } = frames();
    handoff.curtainDrawn(frame);
    step();
    step();

    expect(get(show.updateShow)).toBeNull();
    expect(service.handoffRendered).not.toHaveBeenCalled();
    expect(handlers[DONE]).toBeUndefined();
  });

  it('with Go not answering, is an ordinary start', async () => {
    const { service, handoff, show } = await load();
    vi.mocked(service.getHandoffInfo).mockRejectedValue(new Error('boom'));
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {});

    await expect(handoff.loadHandoff()).resolves.toBeNull();

    expect(get(show.updateShow)).toBeNull();
    expect(quiet).toHaveBeenCalled();
    quiet.mockRestore();
  });
});

describe('a successor', () => {
  it('goes on with the old launcher\'s show on its clock, held, whatever its own status says', async () => {
    const { service, handoff, show, store } = await load();
    vi.mocked(service.getHandoffInfo).mockResolvedValue(ACTIVE);

    await expect(handoff.loadHandoff()).resolves.toEqual(ACTIVE);
    expect(get(show.updateShow)).toMatchObject({ startedAt: PHASE, phase: 'run', version: '1.5.0', handoff: true, held: true });

    // Go's status in a successor: idle on the new version, nothing being applied.
    store.selfUpdateStatus.set({ state: 'idle', currentVersion: '1.5.0' });
    expect(get(show.updateShow)?.phase).toBe('run');
  });

  it('tells Go once, two animation frames after the closed curtain was drawn', async () => {
    const { service, handoff } = await load();
    vi.mocked(service.getHandoffInfo).mockResolvedValue(ACTIVE);
    await handoff.loadHandoff();
    const { frame, step } = frames();

    handoff.curtainDrawn(frame);
    handoff.curtainDrawn(frame);
    expect(service.handoffRendered).not.toHaveBeenCalled();
    step();
    expect(service.handoffRendered).not.toHaveBeenCalled();
    step();
    expect(service.handoffRendered).toHaveBeenCalledOnce();

    handoff.curtainDrawn(frame);
    step();
    step();
    expect(service.handoffRendered).toHaveBeenCalledOnce();
  });

  it('keeps going when Go cannot show the window: Go rolls back on its own clock', async () => {
    const { service, handoff } = await load();
    vi.mocked(service.getHandoffInfo).mockResolvedValue(ACTIVE);
    vi.mocked(service.handoffRendered).mockRejectedValue(new Error('selfupdate: no window to reveal'));
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {});
    await handoff.loadHandoff();
    const { frame, step } = frames();

    handoff.curtainDrawn(frame);
    step();
    step();

    await vi.waitFor(() => expect(quiet).toHaveBeenCalled());
    quiet.mockRestore();
  });

  it.each([
    ['the old launcher goes first', ['done', 'shell']],
    ['the shell is ready first', ['shell', 'done']],
  ])('finishes the show and says the result only when the old launcher is gone and the shell has its data (%s)', async (_name, order) => {
    const { service, handoff, show, store } = await load();
    vi.mocked(service.getHandoffInfo).mockResolvedValue(ACTIVE);
    await handoff.loadHandoff();

    const [first, second] = order;
    const happen = (what: string) => (what === 'done' ? handlers[DONE]({ data: { ...ACTIVE, active: false } }) : handoff.setShellReady(true));
    happen(first);
    expect(get(show.updateShow)?.phase).toBe('run');
    expect(get(store.selfUpdateOutcome)).toBeNull();

    happen(second);
    expect(get(show.updateShow)).toMatchObject({ startedAt: PHASE, phase: 'finish' });
    expect(get(show.updateShow)?.held).toBeFalsy();
    expect(get(store.selfUpdateOutcome)).toMatchObject({ version: '1.5.0', ok: true });
  });

  it('a shell that loses its data again does not take the show back', async () => {
    const { service, handoff, show } = await load();
    vi.mocked(service.getHandoffInfo).mockResolvedValue(ACTIVE);
    await handoff.loadHandoff();
    handoff.setShellReady(true);
    handlers[DONE]({ data: { ...ACTIVE, active: false } });

    handoff.setShellReady(false);
    handoff.setShellReady(true);

    expect(get(show.updateShow)?.phase).toBe('finish');
  });
});
