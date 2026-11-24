import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { selfUpdateOutcome, selfUpdateStatus } from '../stores/selfupdate';
import { endUpdateShow, logoArcsAway, resumeFinishedShow, showFollowsStatus, updateShow } from '../stores/updateShow';
import { renderDom } from '../testing/dom';
import UpdateShowHarness from '../testing/UpdateShowHarness.svelte';
import showSource from './UpdateShow.svelte?raw';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// jsdom lays nothing out and has no Web Animations. The logo and the funnel get their boxes from a
// stub; the frames come from the clock (fake), one per animation frame (fake). What a frame is made
// of is updateShowTimeline.test.ts's; here: what the player gets on screen, and when.

const START = 1_700_000_000_000;
const APPLYING = { state: 'applying', currentVersion: '1.4.0', availableVersion: '1.5.0' } as const;
const READY = { state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0', error: 'still running', errorCode: 'apply' } as const;
const box = (left: number, top: number, width: number, height: number) => ({ left, top, width, height, right: left + width, bottom: top + height, x: left, y: top, toJSON: () => ({}) }) as DOMRect;

beforeAll(() => {
  // The outcome card fades; its transitions finish at once.
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
  endUpdateShow();
  vi.restoreAllMocks();
});

const shown = () => document.querySelector<HTMLElement>('[data-testid="update-show"]');
const clones = () => [...document.querySelectorAll<HTMLElement>('[data-arc-clone]')];
const poses = () => clones().map((clone) => `${clone.style.transform} | ${clone.style.opacity}`);
const logoArcs = () => [...document.querySelectorAll<SVGPathElement>('[data-logo-origin] [data-arc]')];
const picture = () => document.querySelector('img.logo-mark') as HTMLImageElement;

async function advance(ms: number) {
  vi.advanceTimersByTime(ms);
  await tick();
}

describe('the update show while the update is applied', () => {
  it('never starts from a downloaded update', async () => {
    selfUpdateStatus.set({ state: 'ready', currentVersion: '1.4.0', availableVersion: '1.5.0' });
    renderDom(UpdateShowHarness);
    await advance(100);
    expect(shown()).toBeNull();
  });

  it('flies the five arcs off the logo over the whole window, and says once which version it installs and that the launcher restarts', async () => {
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await tick();

    const show = shown() as HTMLElement;
    expect(show.dataset.mode).toBe('flight');
    expect(clones()).toHaveLength(5);
    const dialog = screen.getByRole('dialog', { name: msg('ui.installingUpdateTitle') });
    expect(dialog).toBe(show);
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    const status = within(dialog).getAllByRole('status');
    expect(status).toHaveLength(1);
    expect(status[0].textContent).toContain(msg('ui.installingVersion', { version: '1.5.0' }));
    expect(status[0].textContent).toContain(msg('ui.launcherWillRestartShort'));
    // The logo's own arcs are away while their copies fly.
    expect(picture().classList.contains('away')).toBe(true);
    expect(logoArcs().map((arc) => arc.style.opacity)).toEqual(['0', '0', '0', '0', '0']);
  });

  it('draws the frame of the moment: on the logo at first, on the way mid-flight, gone into the funnel in the hold', async () => {
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await tick();
    expect(shown()?.dataset.stage).toBe('fly');
    expect(clones()[0].style.transform).toBe('translate(0px, 0px) rotate(0deg) scale(1, 1)');
    expect(clones().map((clone) => clone.style.opacity)).toEqual(['1', '1', '1', '1', '1']);

    await advance(600);
    expect(shown()?.dataset.stage).toBe('fly');
    expect(clones()[0].style.transform).not.toBe('translate(0px, 0px) rotate(0deg) scale(1, 1)');

    await advance(2400);
    expect(shown()?.dataset.stage).toBe('hold');
    expect(clones().map((clone) => clone.style.opacity)).toEqual(['0', '0', '0', '0', '0']);
    expect([...document.querySelectorAll('[data-funnel] [data-layer]')].map((layer) => layer.getAttribute('opacity'))).toEqual(['1', '1', '1', '1', '1']);
  });

  it('mounted late, draws the hold at once and plays no flight', async () => {
    showFollowsStatus(APPLYING, START - 10_000);
    renderDom(UpdateShowHarness);
    await tick();
    expect(shown()?.dataset.stage).toBe('hold');
    const seen: string[] = [];
    for (let i = 0; i < 20; i++) {
      seen.push(...clones().map((clone) => clone.style.opacity));
      await advance(16);
    }
    expect(new Set(seen)).toEqual(new Set(['0']));
  });

  it('draws the same thing in two mounts that share the moment the show began and the clock', async () => {
    showFollowsStatus(APPLYING, START - 700);
    const first = renderDom(UpdateShowHarness);
    await tick();
    const a = poses();
    first.unmount();
    expect(clones()).toEqual([]);

    renderDom(UpdateShowHarness);
    await tick();
    expect(poses()).toEqual(a);
    expect(a.some((pose) => !pose.startsWith('translate(0px, 0px)'))).toBe(true);
  });

  it('stays up as long as the status says applying, however long', async () => {
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(120_000);
    expect(shown()?.dataset.stage).toBe('hold');
  });

  it('sends the arcs home and leaves when applying ends without a new version', async () => {
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(3000);
    selfUpdateStatus.set(READY);
    await tick();
    expect(shown()?.dataset.stage).toBe('home');

    await advance(1000);
    expect(shown()).toBeNull();
    expect(get(updateShow)).toBeNull();
    expect(picture().classList.contains('away')).toBe(false);
    expect(logoArcs()).toEqual([]);
  });

  it('finishes when the running version changed: the arcs come back and the show leaves', async () => {
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(3000);
    selfUpdateStatus.set({ state: 'idle', currentVersion: '1.5.0' });
    await advance(100);
    expect(shown()?.dataset.stage).toBe('finish');
    await advance(3000);
    expect(shown()).toBeNull();
    expect(picture().classList.contains('away')).toBe(false);
  });
});

describe('the update show of a seamless restart', () => {
  const curtain = () => document.querySelector<HTMLElement>('[data-testid="update-show"] .curtain');
  const seamless = () => showFollowsStatus(APPLYING, START, { startedAt: START, handoff: true });

  it('closes a curtain of the page colour over the page while the funnel holds', async () => {
    seamless();
    renderDom(UpdateShowHarness);
    await tick();
    expect(curtain()?.style.opacity).toBe('0');
    await advance(1680 + 600);
    const half = Number(curtain()?.style.opacity);
    expect(half).toBeGreaterThan(0);
    expect(half).toBeLessThan(1);
    // Frames come one animation frame (16 ms) apart: the first after the closing moment is opaque.
    await advance(600 + 16);
    expect(curtain()?.style.opacity).toBe('1');
    const rule = /\n {2}\.curtain \{([^}]*)\}/.exec(showSource)?.[1] ?? '';
    expect(rule).toMatch(/background: var\(--bg\)/);
  });

  it('rolled back by Go with the curtain closed, lifts it with the dim as the arcs go home, and leaves', async () => {
    seamless();
    renderDom(UpdateShowHarness);
    await advance(4000);
    expect(curtain()?.style.opacity).toBe('1');

    selfUpdateStatus.set({ ...READY, errorCode: 'handoff_failed' });
    await advance(225);
    expect(shown()?.dataset.stage).toBe('home');
    const lifting = Number(curtain()?.style.opacity);
    expect(lifting).toBeGreaterThan(0);
    expect(lifting).toBeLessThan(1);
    await advance(1000);
    expect(shown()).toBeNull();
    expect(screen.getByRole('button', { name: 'Play' })).toBeTruthy();
  });

  it('says the launcher stays open, not that it restarts', async () => {
    seamless();
    renderDom(UpdateShowHarness);
    await tick();
    const status = screen.getByRole('status');
    expect(status.textContent).toContain(msg('ui.installingVersion', { version: '1.5.0' }));
    expect(status.textContent).toContain(msg('ui.handoffNote'));
    expect(status.textContent).not.toContain(msg('ui.launcherWillRestartShort'));
  });

  it('refused by Go and going on as an ordinary restart, opens no curtain', async () => {
    seamless();
    renderDom(UpdateShowHarness);
    showFollowsStatus(APPLYING, START + 200, { startedAt: START, handoff: false });
    await advance(5000);
    expect(curtain()?.style.opacity).toBe('0');
    expect(screen.getByRole('status').textContent).toContain(msg('ui.launcherWillRestartShort'));
  });

  it('with motion off, closes the curtain in one step when the flight would have ended', async () => {
    document.documentElement.classList.add('no-anim');
    seamless();
    renderDom(UpdateShowHarness);
    await advance(1600);
    expect(shown()?.dataset.mode).toBe('ring');
    expect(curtain()?.style.opacity).toBe('0');
    await advance(80);
    expect(curtain()?.style.opacity).toBe('1');
    // The successor has no logo to put the ring on: both windows draw it above the words.
    expect(document.querySelector('.stage .ring-box [data-ring-value]')).not.toBeNull();
    expect(document.querySelector('.ringmark')).toBeNull();
  });
});

