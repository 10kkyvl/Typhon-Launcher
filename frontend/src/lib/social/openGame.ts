import { openByIGDB } from '../services/sources';
import { navigate } from '../stores/router';
import { toast } from '../stores/toasts';

export async function openGameByIGDB(igdbId: number, title: string): Promise<void> {
  try {
    const game = await openByIGDB(igdbId, title);
    if (!game?.id) {
      toast('Не удалось открыть игру', 'danger');
      return;
    }
    navigate('game', { id: game.id });
  } catch {
    toast('Не удалось открыть игру', 'danger');
  }
}
