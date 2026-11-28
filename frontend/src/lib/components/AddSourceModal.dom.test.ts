import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { addSource, addSourceFile, listSources, selectFeedFile, testSource, testSourceFile, type Source, type SourcePreview } from '../services/sources';
import { sources } from '../stores/sources';
import { toasts } from '../stores/toasts';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import AddSourceModal from './AddSourceModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/sources', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/sources')>()),
  testSource: vi.fn(),
  testSourceFile: vi.fn(),
  addSource: vi.fn(),
  addSourceFile: vi.fn(),
  selectFeedFile: vi.fn(),
  listSources: vi.fn(),
}));

const FEED = 'http://feed.test/feed.json';
const LONG = `https://mirror.example.test/${'very-long-path-segment/'.repeat(8)}feed.json`;

const preview = (patch: Partial<SourcePreview> = {}): SourcePreview => ({
  name: 'FitGirl',
  type: 'url',
  url: FEED,
  feedVersion: 1,
  entries: 3,
  invalid: 0,
  games: 3,
  known: 2,
  unknown: 1,
  fingerprint: '',
  duplicate: false,
  ...patch,
});
const goError = (code: string) => new Error(`typhon:${code}: GO TEXT`);
const goText = () => document.body.textContent?.includes('GO TEXT') || get(toasts).some((toast) => toast.message.includes('GO TEXT'));

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  vi.mocked(listSources).mockResolvedValue([]);
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  toasts.set([]);
  sources.set([]);
  vi.useRealTimers();
  vi.resetAllMocks();
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

function openDialog() {
  const result = render(AddSourceModal, { target: app, props: { open: true } });
  const dialog = screen.getByRole('dialog', { name: msg('modals.addSourceTitle') });
  return { ...result, dialog, keys: user(), field: within(dialog).getByRole('textbox', { name: msg('modals.addSourceUrlLabel') }) as HTMLInputElement };
}

const checkButton = (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('modals.addSourceCheck') });
const describedBy = (element: Element) =>
  (element.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

async function checked(result: () => Promise<SourcePreview>) {
  vi.mocked(testSource).mockImplementationOnce(() => result());
  const opened = openDialog();
  await opened.keys.type(opened.field, FEED);
  await opened.keys.click(checkButton(opened.dialog));
  return opened;
}

describe('AddSourceModal: opening and typing', () => {
  it('puts the focus in the URL field, a url field named by its label', () => {
    const { field } = openDialog();

    expect(document.activeElement).toBe(field);
    expect(field.type).toBe('url');
  });

  it('sends nothing while the URL is typed or pasted: only Check asks, once, for the trimmed URL', async () => {
    vi.mocked(testSource).mockResolvedValue(preview());
    const { dialog, keys, field } = openDialog();

    await keys.type(field, '  http://feed.test/');
    await keys.paste('feed.json  ');
    expect(sentToGo()).toEqual([]);
    expect(vi.mocked(testSource)).not.toHaveBeenCalled();
    expect(vi.mocked(addSource)).not.toHaveBeenCalled();

    await keys.click(checkButton(dialog));

    expect(vi.mocked(testSource).mock.calls).toEqual([[FEED]]);
    expect(await within(dialog).findByText('FitGirl')).toBeTruthy();
    expect(vi.mocked(addSource)).not.toHaveBeenCalled();
  });

  it('holds Check with its reason, in the Tab order, while there is neither a URL nor a file', async () => {
    const { dialog, keys, field } = openDialog();
    const check = checkButton(dialog);

    expect(check.hasAttribute('disabled')).toBe(false);
    expect(check.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(check)).toContain(msg('modals.addSourceNeedInput'));
    await keys.click(check);
    expect(vi.mocked(testSource)).not.toHaveBeenCalled();

    await keys.type(field, FEED);
    expect(check.getAttribute('aria-disabled')).toBeNull();
  });
});

describe('AddSourceModal: Enter in the URL field', () => {
  it('checks the typed URL once, as Check does, and adds nothing', async () => {
    const reading = held<SourcePreview>();
    vi.mocked(testSource).mockReturnValueOnce(reading.promise);
    const { dialog, keys, field } = openDialog();
    await keys.type(field, FEED);

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(testSource).mock.calls).toEqual([[FEED]]);
    expect(checkButton(dialog).getAttribute('aria-busy')).toBe('true');
    reading.resolve(preview());
    expect(await within(dialog).findByText('FitGirl')).toBeTruthy();
    expect(vi.mocked(addSource)).not.toHaveBeenCalled();
  });

  it('does nothing while the field is empty or an input method composes', async () => {
    const { keys, field } = openDialog();

    await keys.keyboard('{Enter}');
    await keys.type(field, FEED);
    await fireEvent.keyDown(field, { key: 'Enter', keyCode: 229 });
    await fireEvent.keyDown(field, { key: 'Enter', isComposing: true });

    expect(vi.mocked(testSource)).not.toHaveBeenCalled();
  });
});

