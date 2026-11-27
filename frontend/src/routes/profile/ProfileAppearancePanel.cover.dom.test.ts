import { cleanup, screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { AccountError, updateProfile, type CurrentUser, type ProfileAppearance } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { EMPTY_SNAPSHOT, getProfilePreview, getProfileSnapshot } from '../../lib/services/profile';
import { uploadProfileCover } from '../../lib/services/profileCover';
import { profileDraft, profileSnapshot } from '../../lib/stores/profile';
import { navigate, resetHistory, stayHere } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Profile from './Profile.svelte';
import { player, SNAPSHOT } from './profileFixtures';
import { pending } from './profileTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));
vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));
vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));
vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));

// The cover's Upload through the read of the chosen picture, the crop dialog and the upload: the
// pressed control keeps focus, busy, and is never natively disabled under it (Chromium drops focus
// to <body> from a control that turns disabled; jsdom keeps it, so the tests read the attributes that
// decide it). The system picker gives focus back to Upload when it closes; jsdom opens none, so a test
// presses Upload, then hands the chosen file to the hidden input without moving focus.

// jsdom decodes no picture: this one says it loaded, at a size the panel takes, at once or, while a
// test holds the reads, when the test lets them finish.
const heldReads: LoadedImage[] = [];
let holdReads = false;
class LoadedImage {
  naturalWidth = 1600;
  naturalHeight = 400;
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  set src(_url: string) {
    if (holdReads) heldReads.push(this);
    else queueMicrotask(() => this.onload?.());
  }
}

function finishReads() {
  holdReads = false;
  for (const image of heldReads.splice(0)) image.onload?.();
}

const button = (name: string) => screen.getByRole('button', { name });
const uploadButton = () => button(msg('profile.upload'));
const still = (extra: Partial<ProfileAppearance> = {}): CurrentUser => player({ appearance: { parallax: false, ...extra } as ProfileAppearance });
const held = (control: HTMLElement) => ({
  focused: document.activeElement === control,
  busy: control.getAttribute('aria-busy') === 'true',
  disabled: control.matches(':disabled'),
});

async function editor(me: CurrentUser = still()) {
  authState.set('authenticated');
  currentUser.set(me);
  const view = renderDom(Profile);
  await view.user.click(await screen.findByRole('button', { name: msg('profileStyle.edit') }));
  const input = document.querySelector<HTMLInputElement>('input[type="file"]') as HTMLInputElement;
  const picker = vi.spyOn(input, 'click').mockImplementation(() => {});
  return { ...view, input, picker };
}

function chosen(input: HTMLInputElement, file: File) {
  Object.defineProperty(input, 'files', { value: [file], configurable: true });
  input.dispatchEvent(new Event('change', { bubbles: true }));
}

// The crop dialog's picture loaded at the size of the chosen one, its stage drawn and its canvas
// encoding: jsdom lays out nothing and draws nothing.
async function cropReady() {
  const dialog = await screen.findByRole('dialog', { name: msg('profile.cropTitle') });
  const img = dialog.querySelector('img') as HTMLImageElement;
  Object.defineProperty(img, 'naturalWidth', { value: 1600 });
  Object.defineProperty(img, 'naturalHeight', { value: 400 });
  img.dispatchEvent(new Event('load'));
  await waitFor(() => expect(within(dialog).getByRole('button', { name: msg('profile.applyCrop') }).matches(':disabled')).toBe(false));
  return dialog;
}

beforeEach(() => {
  locale.set('en');
  resetBindings();
  navigate('profile');
  vi.mocked(getProfileSnapshot).mockResolvedValue(SNAPSHOT);
  vi.mocked(getProfilePreview).mockResolvedValue(SNAPSHOT);
  vi.stubGlobal('Image', LoadedImage);
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(800);
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage() {} } as unknown as CanvasRenderingContext2D);
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((done: BlobCallback) => done(new Blob(['webp'], { type: 'image/webp' })));
});

afterEach(() => {
  holdReads = false;
  heldReads.length = 0;
  cleanup();
  expect(calls).toEqual([]);
  for (const mock of [getProfileSnapshot, getProfilePreview, updateProfile, uploadProfileCover]) vi.mocked(mock).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  stayHere();
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  profileSnapshot.set(EMPTY_SNAPSHOT);
  profileDraft.set(null);
  toasts.set([]);
  document.body.replaceChildren();
});