describe('the start after an update', () => {
  it('opens on the held funnel, plays the finish, then the card says the result', async () => {
    selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
    resumeFinishedShow();
    renderDom(UpdateShowHarness);
    await tick();

    expect(shown()?.dataset.stage).toBe('finish');
    expect(clones().map((clone) => clone.style.opacity)).toEqual(['0', '0', '0', '0', '0']);
    expect(logoArcs().map((arc) => arc.style.opacity)).toEqual(['0', '0', '0', '0', '0']);
    expect(screen.queryByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();

    await advance(3000);
    expect(shown()).toBeNull();
    expect(screen.getByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeTruthy();
  });

  // The shell opens once the sign-in has settled, which for a signed-in player waits on the network:
  // the finish is timed from its first frame on screen, not from when the outcome was read.
  for (const late of [0, 900, 2500]) {
    it(`drawn ${late} ms after the outcome was read, opens on the held funnel and plays the whole finish`, async () => {
      selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
      resumeFinishedShow();
      vi.setSystemTime(START + late);
      renderDom(UpdateShowHarness);
      await tick();

      expect(shown()?.dataset.stage).toBe('finish');
      expect(shown()?.querySelector<HTMLElement>('.scrim')?.style.opacity).toBe('1');
      expect(clones().map((clone) => clone.style.opacity)).toEqual(['0', '0', '0', '0', '0']);
      expect([...document.querySelectorAll('[data-funnel] [data-layer]')].map((layer) => layer.getAttribute('opacity'))).toEqual(['1', '1', '1', '1', '1']);

      // Well into the finish: the innermost arc is back out of the funnel, the card still waits.
      await advance(450 + 200 + 4 * 120 + 500);
      expect(shown()?.dataset.stage).toBe('finish');
      expect(clones()[4].style.opacity).toBe('1');
      expect(screen.queryByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeNull();

      await advance(1500);
      expect(shown()).toBeNull();
      expect(screen.getByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeTruthy();
    });
  }

  // The sign-in can settle first: a guest, an offline start or a saved session answers in one local
  // call, the outcome after three. The steady clock is faked too: the bound is measured on it.
  const withSteadyClock = () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'Date', 'performance', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] });
    vi.setSystemTime(START);
  };

  it('read a few milliseconds after the shell appeared, plays the whole finish from its first frame over it', async () => {
    withSteadyClock();
    renderDom(UpdateShowHarness);
    await advance(30);
    expect(shown()).toBeNull();
    selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
    expect(resumeFinishedShow()).toBe(true);
    await tick();

    expect(shown()?.dataset.stage).toBe('finish');
    expect(shown()?.querySelector<HTMLElement>('.scrim')?.style.opacity).toBe('1');
    expect(clones().map((clone) => clone.style.opacity)).toEqual(['0', '0', '0', '0', '0']);
    await advance(450 + 200 + 4 * 120 + 500);
    expect(shown()?.dataset.stage).toBe('finish');
    expect(screen.queryByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeNull();
    await advance(1500);
    expect(shown()).toBeNull();
    expect(screen.getByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeTruthy();
  });

  it('read three seconds after the shell appeared, plays nothing: no dim over a page in use, the card says the result', async () => {
    withSteadyClock();
    renderDom(UpdateShowHarness);
    await advance(3000);
    selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
    const played = resumeFinishedShow();
    await tick();

    expect(shown()).toBeNull();
    expect(get(updateShow)).toBeNull();
    expect(played).toBe(false);
    expect(screen.getByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeTruthy();
  });

  it('is decoration: hidden from screen readers, takes no focus and moves none at its end', async () => {
    selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
    resumeFinishedShow();
    const { user } = renderDom(UpdateShowHarness);
    await tick();
    const show = shown() as HTMLElement;
    expect(show.getAttribute('aria-hidden')).toBe('true');
    expect(show.hasAttribute('tabindex')).toBe(false);
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(document.body);

    // Tab is not held: the page under the finish keeps its keys.
    await user.keyboard('{Tab}');
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Play' }));
    screen.getByRole('button', { name: 'Play' }).blur();

    await advance(3000);
    expect(shown()).toBeNull();
    await Promise.resolve();
    expect(document.activeElement).toBe(document.body);
  });

  it('with motion off plays nothing and the card says the result at once', async () => {
    document.documentElement.classList.add('no-anim');
    selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
    resumeFinishedShow();
    renderDom(UpdateShowHarness);
    await tick();
    expect(shown()).toBeNull();
    expect(screen.getByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeTruthy();
  });
});

// A time sync or the player setting the clock steps the wall clock back. The show is drawn on the
// wall clock (two processes must draw the same frame) but never backwards: it goes on from the last
// frame it drew at a steady pace. The steady clock is faked too, so both move with the test.
describe('the wall clock stepping back an hour', () => {
  const HOUR = 3_600_000;
  const turn = () => document.querySelector('[data-funnel] [data-layer] .dash')?.getAttribute('transform') ?? '';

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'Date', 'performance', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] });
    vi.setSystemTime(START);
  });

  it('while the funnel holds, goes on turning from where it was: no frame drawn twice, none from before', async () => {
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(3000);
    const before = poses();
    vi.setSystemTime(Date.now() - HOUR);
    await advance(100);

    const show = shown() as HTMLElement;
    expect(show.dataset.stage).toBe('hold');
    expect(show.querySelector<HTMLElement>('.scrim')?.style.opacity).toBe('1');
    expect(show.querySelector<HTMLElement>('.words')?.style.opacity).toBe('1');
    expect(poses()).toEqual(before);
    const a = turn();
    await advance(200);
    expect(turn()).not.toBe(a);
    expect(document.activeElement).toBe(show);
  });

  it('on the way home, brings the arcs back and leaves, giving focus back', async () => {
    renderDom(UpdateShowHarness);
    const play = screen.getByRole('button', { name: 'Play' });
    play.focus();
    selfUpdateStatus.set(APPLYING);
    await advance(3000);
    selfUpdateStatus.set(READY);
    await tick();
    vi.setSystemTime(Date.now() - HOUR);
    await advance(1000);

    expect(shown()).toBeNull();
    expect(get(updateShow)).toBeNull();
    await vi.waitFor(() => expect(document.activeElement).toBe(play));
  });

  it('stepped back before the way home began, still plays it rather than end at once', async () => {
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(3000);
    vi.setSystemTime(Date.now() - HOUR);
    await advance(100);
    selfUpdateStatus.set(READY);
    await advance(100);

    expect(shown()?.dataset.stage).toBe('home');
    await advance(1000);
    expect(shown()).toBeNull();
  });

  it('during the finish, brings the arcs back and leaves, and the card says the result', async () => {
    selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
    resumeFinishedShow();
    renderDom(UpdateShowHarness);
    await advance(800);
    vi.setSystemTime(Date.now() - HOUR);
    await advance(3000);

    expect(shown()).toBeNull();
    expect(screen.getByText(msg('ui.launcherUpdatedTo', { version: '1.5.0' }))).toBeTruthy();
  });

  it('with motion off, keeps the ring as full as it was', async () => {
    document.documentElement.classList.add('no-anim');
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(5000);
    const ring = () => Number(document.querySelector('[data-ring-value]')?.getAttribute('data-ring-value'));
    const full = ring();
    vi.setSystemTime(Date.now() - HOUR);
    await advance(1000);
    expect(ring()).toBeGreaterThanOrEqual(full);
  });
});

