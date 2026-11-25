import { describe, expect, it } from 'vitest';
import { REASONS as install } from './installErrors';
import { REASONS as move } from '../relocate/moveMessages';

// A library code can come back unwrapped from a call whose screen words errors through another package's
// table (adding or removing a game from an install dialog, a move's last step). Go's contract tests read
// one table per package and cannot see that the screen's own table lacks the code.
describe('library codes reach the table of the screen that receives them', () => {
  it.each([
    'library.already_added',
    'library.folder_busy',
    'library.game_not_found',
    'library.moving',
    'library.no_executable',
    'library.removing',
    'library.state_unreadable',
  ])('install and removal dialogs word %s', (code) => {
    expect(Object.keys(install)).toContain(code);
  });

  it.each(['library.executable_outside', 'library.folder_busy', 'library.relocating'])('move dialogs word %s', (code) => {
    expect(Object.keys(move)).toContain(code);
  });
});
