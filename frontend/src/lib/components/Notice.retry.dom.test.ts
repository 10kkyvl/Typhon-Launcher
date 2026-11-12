import { render, screen, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale } from '../i18n';
import { renderDom, user as keys } from '../testing/dom';
import RetryDialogHarness from '../testing/RetryDialogHarness.svelte';
import RetryHarness from '../testing/RetryHarness.svelte';

const MESSAGE = 'The list could not be loaded';

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

const frame = async () => {
  await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  await tick();
};

// What a screen reader is told: words that come into a live region after it is on the page. Each
// entry is the text that arrived in one change.
function listen(region: Element): { heard: string[]; stop: () => void } {
  const heard: string[] = [];
  const observer = new MutationObserver((records) => {
    for (const record of records) {
      if (record.type === 'characterData' && record.target.textContent) heard.push(record.target.textContent.trim());
      for (const node of record.addedNodes) if (node.textContent?.trim()) heard.push(node.textContent.trim());
    }
  });
  observer.observe(region, { subtree: true, childList: true, characterData: true });
  return { heard, stop: () => observer.disconnect() };
}

const retryButton = () => screen.getByRole('button', { name: 'Retry' });

beforeEach(() => {
  locale.set('en');
});

afterEach(() => {
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe('Notice with a retry: while it runs', () => {
  it('stays on screen with its Retry busy and focused; a second press does nothing', async () => {
    const answer = deferred<boolean>();
    const run = vi.fn(() => answer.promise);
    const { user } = renderDom(RetryHarness, { run });
    const retry = retryButton();
    retry.focus();

    await user.keyboard('{Enter}');
    await user.click(retry);
    await user.keyboard(' ');

    expect(run).toHaveBeenCalledTimes(1);
    expect(retry.isConnected).toBe(true);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    expect(screen.getByRole('alert').textContent).toContain(MESSAGE);
    answer.resolve(false);
    await waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
  });

  it('keeps the Retry out of the live region', () => {
    renderDom(RetryHarness, { run: vi.fn(async () => false) });
    expect(screen.getByRole('alert').contains(retryButton())).toBe(false);
  });
});

describe('Notice with a retry: a failure', () => {
  it('is announced again, a second time too, though the words are the same', async () => {
    const { user } = renderDom(RetryHarness, { run: vi.fn(async () => false) });
    const region = screen.getByRole('alert');
    const { heard, stop } = listen(region);

    await user.click(retryButton());
    await frame();
    await frame();
    await user.click(retryButton());
    await frame();
    await frame();
    stop();

    expect(heard.filter((words) => words.includes(MESSAGE))).toHaveLength(2);
    expect(document.activeElement).toBe(retryButton());
    expect(retryButton().hasAttribute('aria-busy')).toBe(false);
  });

  it('announces the words the caller changed them to', async () => {
    const view = renderDom(RetryHarness, { run: vi.fn(async () => false), message: MESSAGE });
    const run = vi.fn(async () => {
      await view.rerender({ message: 'The service did not answer', run });
      return false;
    });
    await view.rerender({ run });
    const { heard, stop } = listen(screen.getByRole('alert'));

    await view.user.click(retryButton());
    await frame();
    await frame();
    stop();

    expect(heard.at(-1)).toBe('The service did not answer');
  });

  it('reads the message once in the page, not once per failure', async () => {
    const { user } = renderDom(RetryHarness, { run: vi.fn(async () => false) });
    await user.click(retryButton());
    await frame();
    await frame();
    await user.click(retryButton());
    await frame();
    await frame();

    const exposed = [...screen.getByRole('alert').querySelectorAll('*')]
      .filter((el) => !el.closest('[aria-hidden="true"]') && el.children.length === 0)
      .map((el) => el.textContent?.trim())
      .filter((words) => words);
    expect(exposed).toEqual([MESSAGE]);
  });

  // What a screen reader can reach: the text outside anything aria-hidden.
  const exposed = (root: Element) =>
    [...root.querySelectorAll('*')]
      .filter((el) => !el.closest('[aria-hidden="true"]') && el.children.length === 0)
      .map((el) => el.textContent?.trim())
      .filter((words) => words);

  it('holds the words on screen in the tree at every moment: after a failure, while it runs, when they change', async () => {
    const view = renderDom(RetryHarness, { run: vi.fn(async () => false), message: MESSAGE });
    await view.user.click(retryButton());
    await frame();
    await frame();

    await view.rerender({ message: 'Disk is full' });
    expect(exposed(screen.getByRole('alert'))).toEqual(['Disk is full']);

    const answer = deferred<boolean>();
    await view.rerender({ run: vi.fn(() => answer.promise) });
    await view.user.click(retryButton());
    expect(exposed(screen.getByRole('alert'))).toEqual(['Disk is full']);
    answer.resolve(false);
    await frame();
    await frame();
    expect(exposed(screen.getByRole('alert'))).toEqual(['Disk is full']);
  });

  it('keeps a titled notice heading a heading after a failure', async () => {
    const { user } = renderDom(RetryHarness, { run: vi.fn(async () => false), title: 'Emulators unavailable', level: 2 });
    await user.click(retryButton());
    await frame();
    await frame();
    expect(screen.getByRole('heading', { level: 2, name: 'Emulators unavailable' })).toBeTruthy();
  });

  it('a thrown error is a failure: the notice stays and says so again, and the error is logged', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { user } = renderDom(RetryHarness, { run: vi.fn(async () => Promise.reject(new Error('down'))) });
    const { heard, stop } = listen(screen.getByRole('alert'));

    await user.click(retryButton());
    await frame();
    await frame();
    stop();

    expect(heard.filter((words) => words.includes(MESSAGE))).toHaveLength(1);
    expect(error).toHaveBeenCalledWith('notice retry', expect.any(Error));
  });

  // It is read in the flow of the page until the user retries: from the press on its message is a
  // polite region, there before the answer, so the failure's words come into a region that exists.
  it('on a notice that does not announce itself, a failed retry is still announced', async () => {
    const answer = deferred<boolean>();
    const { user } = renderDom(RetryHarness, { run: () => answer.promise, announce: false });
    expect(screen.getByText(MESSAGE).closest('[role]')).toBeNull();

    await user.click(retryButton());
    const region = screen.getByRole('status');
    const { heard, stop } = listen(region);
    answer.resolve(false);
    await frame();
    await frame();
    stop();

    expect(heard.filter((words) => words.includes(MESSAGE))).toHaveLength(1);
  });
});

describe('Notice with a retry: success', () => {
  it('moves focus from the Retry to the control the caller names', async () => {
    const { user } = renderDom(RetryHarness, { run: vi.fn(async () => true), to: 'target' });
    retryButton().focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Detect' }));
  });

  it('else to the first control of the part the caller names', async () => {
    const { user } = renderDom(RetryHarness, { run: vi.fn(async () => true), to: 'region' });
    retryButton().focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Choose a program' }));
  });

  it('else to the page heading, never to <body>', async () => {
    const { user } = renderDom(RetryHarness, { run: vi.fn(async () => true) });
    retryButton().focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1, name: 'Emulators' }));
  });

  it('leaves focus alone when it was not on the Retry', async () => {
    const answer = deferred<boolean>();
    const { user } = renderDom(RetryHarness, { run: () => answer.promise, to: 'target' });
    await user.click(retryButton());
    const before = screen.getByRole('button', { name: 'Before' });
    before.focus();

    answer.resolve(true);

    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    expect(document.activeElement).toBe(before);
  });

  it('focuses nothing when the page has gone by the time the answer comes', async () => {
    const answer = deferred<boolean>();
    const view = renderDom(RetryHarness, { run: () => answer.promise, present: true });
    retryButton().focus();
    await view.user.keyboard('{Enter}');

    await view.rerender({ present: false });
    answer.resolve(true);
    await frame();

    expect(document.activeElement).toBe(document.body);
  });
});

