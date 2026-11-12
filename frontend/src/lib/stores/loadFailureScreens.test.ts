import { describe, expect, it } from 'vitest';
import installed from '../../routes/installed/Installed.svelte?raw';
import profile from '../../routes/profile/Profile.svelte?raw';
import activity from '../../routes/activity/Activity.svelte?raw';
import bigPictureProfile from '../bigpicture/components/ProfilePage.svelte?raw';

describe('screens show a failed load instead of an empty state', () => {
  it('the installed games page shows the disk usage failure with a retry', () => {
    // A danger Notice is an alert by its tone; its Retry refreshes the disk usage.
    expect(installed).toMatch(/\{:else if \$storageFailed\}[\s\S]*?<Notice tone="danger">[\s\S]*?games\.installedStorageLoadFailed[\s\S]*?onclick=\{retryStorage\}/);
    expect(installed).toMatch(/async function retryStorage\(\)[\s\S]*?await refreshStorage\(\);/);
  });

  // A danger Notice is an alert by its tone; Big Picture has no backdrop and keeps its own line.
  const NOTICE = /\{#if \$profileFailed\}\s*(<div[^>]*>)?<Notice tone="danger">/;
  const LINE = /\{#if \$profileFailed\}\s*<p[^>]*role="alert"/;

  it.each([
    ['profile', profile, NOTICE, "msg('profile.loadFailed')"],
    ['activity', activity, NOTICE, "msg('profile.loadFailed')"],
    ['big picture profile', bigPictureProfile, LINE, "$t('bp.profile.loadFailed')"],
  ])('the %s screen shows the profile statistics failure', (_name, source, shape, text) => {
    expect(source).toMatch(shape);
    expect(source).toContain(text);
  });
});
