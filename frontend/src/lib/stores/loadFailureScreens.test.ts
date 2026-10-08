import { describe, expect, it } from 'vitest';
import installed from '../../routes/installed/Installed.svelte?raw';
import profile from '../../routes/profile/Profile.svelte?raw';
import activity from '../../routes/activity/Activity.svelte?raw';
import bigPictureProfile from '../bigpicture/components/ProfilePage.svelte?raw';

describe('screens show a failed load instead of an empty state', () => {
  it('the installed games page shows the disk usage failure with a retry', () => {
    expect(installed).toMatch(/\{:else if \$storageFailed\}[\s\S]*?role="alert"[\s\S]*?games\.installedStorageLoadFailed[\s\S]*?onclick=\{refreshStorage\}/);
  });

  it.each([
    ['profile', profile, "msg('profile.loadFailed')"],
    ['activity', activity, "msg('profile.loadFailed')"],
    ['big picture profile', bigPictureProfile, "$t('bp.profile.loadFailed')"],
  ])('the %s screen shows the profile statistics failure', (_name, source, text) => {
    expect(source).toMatch(/\{#if \$profileFailed\}\s*<p[^>]*role="alert"/);
    expect(source).toContain(text);
  });
});