describe('Notice with a retry: taken away while it runs', () => {
  it('leaves no frame, no error and no focus behind when the answer comes after', async () => {
    const errors = vi.spyOn(console, 'error');
    const frames = new Set<number>();
    const request = window.requestAnimationFrame.bind(window);
    const cancel = window.cancelAnimationFrame.bind(window);
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      const id = request((time) => {
        frames.delete(id);
        callback(time);
      });
      frames.add(id);
      return id;
    });
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((id) => {
      frames.delete(id);
      cancel(id);
    });
    const answer = deferred<boolean>();
    const view = renderDom(RetryHarness, { run: () => answer.promise });
    retryButton().focus();
    await view.user.keyboard('{Enter}');

    view.unmount();
    answer.resolve(false);
    // Every promise the answer sets off has settled; a frame asked for meanwhile would still wait.
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect([...frames]).toEqual([]);
    await frame();
    await frame();

    expect(errors).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(document.body);
  });
});

describe('Notice with a retry: dropped by the page before the answer', () => {
  it('still tells the page the retry is over, so what waits on it is free again', async () => {
    const { user } = renderDom(RetryHarness, { run: vi.fn(async () => true), holds: false, to: 'target' });
    const detect = screen.getByRole('button', { name: 'Detect' }) as HTMLButtonElement;
    retryButton().focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    await waitFor(() => expect(detect.disabled).toBe(false));
    expect(document.activeElement).toBe(detect);
  });
});

