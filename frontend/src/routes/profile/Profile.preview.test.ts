import { describe, expect, it, vi } from 'vitest';
import raw from './Profile.svelte?raw';

const source = raw.replace(/\r\n/g, '\n');

function harness(getProfilePreview: () => Promise<unknown>) {
  const start = source.indexOf('  $effect(() => {\n    let active = true;');
  const end = source.indexOf('\n  onMount(', start);
  const js = source
    .slice(start, end)
    .replace('$effect(', 'effect(')
    .replace('$currentUser', 'currentUser');
  const run = new Function('getProfilePreview', 'effect', 'currentUser', `
    let wantsFull = false, full = null, previewFailed = false;
    ${js}
    return { setOpen: (value) => { wantsFull = value; }, state: () => ({ full, previewFailed }) };
  `);
  let rerun!: () => (() => void) | void;
  const api = run(getProfilePreview, (fn: () => (() => void) | void) => { rerun = fn; }, { id: 'u1' }) as {
    setOpen: (value: boolean) => void;
    state: () => { full: unknown; previewFailed: boolean };
  };
  return { ...api, rerun: () => rerun() };
}

describe('profile appearance preview', () => {
  it('tells the user when the showcase preview could not be loaded', async () => {
    const h = harness(() => Promise.reject(new Error('backend offline')));
    h.setOpen(true);

    h.rerun();
    await vi.waitFor(() => expect(h.state().previewFailed).toBe(true));

    expect(h.state().full).toBeNull();
  });

  it('clears the failure when the preview loads on the next open', async () => {
    const getProfilePreview = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ showcase: [] });
    const h = harness(getProfilePreview);
    h.setOpen(true);
    h.rerun();
    await vi.waitFor(() => expect(h.state().previewFailed).toBe(true));

    h.rerun();
    await vi.waitFor(() => expect(h.state().full).toEqual({ showcase: [] }));

    expect(h.state().previewFailed).toBe(false);
  });

  it('shows the failure as an alert', () => {
    expect(source).toMatch(/\{#if previewFailed\}[\s\S]*role="alert"/);
  });
});
