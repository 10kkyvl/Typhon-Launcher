import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { AccountError, pickAvatar, uploadAvatar, type CurrentUser } from '../services/account';
import { toasts } from '../stores/toasts';
import { authState, currentUser } from '../stores/user';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import AvatarCropModal from './AvatarCropModal.svelte';
import AvatarEditor from './AvatarEditor.svelte';

// The upload sends the picture to the account server: the service calls are mocks, and the recording
// runtime under them fails the test if anything else would have reached Go.
vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/account', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/account')>()),
  pickAvatar: vi.fn(),
  uploadAvatar: vi.fn(),
}));

const SRC = 'data:image/png;base64,AAAA';
// jsdom lays nothing out: the crop area is given a width, as a window would.
const VIEWPORT = 300;

let app: HTMLDivElement;

beforeEach(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(VIEWPORT);
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  currentUser.set({ id: 'u-ana', username: 'ana', displayName: 'Ana', avatarUrl: '' } as CurrentUser);
  authState.set('authenticated');
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  vi.mocked(pickAvatar).mockReset();
  vi.mocked(uploadAvatar).mockReset();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  currentUser.set(null);
  authState.set('bootstrapping');
  toasts.set([]);
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

const cropDialog = () => screen.getByRole('dialog', { name: msg('modals.avatarCropTitle') });

/** The picked picture arrives, 600 × 400: it covers the circle at 1×, 450 × 300, centred. */
async function loaded(dialog: HTMLElement) {
  const img = dialog.querySelector('img') as HTMLImageElement;
  Object.defineProperty(img, 'naturalWidth', { configurable: true, value: 600 });
  Object.defineProperty(img, 'naturalHeight', { configurable: true, value: 400 });
  img.dispatchEvent(new Event('load'));
  await tick();
  return img;
}

function openCrop(props: Partial<{ saving: boolean; error: string; onsave: (encoded: string, crop: unknown) => unknown; returnFocus: () => HTMLElement | null }> = {}) {
  const onsave = props.onsave ?? vi.fn();
  const view = render(AvatarCropModal, { target: app, props: { open: true, src: SRC, ...props, onsave } });
  return { ...view, dialog: cropDialog(), onsave, keys: user() };
}

const button = (dialog: HTMLElement, name: string) => within(dialog).getByRole('button', { name });

describe('AvatarCropModal: what it does today', () => {
  it('says a picture that cannot be read in a danger notice', async () => {
    const { dialog } = openCrop();
    (dialog.querySelector('img') as HTMLImageElement).dispatchEvent(new Event('error'));

    await vi.waitFor(() => expect(within(dialog).queryAllByText(msg('modals.avatarCropLoadFailed')).length).toBeGreaterThan(0));
    const notices = within(dialog)
      .getAllByText(msg('modals.avatarCropLoadFailed'))
      .filter((text) => text.closest('[data-tone="danger"]'));
    expect(notices).toHaveLength(1);
  });

  it('Save hands over the picture and the crop of the circle once it is loaded', async () => {
    const { dialog, onsave, keys } = openCrop();
    await loaded(dialog);

    await keys.click(button(dialog, msg('common.save')));

    expect(vi.mocked(onsave).mock.calls).toEqual([['AAAA', { x: 100, y: 0, size: 400 }]]);
  });

  it('Save stays focused and busy while the caller saves', async () => {
    const { dialog, rerender } = openCrop();
    await loaded(dialog);
    const save = button(dialog, msg('common.save'));
    save.focus();

    await rerender({ saving: true });

    expect(save.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(save);
  });

  it('Cancel closes and hands over nothing', async () => {
    const { dialog, onsave, keys } = openCrop();
    await loaded(dialog);

    await keys.click(button(dialog, msg('common.cancel')));

    expect(dialog.isConnected).toBe(false);
    expect(onsave).not.toHaveBeenCalled();
  });

  it('the zoom-in button scales the picture by a quarter', async () => {
    const { dialog, keys } = openCrop();
    const img = await loaded(dialog);

    await keys.click(button(dialog, msg('modals.avatarCropZoomIn')));

    expect(img.style.width).toBe('562.5px');
    expect(within(dialog).getAllByText('125%').length).toBeGreaterThan(0);
  });
});

describe('AvatarCropModal on the kit', () => {
  it('opens on the crop area, a named group the arrow keys move the picture in', async () => {
    const { dialog, keys } = openCrop();
    const img = await loaded(dialog);
    const area = within(dialog).getByRole('group', { name: msg('modals.avatarCropArea') });
    expect(document.activeElement).toBe(area);
    expect(describedBy(area)).toContain(msg('modals.avatarCropKeysHint'));
    expect(img.style.left).toBe('-75px');

    await keys.keyboard('{ArrowLeft}');
    expect(img.style.left).toBe('-91px');
    await keys.keyboard('{ArrowRight}{ArrowRight}');
    expect(img.style.left).toBe('-59px');
  });

  it('the zoom is the kit’s slider: an arrow steps it and the picture follows', async () => {
    const { dialog, keys } = openCrop();
    const img = await loaded(dialog);
    const slider = within(dialog).getByRole('slider', { name: msg('modals.avatarCropZoomLevel') });
    expect(slider.tagName).not.toBe('INPUT');
    expect(slider.getAttribute('aria-valuetext')).toBe('100%');

    slider.focus();
    await keys.keyboard('{ArrowRight}');

    expect(slider.getAttribute('aria-valuetext')).toBe('105%');
    expect(parseFloat(img.style.width)).toBeCloseTo(472.5);
  });

  it('a zoom button at its limit is held with a reason and keeps its place in the Tab order', async () => {
    const { dialog, keys } = openCrop();
    await loaded(dialog);
    const out = button(dialog, msg('modals.avatarCropZoomOut'));

    expect(out).toHaveProperty('disabled', false);
    expect(out.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(out)).toContain(msg('modals.avatarCropZoomAtMin'));

    await keys.click(button(dialog, msg('modals.avatarCropZoomIn')));
    expect(out.getAttribute('aria-disabled')).toBeNull();
    out.focus();
    await keys.keyboard('{Enter}');
    expect(document.activeElement).toBe(out);
    expect(out.getAttribute('aria-disabled')).toBe('true');
  });

  it('Save is held with the reason while the picture cannot be read', async () => {
    const { dialog, onsave, keys } = openCrop();
    (dialog.querySelector('img') as HTMLImageElement).dispatchEvent(new Event('error'));
    await tick();
    const save = button(dialog, msg('common.save'));

    expect(save).toHaveProperty('disabled', false);
    expect(save.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(save)).toContain(msg('modals.avatarCropLoadFailed'));
    await keys.click(save);
    expect(onsave).not.toHaveBeenCalled();
  });

  it('Save hands over once while its answer is pending; meanwhile Cancel is held and Escape does nothing', async () => {
    const answer = held<void>();
    const onsave = vi.fn(() => answer.promise);
    const { dialog, keys } = openCrop({ onsave });
    await loaded(dialog);
    const save = button(dialog, msg('common.save'));
    const cancel = button(dialog, msg('common.cancel'));
    save.focus();

    await keys.keyboard('{Enter}');
    await keys.click(save);

    expect(onsave).toHaveBeenCalledTimes(1);
    expect(save.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(save);
    expect(cancel).toHaveProperty('disabled', false);
    expect(describedBy(cancel)).toContain(msg('modals.actionRunning'));
    await keys.keyboard('{Escape}');
    await keys.click(cancel);
    expect(dialog.isConnected).toBe(true);

    answer.resolve();
    await vi.waitFor(() => expect(save.getAttribute('aria-busy')).toBeNull());
  });

  it('closed with the control that opened it gone, gives focus where `returnFocus` says', async () => {
    const elsewhere = document.createElement('button');
    elsewhere.textContent = 'Change avatar';
    document.body.append(elsewhere);
    const { dialog, keys } = openCrop({ returnFocus: () => elsewhere });
    await loaded(dialog);

    await keys.click(button(dialog, msg('common.cancel')));

    expect(dialog.isConnected).toBe(false);
    expect(document.activeElement).toBe(elsewhere);
  });
});

describe('AvatarCropModal from the avatar editor', () => {
  async function pickAndSave() {
    vi.mocked(pickAvatar).mockResolvedValueOnce({ data: 'AAAA', mime: 'image/png' });
    const upload = held<CurrentUser>();
    vi.mocked(uploadAvatar).mockReturnValueOnce(upload.promise);
    render(AvatarEditor, { target: app });
    const keys = user();
    const change = screen.getByRole('button', { name: msg('ui.avatarChange') });
    await keys.click(change);
    const dialog = await screen.findByRole('dialog', { name: msg('modals.avatarCropTitle') });
    await loaded(dialog);
    const save = button(dialog, msg('common.save'));
    save.focus();
    await keys.keyboard('{Enter}');
    return { upload, dialog, change, save, keys };
  }

  it('sends the picture once, and a failed upload is said in the dialog by its code; the dialog stays', async () => {
    const { upload, dialog } = await pickAndSave();

    upload.reject(new AccountError('avatar_too_large'));

    expect(await within(dialog).findByText(msg('state.accountErrorAvatarTooLarge'))).toBeTruthy();
    expect(dialog.isConnected).toBe(true);
    expect(vi.mocked(uploadAvatar).mock.calls).toEqual([['AAAA', { x: 100, y: 0, size: 400 }]]);
  });

  it('an upload that lands closes the dialog, says so, and gives focus back to "Change avatar"', async () => {
    const { upload, dialog, change } = await pickAndSave();

    upload.resolve({ id: 'u-ana', username: 'ana', displayName: 'Ana', avatarUrl: 'a.webp' } as CurrentUser);

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(get(toasts).map((toast) => [toast.message, toast.kind])).toEqual([[msg('ui.avatarUpdated'), 'success']]);
    expect(document.activeElement).toBe(change);
  });

  it('after a failed upload, focus goes to the words that say it', async () => {
    const { upload, dialog } = await pickAndSave();

    upload.reject(new AccountError('unsupported_avatar'));
    const words = await within(dialog).findByText(msg('state.accountErrorUnsupportedAvatar'));
    await tick();
    await tick();

    expect(words.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true);
  });
});