// A caller whose load clears its failure when it starts takes the notice away during the notice's own
// retry: a failure then comes back as a new notice, which is drawn and not said again, with focus
// lost. Nothing can repair that from inside; in development it is named.
describe('Notice with a retry: dropped by its caller during the retry', () => {
  it('names the broken contract when the retry then fails', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const answer = deferred<boolean>();
    const { user } = renderDom(RetryHarness, { run: () => answer.promise, holds: false, resets: true });

    await user.click(retryButton());
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    answer.resolve(false);
    await screen.findByRole('alert');

    await waitFor(() => expect(error).toHaveBeenCalledTimes(1));
    expect(error.mock.calls[0][0]).toMatch(/^Notice: .*bind:retrying/);
  });

  it('says nothing when the whole page went, or when the retry worked', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { user } = renderDom(RetryHarness, { run: vi.fn(async () => true), holds: false, resets: true });
    await user.click(retryButton());
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());

    const answer = deferred<boolean>();
    const left = renderDom(RetryHarness, { run: () => answer.promise });
    await left.user.click(screen.getAllByRole('button', { name: 'Retry' })[0]);
    left.unmount();
    answer.resolve(false);
    await frame();

    expect(error).not.toHaveBeenCalled();
  });
});

// A dialog is portalled out of the page, and the page under it is inert: focus that falls back to
// the page heading there would be refused by a browser and end on <body>.
describe('Notice with a retry: inside a dialog', () => {
  function setup(run: () => Promise<boolean>) {
    const app = document.createElement('div');
    app.id = 'app';
    document.body.append(app);
    const view = render(RetryDialogHarness, { target: app, props: { run } });
    return { ...view, user: keys() };
  }

  it('on success with no target keeps focus in the dialog, never on the page under it', async () => {
    const { user } = setup(vi.fn(async () => true));
    const dialog = screen.getByRole('dialog', { name: 'Pick a game' });
    retryButton().focus();

    await user.keyboard('{Enter}');

    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull());
    const active = document.activeElement as HTMLElement;
    expect(active).not.toBe(document.body);
    expect(active.closest('[inert]')).toBeNull();
    expect(active.contains(dialog) || dialog.contains(active)).toBe(true);
  });

  it('focuses nothing when the dialog has closed by the time the answer comes', async () => {
    const answer = deferred<boolean>();
    const { user, rerender } = setup(() => answer.promise);
    retryButton().focus();
    await user.keyboard('{Enter}');

    await rerender({ open: false });
    const before = document.activeElement;
    answer.resolve(true);
    await frame();

    expect(document.activeElement).toBe(before);
    expect(document.activeElement).not.toBe(screen.getByRole('heading', { level: 1, name: 'Library' }));
  });
});
