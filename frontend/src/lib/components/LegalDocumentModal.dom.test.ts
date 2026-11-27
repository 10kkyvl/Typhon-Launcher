import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const backend = vi.hoisted(() => ({ inWails: true }));

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({
  get inWails() {
    return backend.inWails;
  },
}));
vi.mock('../services/legal', async (original) => (await import('../testing/settingsDialogsTesting')).legalService(await original()));
vi.mock('../../../bindings/typhon/internal/legal', async (original) => (await import('../testing/settingsDialogsTesting')).fakeService(await original(), 'legal'));

import { msg } from '../i18n';
import { getLegalDocument, type LegalDocument } from '../services/legal';
import { user } from '../testing/dom';
import { calls, resetBindings } from '../testing/fakeBinding';
import { deferred } from '../testing/settingsDialogsTesting';
import LegalDocumentModal from './LegalDocumentModal.svelte';

let app: HTMLDivElement;

beforeEach(() => {
  backend.inWails = true;
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  vi.restoreAllMocks();
  vi.mocked(getLegalDocument).mockReset();
  expect(calls).toEqual([]);
  resetBindings();
});

const BODY = [
  '# Privacy policy',
  '',
  'Intro with **bold** words and `code`.',
  '',
  '## 1. On the device',
  '',
  '- one',
  '- two',
  '',
  '### 1.1. Logs',
  '',
  'See [the Svelte contributors](https://github.com/sveltejs/svelte/graphs/contributors) and [a script](javascript:alert(1)).',
].join('\n');

const doc = (body = BODY): LegalDocument => ({ id: 'privacy', title: 'Privacy policy', body });