// jsdom has no ResizeObserver and lays nothing out: a stand-in observer is called by hand, and the
// boxes the stub answers are moved as a resize or a snap would move them.
describe('a window resized while the arcs fly', () => {
  const observers: { callback: () => void; watched: Set<Element>; disconnected: boolean }[] = [];

  beforeEach(() => {
    observers.length = 0;
    vi.stubGlobal(
      'ResizeObserver',
      class {
        record: { callback: () => void; watched: Set<Element>; disconnected: boolean };
        constructor(callback: () => void) {
          this.record = { callback, watched: new Set(), disconnected: false };
          observers.push(this.record);
        }
        observe(element: Element) {
          this.record.watched.add(element);
        }
        unobserve(element: Element) {
          this.record.watched.delete(element);
        }
        disconnect() {
          this.record.disconnected = true;
          this.record.watched.clear();
        }
      },
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const watched = () => observers.flatMap((observer) => [...observer.watched]);

  it('watches the logo, the funnel and the window, and flies from and to where they are now', async () => {
    const first = renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await tick();
    await advance(3000);
    expect(clones()[0].style.top).toBe('30px');
    const before = poses();

    vi.mocked(Element.prototype.getBoundingClientRect).mockImplementation(function (this: Element) {
      if (this.matches('[data-logo-origin]')) return box(20, 90, 28, 28);
      if (this.matches('.funnel-box')) return box(300, 200, 360, 277);
      return box(0, 0, 0, 0);
    });
    for (const observer of observers) observer.callback();
    // The fake animation frames fall on 16 ms marks of their own: within two of them, one is drawn.
    await advance(32);

    expect(clones()[0].style.top).toBe('90px');
    // In the funnel, every arc where its ring is now, as a window first laid out this way draws it,
    // and the show goes on from where it was.
    expect(shown()?.dataset.stage).toBe('hold');
    const resized = poses();
    expect(resized).not.toEqual(before);
    const origins = clones().map((clone) => clone.style.transformOrigin);
    first.unmount();
    renderDom(UpdateShowHarness);
    await tick();
    expect(poses()).toEqual(resized);
    expect(clones().map((clone) => clone.style.transformOrigin)).toEqual(origins);
    expect(watched().some((element) => element.matches('[data-logo-origin]'))).toBe(true);
    expect(watched().some((element) => element.matches('.funnel-box'))).toBe(true);
    expect(watched()).toContain(shown());
  });

  it('stops watching when the show ends', async () => {
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(3000);
    expect(observers.length).toBeGreaterThan(0);
    selfUpdateStatus.set(READY);
    await tick();
    await advance(1000);
    expect(shown()).toBeNull();
    expect(observers.every((observer) => observer.disconnected)).toBe(true);
  });
});

describe('the update show with motion off', () => {
  it('is a still ring on the logo with the same words, and nothing flies', async () => {
    document.documentElement.classList.add('no-anim');
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await tick();
    expect(shown()?.dataset.mode).toBe('ring');
    expect(clones()).toEqual([]);
    expect(document.querySelector('[data-funnel]')).toBeNull();
    expect(document.querySelector('.ringmark [data-ring-value]')).not.toBeNull();
    expect(screen.getByRole('status').textContent).toContain(msg('ui.installingVersion', { version: '1.5.0' }));
    expect(picture().classList.contains('away')).toBe(false);
  });

  it('switched off mid-flight, ends the motion at once: no copy left, the logo whole, the words still there', async () => {
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(600);
    expect(clones()).toHaveLength(5);

    document.documentElement.classList.add('no-anim');
    await advance(0);
    expect(clones()).toEqual([]);
    expect(document.querySelector('[data-arc-clone]')).toBeNull();
    expect(shown()?.dataset.mode).toBe('ring');
    expect(picture().classList.contains('away')).toBe(false);
    expect(screen.getByRole('status').textContent).toContain(msg('ui.installingVersion', { version: '1.5.0' }));
  });
});

describe('the update show in Big Picture (no sidebar logo)', () => {
  it('is the ring above the words, in the middle of the window', async () => {
    renderDom(UpdateShowHarness, { logo: false });
    selfUpdateStatus.set(APPLYING);
    await tick();
    expect(shown()?.dataset.mode).toBe('ring');
    expect(clones()).toEqual([]);
    expect(document.querySelector('.stage .ring-box [data-ring-value]')).not.toBeNull();
    expect(screen.getByRole('status').textContent).toContain(msg('ui.launcherWillRestartShort'));
  });

  it('at a start after an update has no logo to bring the arcs back to: whichever answers first, no dim stays', async () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'Date', 'performance', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] });
    selfUpdateOutcome.set({ version: '1.5.0', ok: true, finishedAt: '' });
    expect(resumeFinishedShow()).toBe(true);
    const outcomeFirst = renderDom(UpdateShowHarness, { logo: false });
    await tick();
    expect(shown()).toBeNull();
    expect(get(updateShow)).toBeNull();
    outcomeFirst.unmount();

    renderDom(UpdateShowHarness, { logo: false });
    await advance(30);
    expect(resumeFinishedShow()).toBe(true);
    await tick();
    expect(shown()).toBeNull();
    expect(get(updateShow)).toBeNull();
  });
});

