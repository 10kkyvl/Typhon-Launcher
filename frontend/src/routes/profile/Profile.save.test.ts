import { describe, expect, it, vi } from 'vitest';
import raw from './Profile.svelte?raw';

const source = raw.replace(/\r\n/g, '\n');

interface Calls {
  saveProfile: ReturnType<typeof vi.fn>;
  toast: ReturnType<typeof vi.fn>;
  leaveEditing: ReturnType<typeof vi.fn>;
}

function harness(saveProfile: () => Promise<boolean>) {
  const start = source.indexOf('  async function save() {');
  const end = source.indexOf('\n  $effect(', start);
  const js = source
    .slice(start, end)
    .replace('$state.snapshot(draft) as ProfileSettings', 'JSON.parse(JSON.stringify(draft))')
    .replace(/\$currentUser/g, 'currentUser');
  const calls: Calls = { saveProfile: vi.fn(saveProfile), toast: vi.fn(), leaveEditing: vi.fn() };
  const run = new Function(
    'saveProfile',
    'toast',
    'leaveEditing',
    'layoutPatch',
    'accountErrorText',
    'msg',
    'currentUser',
    `
    let saving = false, saveError = '';
    const canSave = true, owner = 'u1', resetLayout = false;
    const draft = { layout: { version: 1, blocks: [] } };
    const settings = { showcase: [], layout: null };
    const layout = draft.layout;
    ${js}
    return { save, state: () => ({ saving, saveError }) };
  `,
  );
  const api = run(
    calls.saveProfile,
    calls.toast,
    calls.leaveEditing,
    () => ({ send: false }),
    (err: unknown, fallback: string) => (err instanceof Error ? err.message : fallback),
    (key: string) => key,
    { id: 'u1' },
  ) as { save: () => Promise<void>; state: () => { saving: boolean; saveError: string } };
  return { ...api, calls };
}

describe('profile save', () => {
  it('stays in edit mode and says so when another save is still in flight', async () => {
    const h = harness(() => Promise.resolve(false));

    await h.save();

    expect(h.calls.leaveEditing).not.toHaveBeenCalled();
    expect(h.calls.toast).not.toHaveBeenCalled();
    expect(h.state()).toEqual({ saving: false, saveError: 'profile.saveBusy' });
  });

  it('leaves edit mode and confirms once the profile was saved', async () => {
    const h = harness(() => Promise.resolve(true));

    await h.save();

    expect(h.calls.leaveEditing).toHaveBeenCalledTimes(1);
    expect(h.calls.toast).toHaveBeenCalledWith('social.settingsSaved', 'success');
    expect(h.state().saveError).toBe('');
  });

  it('keeps the draft and shows the error when the save fails', async () => {
    const h = harness(() => Promise.reject(new Error('offline')));

    await h.save();

    expect(h.calls.leaveEditing).not.toHaveBeenCalled();
    expect(h.state()).toEqual({ saving: false, saveError: 'offline' });
  });
});
