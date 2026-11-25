import { screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { getHandoffInfo, handoffRendered } from '../services/selfupdate';
import { loadHandoff, setShellReady } from '../stores/handoff';
import { selfUpdateOutcome, selfUpdateStatus } from '../stores/selfupdate';
import { endUpdateShow, logoArcsAway, resumeFinishedShow, showFollowsStatus, updateShow } from '../stores/updateShow';
import { renderDom } from '../testing/dom';
import UpdateShowHarness from '../testing/UpdateShowHarness.svelte';

const handlers: Record<string, (event: { data: unknown }) => void> = {};

vi.mock('@wailsio/runtime', async () => {
  const runtime = (await import('../testing/dom')).wailsRuntime();
  runtime.Events.On = vi.fn((name: string | undefined, cb: (event: unknown) => void) => {
    if (name) handlers[name] = cb as (event: { data: unknown }) => void;
    return () => {};
  });
  return runtime;
});
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/selfupdate', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/selfupdate')>()),
  getHandoffInfo: vi.fn(),
  handoffRendered: vi.fn(async () => {}),
}));

// A seamless restart seen from both windows. The old one holds the funnel over a curtain that closes
// to the page colour; the successor, started hidden with no shell yet, must draw the very same frame
// from the moment the show began, tell Go only once that frame is on screen with the curtain closed,
// and play the finish only once the old launcher is gone and its shell has its first data.
// jsdom lays nothing out: the logo and the funnel get their boxes from a stub, the clock and the
// animation frames are fake.

const START = 1_700_000_000_000;
const CLOSED = 2880; // the flight (4 × 120 + 1200) and one more flight's time for the curtain
const APPLYING = { state: 'applying', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;
const box = (left: number, top: number, width: number, height: number) => ({ left, top, width, height, right: left + width, bottom: top + height, x: left, y: top, toJSON: () => ({}) }) as DOMRect;

beforeAll(() => {
  const finished = () => ({
    cancel: () => {},
    set onfinish(done: () => void) {
      done();
    },
  });
  Object.defineProperty(Element.prototype, 'animate', { value: finished, configurable: true, writable: true });
});

afterAll(() => {
  Reflect.deleteProperty(Element.prototype, 'animate');
});

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] });
  vi.setSystemTime(START);
  vi.mocked(handoffRendered).mockClear();
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
    if (this.matches('[data-logo-origin]')) return box(20, 30, 28, 28);
    if (this.matches('.funnel-box')) return box(500, 300, 360, 277);
    return box(0, 0, 0, 0);
  });
});