describe("the cover's Upload through a still picture: read, crop, upload", () => {
  it('keeps focus, busy, through the read; the crop dialog gives focus back to it; it stays busy through the upload', async () => {
    const answer = pending<string>();
    vi.mocked(uploadProfileCover).mockReturnValue(answer.promise);
    const { user, input, picker } = await editor();
    const upload = uploadButton();
    holdReads = true;

    await user.click(upload);
    expect(picker).toHaveBeenCalledTimes(1);
    chosen(input, new File(['png'], 'cover.png', { type: 'image/png' }));
    await tick();

    expect(held(upload)).toEqual({ focused: true, busy: true, disabled: false });
    await user.click(upload);
    await user.keyboard('{Enter}');
    expect(picker).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(heldReads).toHaveLength(1));
    expect(held(upload)).toEqual({ focused: true, busy: true, disabled: false });
    finishReads();

    const dialog = await cropReady();
    await user.click(within(dialog).getByRole('button', { name: msg('profile.applyCrop') }));

    await waitFor(() => expect(screen.queryByRole('dialog', { name: msg('profile.cropTitle') })).toBeNull());
    await waitFor(() => expect(held(upload)).toEqual({ focused: true, busy: true, disabled: false }));
    expect(uploadProfileCover).toHaveBeenCalledTimes(1);

    answer.resolve('https://cdn.example/cover.webp');

    await waitFor(() => expect(held(upload)).toEqual({ focused: true, busy: false, disabled: false }));
    expect(button(msg('profile.removeCover'))).toBeTruthy();
    expect(uploadProfileCover).toHaveBeenCalledTimes(1);
  });

  it('Cancel in the crop dialog gives focus back to Upload and uploads nothing', async () => {
    const { user, input } = await editor();
    const upload = uploadButton();
    await user.click(upload);
    chosen(input, new File(['png'], 'cover.png', { type: 'image/png' }));

    const dialog = await cropReady();
    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await waitFor(() => expect(screen.queryByRole('dialog', { name: msg('profile.cropTitle') })).toBeNull());
    await waitFor(() => expect(held(upload)).toEqual({ focused: true, busy: false, disabled: false }));
    expect(uploadProfileCover).not.toHaveBeenCalled();
  });

  // WebKit does not focus a clicked button: the dialog opens from <body> and has no opener to go back to.
  it('opened while nothing held focus, the crop dialog gives focus to Upload when it closes', async () => {
    const { user, input } = await editor();
    const upload = uploadButton();
    await user.click(upload);
    upload.blur();
    chosen(input, new File(['png'], 'cover.png', { type: 'image/png' }));

    const dialog = await cropReady();
    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await waitFor(() => expect(held(upload)).toEqual({ focused: true, busy: false, disabled: false }));
  });

  it('"Edit crop" pressed again keeps focus, busy, through the upload its crop starts', async () => {
    vi.mocked(uploadProfileCover).mockResolvedValueOnce('https://cdn.example/one.webp');
    const { user, input } = await editor();
    await user.click(uploadButton());
    chosen(input, new File(['png'], 'cover.png', { type: 'image/png' }));
    await user.click(within(await cropReady()).getByRole('button', { name: msg('profile.applyCrop') }));
    const recrop = await screen.findByRole('button', { name: msg('profile.editCrop') });

    const answer = pending<string>();
    vi.mocked(uploadProfileCover).mockReturnValueOnce(answer.promise);
    await user.click(recrop);
    await user.click(within(await cropReady()).getByRole('button', { name: msg('profile.applyCrop') }));

    await waitFor(() => expect(screen.queryByRole('dialog', { name: msg('profile.cropTitle') })).toBeNull());
    await waitFor(() => expect(held(recrop)).toEqual({ focused: true, busy: true, disabled: false }));
    expect(uploadProfileCover).toHaveBeenCalledTimes(2);
    answer.resolve('https://cdn.example/two.webp');
    await waitFor(() => expect(held(recrop)).toEqual({ focused: true, busy: false, disabled: false }));
  });
});

