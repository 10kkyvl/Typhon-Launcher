import { render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { confirmMatch, similarReleases, type Release, type SimilarRelease } from '../services/sources';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import SimilarReleases from './SimilarReleases.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  similarReleases: vi.fn(),
  confirmMatch: vi.fn(),
}));

function similar(id: string, title: string): SimilarRelease {
  return {
    release: {
      id,
      sourceId: 'fixture',
      rawTitle: `${title} [repack]`,
      title,
      normalizedTitle: title.toLowerCase(),
      canonicalGameId: null,
      version: '',
      size: 1024,
      uploadedAt: null,
      uris: [],
      matchStatus: 'review',
      matchConfidence: 0.5,
      matchMethod: 'fuzzy',
      availability: 'available',
      firstSeenAt: '',
      lastSeenAt: '',
      createdAt: '',
    } as unknown as Release,
    sourceName: 'Fixture source',
    gameTitle: '',
    matchedCandidate: { id: '', title: '', year: 0 },
    reason: 'ambiguous',
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

let page: HTMLElement;
let heading: HTMLElement;

// The game page: its title heading in <main>, the block under it.
function renderBlock() {
  return render(SimilarReleases, { target: page, props: { gameId: 'game-1', familyIds: [] } });
}

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  page = document.createElement('main');
  heading = document.createElement('h1');
  heading.tabIndex = -1;
  heading.textContent = 'Hades';
  page.append(heading);
  document.body.append(page);
});

afterEach(() => {
  vi.mocked(similarReleases).mockReset();
  vi.mocked(confirmMatch).mockReset();
  toasts.set([]);
  page.remove();
  vi.restoreAllMocks();
});

const linkName = { name: msg('games.similarLink') };
const retryName = { name: msg('games.catalogRetry') };

describe('SimilarReleases: a failed link', () => {
  it('says the sources code\'s text, never the Go words or the code', async () => {
    vi.mocked(similarReleases).mockResolvedValue([similar('r1', 'Hades')]);
    vi.mocked(confirmMatch).mockRejectedValue(new Error('typhon:sources.release_not_found: релиз не найден'));
    renderBlock();
    const keys = user();

    await keys.click(await screen.findByRole('button', linkName));

    await vi.waitFor(() => expect(get(toasts).map((item) => item.message)).toEqual([msg('errSources.srcReleaseNotFound')]));
  });

  it('says its own fallback when the code has no text', async () => {
    vi.mocked(similarReleases).mockResolvedValue([similar('r1', 'Hades')]);
    vi.mocked(confirmMatch).mockRejectedValue(new Error('typhon:sources.something_new: что-то сломалось'));
    renderBlock();
    const keys = user();

    await keys.click(await screen.findByRole('button', linkName));

    await vi.waitFor(() => expect(get(toasts).map((item) => item.message)).toEqual([msg('games.blocks.similarLinkFailed')]));
  });
});

describe('SimilarReleases: Link while it runs', () => {
  it('stays focused and busy, not disabled, and a second press sends nothing', async () => {
    vi.mocked(similarReleases).mockResolvedValue([similar('r1', 'Hades'), similar('r2', 'Hades II')]);
    const linking = deferred<void>();
    vi.mocked(confirmMatch).mockReturnValue(linking.promise);
    renderBlock();
    const keys = user();
    const [link] = await screen.findAllByRole('button', linkName);
    link.focus();

    await keys.keyboard('{Enter}');

    expect(link.getAttribute('aria-busy')).toBe('true');
    expect((link as HTMLButtonElement).disabled).toBe(false);
    expect(document.activeElement).toBe(link);
    await keys.keyboard('{Enter}');
    expect(confirmMatch).toHaveBeenCalledTimes(1);
    linking.resolve();
  });

  it('when the block goes after the link, focus moves to the page heading', async () => {
    vi.mocked(similarReleases).mockResolvedValueOnce([similar('r1', 'Hades')]).mockResolvedValueOnce([]);
    vi.mocked(confirmMatch).mockResolvedValue(undefined);
    renderBlock();
    const keys = user();
    const link = await screen.findByRole('button', linkName);
    link.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(link.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(heading));
  });

  it('when other releases stay, focus moves to the next one\'s Link', async () => {
    vi.mocked(similarReleases)
      .mockResolvedValueOnce([similar('r1', 'Hades'), similar('r2', 'Hades II')])
      .mockResolvedValueOnce([similar('r2', 'Hades II')]);
    vi.mocked(confirmMatch).mockResolvedValue(undefined);
    renderBlock();
    const keys = user();
    const [link] = await screen.findAllByRole('button', linkName);
    link.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(link.isConnected).toBe(false));
    const rest = screen.getByRole('button', linkName);
    await vi.waitFor(() => expect(document.activeElement).toBe(rest));
  });
});

describe('SimilarReleases: retrying a failed load', () => {
  it('keeps the section and the focus on its busy Retry until the answer, then focuses the list', async () => {
    const again = deferred<SimilarRelease[]>();
    vi.mocked(similarReleases).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    renderBlock();
    const keys = user();
    const retry = await screen.findByRole('button', retryName);
    const notice = retry.closest('.notice') as HTMLElement;
    expect(within(notice).getByText(msg('games.similarFailed'))).toBeTruthy();
    retry.focus();

    await keys.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve([similar('r1', 'Hades')]);
    const link = await screen.findByRole('button', linkName);
    await vi.waitFor(() => expect(document.activeElement).toBe(link));
  });

  it('keeps the notice when the retry fails again, says it again, and can be retried once more', async () => {
    vi.mocked(similarReleases).mockRejectedValue(new Error('offline'));
    renderBlock();
    const keys = user();
    const retry = await screen.findByRole('button', retryName);
    const notice = retry.closest('.notice') as HTMLElement;
    const said = sayingAgain(notice);
    retry.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(msg('games.similarFailed')));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(retry.hasAttribute('aria-busy')).toBe(false);

    await keys.keyboard('{Enter}');
    await vi.waitFor(() => expect(similarReleases).toHaveBeenCalledTimes(3));
  });
});