describe('a window not laid out', () => {
  it('has no funnel to fly to: the ring stands in, and the way home still ends the show', async () => {
    vi.mocked(Element.prototype.getBoundingClientRect).mockImplementation(function (this: Element) {
      return this.matches('[data-logo-origin]') ? box(20, 30, 28, 28) : box(0, 0, 0, 0);
    });
    renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await tick();
    expect(shown()?.dataset.mode).toBe('ring');
    selfUpdateStatus.set(READY);
    await tick();
    expect(shown()).toBeNull();
  });
});

describe('the shell going while the show runs', () => {
  it('leaves no copy of an arc on the page and gives the logo its arcs back', async () => {
    const { unmount } = renderDom(UpdateShowHarness);
    selfUpdateStatus.set(APPLYING);
    await advance(600);
    unmount();
    expect(document.querySelector('[data-arc-clone]')).toBeNull();
    expect(shown()).toBeNull();
    expect(get(logoArcsAway)).toBeNull();
  });
});

describe('focus while the show is up', () => {
  it('holds focus so no key reaches the page, and gives it back to where it was', async () => {
    const { user } = renderDom(UpdateShowHarness);
    const play = screen.getByRole('button', { name: 'Play' });
    play.focus();
    selfUpdateStatus.set(APPLYING);
    await tick();
    expect(document.activeElement).toBe(shown());

    await user.keyboard('{Tab}');
    expect(document.activeElement).toBe(shown());

    selfUpdateStatus.set(READY);
    await advance(1000);
    await vi.waitFor(() => expect(document.activeElement).toBe(play));
  });

  it('gives focus to the page heading when what held it is gone', async () => {
    const { rerender } = renderDom(UpdateShowHarness);
    screen.getByRole('button', { name: 'Play' }).focus();
    selfUpdateStatus.set(APPLYING);
    await tick();
    await rerender({ page: false });
    await rerender({ page: true });

    selfUpdateStatus.set(READY);
    await advance(1000);
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Library' })));
  });

  it('gives no focus back into a part of the page that is inert when it ends: the page heading takes it', async () => {
    renderDom(UpdateShowHarness);
    const play = screen.getByRole('button', { name: 'Play' });
    play.focus();
    selfUpdateStatus.set(APPLYING);
    await tick();
    expect(document.activeElement).toBe(shown());
    play.setAttribute('inert', '');

    selfUpdateStatus.set(READY);
    await advance(1000);
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Library' })));
    expect(document.activeElement).not.toBe(play);
  });
});
