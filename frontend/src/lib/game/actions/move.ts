import { openMoveGame as openMove } from '../../stores/relocate';

export function openMoveGame(gameId: string) {
  openMove(gameId);
}
