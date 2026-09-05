import { openByIGDB } from '../services/sources';
import { navigate } from '../stores/router';
import { toast } from '../stores/toasts';
import { msg } from '../i18n';

export async function openGameByIGDB(igdbId: number, title: string): Promise<void> {
  try {
    const game = await openByIGDB(igdbId, title);
    if (!game?.id) {
      toast(msg('social.userOpenGameFailed'), 'danger');
      return;
    }
    navigate('game', { id: game.id });
  } catch {
    toast(msg('social.userOpenGameFailed'), 'danger');
  }
}
