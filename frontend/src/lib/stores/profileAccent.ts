import { get } from 'svelte/store';
import { msg } from '../i18n';
import { appearanceOf } from '../profile/appearance';
import { privacyPatch } from '../profile/settingsPatch';
import { accountErrorText } from '../services/accountMessages';
import { normalizeColor } from '../profile/colorNormalize';
import { displayedAccent } from '../theme/apply';
import { confirmedAccent } from './confirmedAccent';
import { toast } from './toasts';
import { authState, currentUser, saveProfile } from './user';

// Only joins confirmations that arrive back to back; previews never reach here.
export const ACCENT_SYNC_DELAY_MS = 800;

export function launcherAccent(): string | null {
  return normalizeColor(get(confirmedAccent) || get(displayedAccent));
}

async function sync(failed: { accent: string }): Promise<boolean> {
  if (get(authState) !== 'authenticated') return true;
  const profile = get(currentUser)?.profile;
  if (!profile?.appearance || profile.appearance.accentSource !== 'theme') return true;
  const raw = get(confirmedAccent) || get(displayedAccent);
  const accent = launcherAccent();
  if (!accent) {
    if (failed.accent !== raw) {
      failed.accent = raw;
      toast(msg('profileStyle.accentThemeUnreadable'), 'danger');
    }
    return true;
  }
  if (accent === profile.appearance.accent.toLowerCase() || accent === failed.accent) return true;
  try {
    const saved = await saveProfile({
      profile: { ...privacyPatch(profile, profile.visibility), appearance: { ...appearanceOf(profile.appearance), accent } },
    });
    if (saved) failed.accent = '';
    return saved;
  } catch (err) {
    failed.accent = accent;
    toast(accountErrorText(err, msg('profileStyle.accentSyncFailed')), 'danger');
    return true;
  }
}

export function initProfileAccentSync(delay = ACCENT_SYNC_DELAY_MS): () => void {
  const failed = { accent: '' };
  let timer: ReturnType<typeof setTimeout> | null = null;
  const schedule = () => {
    if (timer !== null) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      void sync(failed).then((done) => { if (!done) schedule(); });
    }, delay);
  };
  const stops = [confirmedAccent.subscribe(schedule), currentUser.subscribe(schedule)];
  return () => {
    if (timer !== null) clearTimeout(timer);
    for (const stop of stops) stop();
  };
}
