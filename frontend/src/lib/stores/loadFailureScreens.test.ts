import { describe, expect, it } from 'vitest';
import installed from '../../routes/installed/InstalledStorage.svelte?raw';
import profile from '../../routes/profile/Profile.svelte?raw';
import activity from '../../routes/activity/Activity.svelte?raw';
import bigPictureProfile from '../bigpicture/components/ProfilePage.svelte?raw';

describe('screens show a failed load instead of an empty state', () => {
  it('the installed games page shows the disk usage failure with a retry', () => {
    // A danger Notice is an alert by its tone; its retry refreshes the disk usage.
    expect(installed).toMatch(/\{:else if \$storageFailed \|\| storageRetrying\}[\s\S]*?<Notice tone="danger"[^>]*retry=\{\{[^}]*run: retryStorage[\s\S]*?games\.installedStorageLoadFailed/);
    expect(installed).toMatch(/async function retryStorage\(\)[\s\S]*?await refreshStorage\(\);/);
  });

  // A danger Notice is an alert by its tone; Big Picture has no backdrop and keeps its own line.
  const NOTICE = /\{#if \$profileFailed\}\s*(<div[^>]*>)?<Notice tone="danger">/;
  const LINE = /\{#if \$profileFailed\}\s*<p[^>]*role="alert"/;

  it('the profile screen shows the profile statistics failure with a retry, kept while it runs', () => {
    // The store's refresh answers whether the newest read came: Retry waits for the read the page will show.
    expect(profile).toMatch(/\{#if \$profileFailed \|\| snapshotRetrying\}\s*(<div[^>]*>)?\s*<Notice tone="danger" bind:retrying=\{snapshotRetrying\} retry=\{\{[^}]*run: refreshProfile \}\}[^>]*>\{msg\('profile\.loadFailed'\)\}/);
  });

  it.each([
    ['activity', activity, NOTICE, "msg('profile.loadFailed')"],
    ['big picture profile', bigPictureProfile, LINE, "$t('bp.profile.loadFailed')"],
  ])('the %s screen shows the profile statistics failure', (_name, source, shape, text) => {
    expect(source).toMatch(shape);
    expect(source).toContain(text);
  });
});