describe('"Adjust crop" pressed by a click that takes no focus (WebKit)', () => {
  it('gets focus back when its dialog closes, and keeps it, busy, through the upload', async () => {
    vi.mocked(uploadProfileCover).mockResolvedValueOnce('https://cdn.example/one.webp');
    const { user, input } = await editor();
    await user.click(uploadButton());
    chosen(input, new File(['png'], 'cover.png', { type: 'image/png' }));
    await user.click(within(await cropReady()).getByRole('button', { name: msg('profile.applyCrop') }));
    const recrop = await screen.findByRole('button', { name: msg('profile.editCrop') });
    const answer = pending<string>();
    vi.mocked(uploadProfileCover).mockReturnValueOnce(answer.promise);

    (document.activeElement as HTMLElement | null)?.blur();
    recrop.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    expect(document.activeElement).toBe(document.body);
    await user.click(within(await cropReady()).getByRole('button', { name: msg('profile.applyCrop') }));

    await waitFor(() => expect(screen.queryByRole('dialog', { name: msg('profile.cropTitle') })).toBeNull());
    await waitFor(() => expect(held(recrop)).toEqual({ focused: true, busy: true, disabled: false }));
    answer.resolve('https://cdn.example/two.webp');
    await waitFor(() => expect(held(recrop)).toEqual({ focused: true, busy: false, disabled: false }));
  });
});

describe('while a second cover uploads', () => {
  // A Remove confirmed meanwhile would be undone by the upload's answer, and a second crop would be a
  // second upload: both are held until the answer.
  it('Remove cover and Adjust crop open nothing, and only the one upload is sent', async () => {
    vi.mocked(uploadProfileCover).mockResolvedValueOnce('https://cdn.example/one.webp');
    const { user, input } = await editor();
    await user.click(uploadButton());
    chosen(input, new File(['png'], 'one.png', { type: 'image/png' }));
    await user.click(within(await cropReady()).getByRole('button', { name: msg('profile.applyCrop') }));
    const recrop = await screen.findByRole('button', { name: msg('profile.editCrop') });
    const remove = button(msg('profile.removeCover'));

    const answer = pending<string>();
    vi.mocked(uploadProfileCover).mockReturnValueOnce(answer.promise);
    await user.click(uploadButton());
    chosen(input, new File(['png'], 'two.png', { type: 'image/png' }));
    await user.click(within(await cropReady()).getByRole('button', { name: msg('profile.applyCrop') }));
    await waitFor(() => expect(uploadProfileCover).toHaveBeenCalledTimes(2));

    await user.click(remove);
    await user.click(recrop);

    expect(screen.queryByRole('dialog')).toBeNull();
    expect(uploadProfileCover).toHaveBeenCalledTimes(2);
    answer.resolve('https://cdn.example/two.webp');
    await waitFor(() => expect(uploadButton().getAttribute('aria-busy')).toBeNull());
    expect(uploadProfileCover).toHaveBeenCalledTimes(2);
  });
});

describe("the cover's Upload through a GIF: read, upload", () => {
  it('keeps focus, busy, until the answer; a second press opens no picker and sends nothing more', async () => {
    const answer = pending<string>();
    vi.mocked(uploadProfileCover).mockReturnValue(answer.promise);
    const { user, input, picker } = await editor();
    const upload = uploadButton();

    await user.click(upload);
    chosen(input, new File(['GIF89a'], 'cover.gif', { type: 'image/gif' }));
    await waitFor(() => expect(uploadProfileCover).toHaveBeenCalledTimes(1));

    expect(held(upload)).toEqual({ focused: true, busy: true, disabled: false });
    await user.click(upload);
    await user.keyboard('{Enter}');
    expect(picker).toHaveBeenCalledTimes(1);
    expect(uploadProfileCover).toHaveBeenCalledTimes(1);

    answer.resolve('https://cdn.example/cover.gif');

    await waitFor(() => expect(held(upload)).toEqual({ focused: true, busy: false, disabled: false }));
    expect(screen.queryByRole('dialog', { name: msg('profile.cropTitle') })).toBeNull();
  });

  it('a failed upload leaves focus on Upload and says the failure in the panel, in catalogued words', async () => {
    vi.mocked(uploadProfileCover).mockRejectedValue(new AccountError('network'));
    const { user, input } = await editor();
    const upload = uploadButton();

    await user.click(upload);
    chosen(input, new File(['GIF89a'], 'cover.gif', { type: 'image/gif' }));

    const alert = await screen.findByText(accountErrorText(new AccountError('network'), msg('profile.coverError')));
    expect(alert.closest('[data-tone="danger"]')?.closest('.side')).not.toBeNull();
    expect(held(upload)).toEqual({ focused: true, busy: false, disabled: false });
    expect(uploadProfileCover).toHaveBeenCalledTimes(1);
  });
});