afterEach(() => {
  document.documentElement.classList.remove('no-anim');
  selfUpdateStatus.set({ state: 'idle', currentVersion: '' });
  selfUpdateOutcome.set(null);
  setShellReady(false);
  endUpdateShow();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

async function advance(ms: number) {
  vi.advanceTimersByTime(ms);
  await tick();
}

async function becomeSuccessor(phaseStart: number) {
  vi.mocked(getHandoffInfo).mockResolvedValue({ active: true, phaseStart, version: '1.5.0', background: '#090d17' });
  expect(await loadHandoff()).not.toBeNull();
}

/**
 * Go is told once the frame with the closed curtain is on screen: not within the first animation
 * frame after it was drawn, and within a few. The fake frames fall on 16 ms marks, so where the
 * second one lands depends on when the first was asked for.
 */
async function reportedAfterTwoFrames() {
  expect(handoffRendered).not.toHaveBeenCalled();
  await advance(16);
  expect(handoffRendered).not.toHaveBeenCalled();
  for (let i = 0; i < 3 && vi.mocked(handoffRendered).mock.calls.length === 0; i++) await advance(16);
  expect(handoffRendered).toHaveBeenCalledOnce();
}

const shown = () => document.querySelector<HTMLElement>('[data-testid="update-show"]');

/** What a person sees of the show: its parts' look, never how they got there. */
function picture() {
  const show = shown();
  const look = (selector: string) => show?.querySelector<HTMLElement>(selector)?.style.opacity ?? null;
  return {
    mode: show?.dataset.mode,
    stage: show?.dataset.stage,
    scrim: look('.scrim'),
    curtain: look('.curtain'),
    funnel: [...(show?.querySelectorAll('[data-funnel] [data-layer]') ?? [])].map((layer) => layer.outerHTML),
    ring: show?.querySelector('[data-ring-value]')?.outerHTML ?? null,
    ringPlace: show?.querySelector('[data-ring-value]')?.closest('.ring-box, .ringmark')?.className.split(' ')[0] ?? null,
    words: show?.querySelector('[role="status"]')?.textContent?.trim() ?? null,
  };
}

describe('the moment the successor takes the old window\'s place', () => {
  it('draws what the old window draws at that moment: the same curtain, funnel at the same angle, and words', async () => {
    const began = START - 4000;
    showFollowsStatus(APPLYING, began, { startedAt: began, handoff: true });
    const old = renderDom(UpdateShowHarness);
    await tick();
    const seen = picture();
    old.unmount();
    endUpdateShow();

    await becomeSuccessor(began);
    renderDom(UpdateShowHarness, { logo: false, page: false });
    await tick();

    expect(seen.curtain).toBe('1');
    expect(seen.stage).toBe('hold');
    expect(seen.funnel).toHaveLength(5);
    expect(picture()).toEqual(seen);
  });

  it('with motion off, draws the same ring and words on the same closed curtain', async () => {
    document.documentElement.classList.add('no-anim');
    const began = START - 4000;
    showFollowsStatus(APPLYING, began, { startedAt: began, handoff: true });
    const old = renderDom(UpdateShowHarness);
    await tick();
    const seen = picture();
    old.unmount();
    endUpdateShow();

    await becomeSuccessor(began);
    renderDom(UpdateShowHarness, { logo: false, page: false });
    await tick();

    expect(seen).toMatchObject({ mode: 'ring', curtain: '1' });
    expect(seen.ring).not.toBeNull();
    expect(picture()).toEqual(seen);
  });
});

describe('the colour of the curtain', () => {
  // The old window hands over its page colour; the successor's theme may resolve --bg to another one.
  const curtainColour = () => shown()?.querySelector<HTMLElement>('.curtain')?.style.backgroundColor ?? null;

  it('in a successor is the colour handed over, while held and while it lifts off the page', async () => {
    vi.mocked(getHandoffInfo).mockResolvedValue({ active: true, phaseStart: START - 10_000, version: '1.5.0', background: '#123456' });
    expect(await loadHandoff()).not.toBeNull();
    renderDom(UpdateShowHarness, { logo: true, page: true });
    await advance(48);
    expect(shown()?.querySelector<HTMLElement>('.curtain')?.style.opacity).toBe('1');
    expect(curtainColour()).toBe('rgb(18, 52, 86)');

    setShellReady(true);
    handlers['launcher:handoff_done']({ data: { active: false, phaseStart: START - 10_000, version: '1.5.0', background: '#123456' } });
    await advance(100);
    expect(shown()?.dataset.stage).toBe('finish');
    expect(curtainColour()).toBe('rgb(18, 52, 86)');
  });

  it('handed something that is not a colour, is left to the page token', async () => {
    vi.mocked(getHandoffInfo).mockResolvedValue({ active: true, phaseStart: START - 10_000, version: '1.5.0', background: 'url(x)' });
    expect(await loadHandoff()).not.toBeNull();
    renderDom(UpdateShowHarness, { logo: false, page: false });
    await advance(48);
    expect(shown()?.querySelector<HTMLElement>('.curtain')?.style.opacity).toBe('1');
    expect(curtainColour()).toBe('');
  });

  it('in the old window and in an ordinary show is left to the page token', async () => {
    showFollowsStatus(APPLYING, START - 10_000, { startedAt: START - 10_000, handoff: true });
    renderDom(UpdateShowHarness);
    await tick();
    expect(shown()?.querySelector<HTMLElement>('.curtain')?.style.opacity).toBe('1');
    expect(curtainColour()).toBe('');
  });
});

describe('telling Go the successor may be shown', () => {
  it('waits for the curtain to close, then two animation frames, and tells Go once', async () => {
    await becomeSuccessor(START - 1000);
    renderDom(UpdateShowHarness, { logo: false, page: false });
    await advance(CLOSED - 1000 - 40);
    expect(Number(shown()?.querySelector<HTMLElement>('.curtain')?.style.opacity)).toBeLessThan(1);
    expect(handoffRendered).not.toHaveBeenCalled();

    await advance(60);
    expect(shown()?.querySelector<HTMLElement>('.curtain')?.style.opacity).toBe('1');
    await reportedAfterTwoFrames();

    await advance(5000);
    expect(handoffRendered).toHaveBeenCalledOnce();
  });

  it('mounted with the curtain already closed, tells Go two frames after its first', async () => {
    await becomeSuccessor(START - 10_000);
    renderDom(UpdateShowHarness, { logo: false, page: false });
    await tick();
    await reportedAfterTwoFrames();
  });

  it('with motion off, tells Go once the still curtain has closed', async () => {
    document.documentElement.classList.add('no-anim');
    await becomeSuccessor(START - 1000);
    renderDom(UpdateShowHarness, { logo: false, page: false });
    await advance(600);
    expect(handoffRendered).not.toHaveBeenCalled();
    // The step lands at 680 ms.
    await advance(200);
    expect(shown()?.querySelector<HTMLElement>('.curtain')?.style.opacity).toBe('1');
    await reportedAfterTwoFrames();
  });

  it('never does in an ordinary start, nor at the finish after an ordinary restart', async () => {
    vi.mocked(getHandoffInfo).mockResolvedValue({ active: false, phaseStart: 0, version: '', background: '' });
    expect(await loadHandoff()).toBeNull();
    resumeFinishedShow();
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(10_000);
    expect(handoffRendered).not.toHaveBeenCalled();
  });
});

describe('after the hand-over', () => {
  it('keeps the curtain closed until the old launcher is gone and the shell has its data, then finishes, says the result and gives focus to the page heading', async () => {
    await becomeSuccessor(START - 10_000);
    const { rerender } = renderDom(UpdateShowHarness, { logo: false, page: false });
    await advance(48);
    expect(document.activeElement).toBe(shown());

    // The shell mounts under the curtain once its sign-in is settled; the old launcher is still there.
    await rerender({ logo: true, page: true });
    setShellReady(true);
    await advance(2000);
    expect(shown()?.querySelector<HTMLElement>('.curtain')?.style.opacity).toBe('1');
    expect(get(logoArcsAway)).toEqual([true, true, true, true, true]);
    expect(screen.queryByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeNull();

    handlers['launcher:handoff_done']({ data: { active: false, phaseStart: START - 10_000, version: '1.5.0', background: '#090d17' } });
    await advance(100);
    expect(shown()?.dataset.stage).toBe('finish');
    await advance(3000);
    expect(shown()).toBeNull();
    expect(get(updateShow)).toBeNull();
    expect(get(logoArcsAway)).toBeNull();
    expect(screen.getByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeTruthy();
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Library' })));
  });

  it('holds the page out of reach while the curtain is closed: Tab stays on the show', async () => {
    await becomeSuccessor(START - 10_000);
    const { user } = renderDom(UpdateShowHarness, { logo: true, page: true });
    await advance(48);
    await user.keyboard('{Tab}');
    expect(document.activeElement).toBe(shown());
  });
});