function show(documentId = 'privacy', title = 'Privacy') {
  const rendered = render(LegalDocumentModal, { target: app, props: { open: true, documentId, title } });
  return { ...rendered, dialog: screen.getByRole('dialog', { name: title }) };
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

describe('LegalDocumentModal', () => {
  it('says it is loading with a labelled spinner, and a failed load in a danger notice', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const loaded = deferred<LegalDocument>();
    vi.mocked(getLegalDocument).mockReturnValueOnce(loaded.promise);
    const { dialog } = show();
    expect(within(dialog).getByRole('status').textContent?.trim()).toBe(msg('modals.legalLoading'));
    expect(getLegalDocument).toHaveBeenCalledExactlyOnceWith('privacy');

    loaded.reject(new Error('offline'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('modals.legalLoadFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('says outside the app that the documents are not there, with nothing to retry', async () => {
    backend.inWails = false;
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(getLegalDocument).mockRejectedValueOnce(new Error('unavailable in browser'));
    const { dialog } = show();

    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('modals.legalUnavailableOutsideApp'));
    expect(within(dialog).queryByRole('button', { name: msg('common.retry') })).toBeNull();
  });

  it('shows the document’s paragraphs, lists and emphasis', async () => {
    vi.mocked(getLegalDocument).mockResolvedValueOnce(doc());
    const { dialog } = show();

    expect(await within(dialog).findByText('bold')).toBeTruthy();
    expect(within(dialog).getByText('bold').tagName).toBe('STRONG');
    expect(within(dialog).getByText('code').tagName).toBe('CODE');
    expect(within(dialog).getAllByRole('listitem').map((item) => item.textContent)).toEqual(['one', 'two']);
  });

  it('keeps the document’s headings under the dialog’s title in the outline', async () => {
    vi.mocked(getLegalDocument).mockResolvedValueOnce(doc());
    const { dialog } = show();
    await within(dialog).findByText('bold');

    const title = within(dialog).getByRole('heading', { name: 'Privacy' });
    const levels = within(dialog)
      .getAllByRole('heading')
      .filter((heading) => heading !== title)
      .map((heading) => [heading.tagName, heading.textContent]);
    expect(Number(title.tagName.slice(1))).toBe(3);
    expect(levels).toEqual([
      ['H4', 'Privacy policy'],
      ['H5', '1. On the device'],
      ['H6', '1.1. Logs'],
    ]);
  });

  it('opens a link of the document outside, as a link with a web address only', async () => {
    vi.mocked(getLegalDocument).mockResolvedValueOnce(doc());
    const { dialog } = show();
    await within(dialog).findByText('bold');

    const link = within(dialog).getByRole('link', { name: 'the Svelte contributors' });
    expect(link.getAttribute('href')).toBe('https://github.com/sveltejs/svelte/graphs/contributors');
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
    expect(within(dialog).queryByRole('link', { name: 'a script' })).toBeNull();
    expect(within(dialog).getByText(/a script/)).toBeTruthy();
  });

  it('hands the focus to the document once it shows, a region the keys can scroll, named by the title', async () => {
    const loaded = deferred<LegalDocument>();
    vi.mocked(getLegalDocument).mockReturnValueOnce(loaded.promise);
    const { dialog } = show();

    loaded.resolve(doc());
    await within(dialog).findByText('bold');

    const region = within(dialog).getByRole('document', { name: 'Privacy' });
    await vi.waitFor(() => expect(document.activeElement).toBe(region));
    expect(region.tabIndex).toBe(0);
  });

  it('leaves the focus where the player moved it while the document loaded', async () => {
    const loaded = deferred<LegalDocument>();
    vi.mocked(getLegalDocument).mockReturnValueOnce(loaded.promise);
    const { dialog } = show();
    const close = within(dialog).getByRole('button', { name: msg('common.close') });
    close.focus();

    loaded.resolve(doc());
    await within(dialog).findByText('bold');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(document.activeElement).toBe(close);
  });

  it('gives the focus back to what opened it on close', async () => {
    vi.mocked(getLegalDocument).mockResolvedValueOnce(doc());
    const opener = document.createElement('button');
    opener.textContent = 'Open';
    app.append(opener);
    opener.focus();
    const { dialog } = show();
    await within(dialog).findByText('bold');

    await user().keyboard('{Escape}');

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  it('loads again from the failure notice’s Retry, and gives the focus to the document', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(getLegalDocument).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(doc());
    const { dialog } = show();
    const keys = user();
    const retry = await within(dialog).findByRole('button', { name: msg('common.retry') });
    retry.focus();

    await keys.keyboard('{Enter}');

    expect(await within(dialog).findByText('bold')).toBeTruthy();
    expect(getLegalDocument).toHaveBeenNthCalledWith(2, 'privacy');
    await vi.waitFor(() => expect(document.activeElement).toBe(within(dialog).getByRole('document', { name: 'Privacy' })));
  });

  it('keeps the notice and the focus on Retry when it fails again, and says it again', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(getLegalDocument).mockRejectedValue(new Error('offline'));
    const { dialog } = show();
    const keys = user();
    const retry = await within(dialog).findByRole('button', { name: msg('common.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    const said = sayingAgain(notice);
    retry.focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(msg('modals.legalLoadFailed')));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });

  it('says a document with no text in its own words', async () => {
    vi.mocked(getLegalDocument).mockResolvedValueOnce(doc('\n\n<!-- nothing -->\n'));
    const { dialog } = show();

    expect(await within(dialog).findByText(msg('modals.legalEmpty'))).toBeTruthy();
    expect(within(dialog).queryByRole('alert')).toBeNull();
  });

  it('shows the document of its latest opening when an earlier one’s answer lands last', async () => {
    const first = deferred<LegalDocument>();
    const second = deferred<LegalDocument>();
    vi.mocked(getLegalDocument).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { rerender } = show('privacy', 'Privacy');
    await rerender({ open: false });
    await rerender({ open: true, documentId: 'terms', title: 'Terms' });
    const dialog = screen.getByRole('dialog', { name: 'Terms' });

    second.resolve({ id: 'terms', title: 'Terms', body: 'The terms text.' });
    expect(await within(dialog).findByText('The terms text.')).toBeTruthy();
    first.resolve(doc('The privacy text.'));
    await first.promise;
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(dialog).queryByText('The privacy text.')).toBeNull();
    expect(within(dialog).getByText('The terms text.')).toBeTruthy();
  });

  it('keeps the document of its latest opening when an earlier one’s failure lands last', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const first = deferred<LegalDocument>();
    const second = deferred<LegalDocument>();
    vi.mocked(getLegalDocument).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { rerender } = show('privacy', 'Privacy');
    await rerender({ open: false });
    await rerender({ open: true, documentId: 'terms', title: 'Terms' });
    const dialog = screen.getByRole('dialog', { name: 'Terms' });

    second.resolve({ id: 'terms', title: 'Terms', body: 'The terms text.' });
    expect(await within(dialog).findByText('The terms text.')).toBeTruthy();
    first.reject(new Error('typhon:legal.read: late'));
    await expect(first.promise).rejects.toThrow('late');
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(within(dialog).queryByRole('alert')).toBeNull();
    expect(within(dialog).getByText('The terms text.')).toBeTruthy();
  });
});