describe('AddSourceModal: checking', () => {
  it('keeps the focus on Check, busy, while the feed is read, and a second press sends nothing', async () => {
    const reading = held<SourcePreview>();
    vi.mocked(testSource).mockReturnValueOnce(reading.promise);
    const { dialog, keys, field } = openDialog();
    await keys.type(field, FEED);
    const check = checkButton(dialog);
    check.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    expect(vi.mocked(testSource)).toHaveBeenCalledTimes(1);
    expect(check.isConnected).toBe(true);
    expect(check.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(check);
    expect(within(dialog).getAllByRole('status').map((status) => status.textContent)).toContainEqual(expect.stringContaining(msg('modals.addSourceChecking')));
    reading.resolve(preview());
  });

  it('says a refused check at the URL field by its code, never Go text, keeps the URL and puts the focus there', async () => {
    const { dialog, keys } = await checked(() => Promise.reject(goError('sources.feed_invalid_url')));

    const field = await vi.waitFor(() => {
      const found = within(dialog).getByRole('textbox', { name: msg('modals.addSourceUrlLabel') }) as HTMLInputElement;
      expect(found.getAttribute('aria-invalid')).toBe('true');
      return found;
    });
    expect(describedBy(field)).toContain(msg('errSources.srcFeedInvalidUrl'));
    expect(field.value).toBe(FEED);
    await vi.waitFor(() => expect(document.activeElement).toBe(field));
    expect(goText()).toBe(false);
    expect(get(toasts)).toEqual([]);
    expect(keys).toBeTruthy();
  });

  it('gives a failure with no code the dialog’s own words for a check', async () => {
    const { dialog } = await checked(() => Promise.reject(new Error('dial tcp: GO TEXT')));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.addSourceCheckFailed'));
    expect(goText()).toBe(false);
  });

  it('drops a check that answers after the dialog was opened again', async () => {
    const reading = held<SourcePreview>();
    vi.mocked(testSource).mockReturnValueOnce(reading.promise);
    const { dialog, keys, field, rerender } = openDialog();
    await keys.type(field, FEED);
    await keys.click(checkButton(dialog));
    await rerender({ open: false });
    await rerender({ open: true });

    reading.resolve(preview());
    await new Promise((resolve) => setTimeout(resolve, 0));

    const again = screen.getByRole('dialog', { name: msg('modals.addSourceTitle') });
    expect(within(again).queryByText('FitGirl')).toBeNull();
    expect((within(again).getByRole('textbox', { name: msg('modals.addSourceUrlLabel') }) as HTMLInputElement).value).toBe('');
  });
});

describe('AddSourceModal: the preview', () => {
  it('states what is wrong with the checked feed in warning notices read with the preview', async () => {
    const { dialog } = await checked(async () => preview({ insecure: true, duplicate: true }));

    for (const text of [msg('modals.addSourceInsecure'), msg('modals.addSourceDuplicate')]) {
      const notice = (await within(dialog).findByText(text)).closest('[data-tone="warning"]') as HTMLElement;
      expect(notice).not.toBeNull();
      expect(notice.querySelector('[role="status"], [role="alert"]')).toBeNull();
    }
  });

  it('shows a long URL whole, wrapped, with no native title', async () => {
    const { dialog } = await checked(async () => preview({ url: LONG }));

    expect(await within(dialog).findByText(LONG)).toBeTruthy();
    expect(dialog.querySelectorAll('[title]').length).toBe(0);
  });

  it('moves the focus to Add once the preview is in', async () => {
    const { dialog } = await checked(async () => preview());

    const add = await within(dialog).findByRole('button', { name: msg('common.add') });
    await vi.waitFor(() => expect(document.activeElement).toBe(add));
  });
});

describe('AddSourceModal: adding', () => {
  it('keeps the focus on "add", busy, while the source is added, and two presses add it once', async () => {
    const adding = held<Source>();
    vi.mocked(addSource).mockReturnValueOnce(adding.promise);
    const { dialog, keys } = await checked(async () => preview());
    const add = await within(dialog).findByRole('button', { name: msg('common.add') });
    add.focus();

    await keys.keyboard('{Enter}');
    await keys.keyboard('{Enter}');

    const busy = within(dialog).getByRole('button', { name: msg('modals.addSourceAdding') });
    expect(busy).toBe(add);
    expect(busy.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(busy);
    expect(vi.mocked(addSource).mock.calls).toEqual([[FEED]]);
    adding.resolve({ id: 's1', name: 'FitGirl' } as Source);
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(get(toasts).map((toast) => toast.message)).toEqual([msg('modals.addSourceAdded', { name: 'FitGirl' })]);
  });

  it('says a refused add at the URL field by its code, the typed URL still there and focused', async () => {
    vi.mocked(addSource).mockRejectedValueOnce(goError('sources.source_exists'));
    const { dialog, keys } = await checked(async () => preview());
    await keys.click(await within(dialog).findByRole('button', { name: msg('common.add') }));

    const field = await within(dialog).findByRole('textbox', { name: msg('modals.addSourceUrlLabel') });
    expect((field as HTMLInputElement).value).toBe(FEED);
    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect(describedBy(field)).toContain(msg('errSources.srcSourceExists'));
    await vi.waitFor(() => expect(document.activeElement).toBe(field));
    expect(goText()).toBe(false);
    expect(get(toasts)).toEqual([]);
  });

  it('gives a failed add with no code the dialog’s own words', async () => {
    vi.mocked(addSource).mockRejectedValueOnce(new Error('save sources: GO TEXT'));
    const { dialog, keys } = await checked(async () => preview());
    await keys.click(await within(dialog).findByRole('button', { name: msg('common.add') }));

    const field = await within(dialog).findByRole('textbox', { name: msg('modals.addSourceUrlLabel') });
    expect(describedBy(field)).toContain(msg('modals.addSourceAddFailed'));
    expect(goText()).toBe(false);
  });

  it('hands a refusal that answers after the dialog was minimised to a toast, in words by code', async () => {
    const adding = held<Source>();
    vi.mocked(addSource).mockReturnValueOnce(adding.promise);
    const { dialog, keys } = await checked(async () => preview());
    await keys.click(await within(dialog).findByRole('button', { name: msg('common.add') }));
    await keys.click(within(dialog).getByRole('button', { name: msg('modals.addSourceMinimize') }));
    expect(screen.queryByRole('dialog')).toBeNull();

    adding.reject(goError('sources.feed_challenge'));
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(get(toasts).map((toast) => [toast.message, toast.kind])).toEqual([[msg('errSources.srcFeedChallenge'), 'danger']]);
    expect(goText()).toBe(false);
  });
});

// Go's AddSource saves the source, then answers its first refresh's error.
describe('AddSourceModal: a source saved whose first refresh failed', () => {
  const saved = { id: 'n', name: 'feed.test', type: 'url', url: FEED, status: 'error' } as Source;
  const told = () => [msg('modals.addSourceAddedRefreshFailed', { name: 'feed.test', reason: msg('errSources.srcFeedHttpStatus', { status: '503' }) }), 'warning'];

  it('says it was added with why the refresh failed, in one warning toast, and closes with the focus back on the opener', async () => {
    const opener = document.createElement('button');
    opener.textContent = 'Add source';
    app.append(opener);
    opener.focus();
    vi.mocked(addSource).mockImplementationOnce(async () => {
      vi.mocked(listSources).mockResolvedValue([saved]);
      throw goError('sources.feed_bad_status: status 503');
    });
    const { dialog, keys } = await checked(async () => preview());

    await keys.click(await within(dialog).findByRole('button', { name: msg('common.add') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(get(toasts).map((toast) => [toast.message, toast.kind])).toEqual([told()]);
    expect(vi.mocked(addSource).mock.calls).toEqual([[FEED]]);
    expect(get(sources).map((item) => item.id)).toEqual(['n']);
    await vi.waitFor(() => expect(document.activeElement).toBe(opener));
    expect(goText()).toBe(false);
  });

  it('says the same once the dialog was minimised', async () => {
    const adding = held<Source>();
    vi.mocked(addSource).mockReturnValueOnce(adding.promise);
    const { dialog, keys } = await checked(async () => preview());
    await keys.click(await within(dialog).findByRole('button', { name: msg('common.add') }));
    await keys.click(within(dialog).getByRole('button', { name: msg('modals.addSourceMinimize') }));

    vi.mocked(listSources).mockResolvedValue([saved]);
    adding.reject(goError('sources.feed_bad_status: status 503'));

    await vi.waitFor(() => expect(get(toasts).map((toast) => [toast.message, toast.kind])).toEqual([told()]));
  });

  it('finds the source saved in Go’s spelling of a Cyrillic URL, by the place Go named at Check, beside another new one', async () => {
    const typed = 'https://раздачи.рф/feed.json';
    const goPlace = 'https://%D1%80%D0%B0%D0%B7%D0%B4%D0%B0%D1%87%D0%B8.%D1%80%D1%84/feed.json';
    vi.mocked(testSource).mockResolvedValueOnce(preview({ url: goPlace }));
    vi.mocked(addSource).mockImplementationOnce(async () => {
      vi.mocked(listSources).mockResolvedValue([
        { ...saved, id: 'x', name: 'elsewhere', url: 'https://elsewhere.test/feed.json' },
        { ...saved, url: goPlace },
      ]);
      throw goError('sources.feed_bad_status: status 503');
    });
    const { dialog, keys, field } = openDialog();
    await keys.type(field, typed);
    await keys.click(checkButton(dialog));

    await keys.click(await within(dialog).findByRole('button', { name: msg('common.add') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(get(toasts).map((toast) => [toast.message, toast.kind])).toEqual([told()]);
    expect(vi.mocked(addSource).mock.calls).toEqual([[typed]]);
  });

  it('keeps a refusal that saved nothing at the field, the URL there, sent once', async () => {
    vi.mocked(addSource).mockRejectedValueOnce(goError('sources.feed_invalid_url'));
    const { dialog, keys } = await checked(async () => preview());
    await keys.click(await within(dialog).findByRole('button', { name: msg('common.add') }));

    const field = await within(dialog).findByRole('textbox', { name: msg('modals.addSourceUrlLabel') });
    expect((field as HTMLInputElement).value).toBe(FEED);
    await vi.waitFor(() => expect(describedBy(field)).toContain(msg('errSources.srcFeedInvalidUrl')));
    expect(vi.mocked(addSource).mock.calls).toEqual([[FEED]]);
    expect(get(toasts)).toEqual([]);
  });
});

describe('AddSourceModal: a feed file', () => {
  it('is a path field: the chosen path is shown whole and read on its Browse button, and is checked as a file', async () => {
    vi.mocked(selectFeedFile).mockResolvedValueOnce('/Users/me/feeds/feed.json');
    vi.mocked(testSourceFile).mockResolvedValueOnce(preview({ type: 'file', url: '', path: '/Users/me/feeds/feed.json' }));
    const { dialog, keys } = openDialog();
    const browse = within(dialog).getByRole('button', { name: msg('modals.addSourcePickFile') });
    expect(describedBy(browse)).toContain(msg('modals.addSourceNoFile'));

    await keys.click(browse);

    expect(vi.mocked(testSourceFile).mock.calls).toEqual([['/Users/me/feeds/feed.json']]);
    expect(await within(dialog).findByText('/Users/me/feeds/feed.json')).toBeTruthy();
    expect(dialog.querySelectorAll('[title]').length).toBe(0);
  });

  it('adds the picked file, not a URL typed before it', async () => {
    vi.mocked(selectFeedFile).mockResolvedValueOnce('/Users/me/feeds/feed.json');
    vi.mocked(testSourceFile).mockResolvedValueOnce(preview({ type: 'file', url: '', path: '/Users/me/feeds/feed.json' }));
    vi.mocked(addSourceFile).mockResolvedValueOnce({ id: 'f', name: 'feed.json' } as Source);
    const { dialog, keys, field } = openDialog();
    await keys.type(field, FEED);

    await keys.click(within(dialog).getByRole('button', { name: msg('modals.addSourcePickFile') }));
    await keys.click(await within(dialog).findByRole('button', { name: msg('common.add') }));

    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(vi.mocked(testSource)).not.toHaveBeenCalled();
    expect(vi.mocked(addSourceFile).mock.calls).toEqual([['/Users/me/feeds/feed.json']]);
    expect(vi.mocked(addSource)).not.toHaveBeenCalled();
  });

  it('keeps one of the two: a picked file clears the typed URL, and a URL typed after clears the file', async () => {
    vi.mocked(selectFeedFile).mockResolvedValueOnce('/Users/me/feeds/feed.json');
    vi.mocked(testSourceFile).mockResolvedValueOnce(preview({ type: 'file', url: '', path: '/Users/me/feeds/feed.json' }));
    vi.mocked(testSource).mockResolvedValueOnce(preview());
    const { dialog, keys, field } = openDialog();
    await keys.type(field, FEED);

    await keys.click(within(dialog).getByRole('button', { name: msg('modals.addSourcePickFile') }));
    await keys.click(await within(dialog).findByRole('button', { name: msg('common.back') }));

    const url = within(dialog).getByRole('textbox', { name: msg('modals.addSourceUrlLabel') }) as HTMLInputElement;
    const browse = within(dialog).getByRole('button', { name: msg('modals.addSourcePickFile') });
    expect(url.value).toBe('');
    expect(describedBy(browse)).toContain('/Users/me/feeds/feed.json');

    await keys.type(url, FEED);

    expect(describedBy(browse)).toContain(msg('modals.addSourceNoFile'));
    await keys.click(checkButton(dialog));
    expect(vi.mocked(testSource).mock.calls).toEqual([[FEED]]);
    expect(vi.mocked(testSourceFile).mock.calls).toEqual([['/Users/me/feeds/feed.json']]);
  });

  it('says a picker that could not open at the file field, by code', async () => {
    vi.mocked(selectFeedFile).mockRejectedValueOnce(goError('sources.dialog_unavailable'));
    const { dialog, keys } = openDialog();
    const browse = within(dialog).getByRole('button', { name: msg('modals.addSourcePickFile') });

    await keys.click(browse);

    await vi.waitFor(() => expect(describedBy(browse)).toContain(msg('errSources.srcDialogUnavailable')));
    expect(vi.mocked(testSourceFile)).not.toHaveBeenCalled();
    expect(goText()).toBe(false);
  });

  it('says a refused file at the file field, not at the URL field', async () => {
    vi.mocked(selectFeedFile).mockResolvedValueOnce('/Users/me/feeds/feed.json');
    vi.mocked(testSourceFile).mockRejectedValueOnce(goError('sources.feed_not_regular_file'));
    const { dialog, keys, field } = openDialog();

    await keys.click(within(dialog).getByRole('button', { name: msg('modals.addSourcePickFile') }));

    const browse = await within(dialog).findByRole('button', { name: msg('modals.addSourcePickFile') });
    await vi.waitFor(() => expect(describedBy(browse)).toContain(msg('errSources.srcFeedNotRegularFile')));
    expect(within(dialog).getByRole('textbox', { name: msg('modals.addSourceUrlLabel') }).getAttribute('aria-invalid')).toBeNull();
    expect(field).toBeTruthy();
  });
});
