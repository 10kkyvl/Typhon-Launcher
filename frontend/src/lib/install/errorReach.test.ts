import { describe, expect, it } from 'vitest';
import { REASONS as install } from './installErrors';
import { REASONS as move } from '../relocate/moveMessages';
import { updateErrorText } from '../updates/updateErrors';
import { msg } from '../i18n';

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

  it.each(['library.executable_outside', 'library.folder_busy', 'library.relocating', 'library.state_unreadable'])('move dialogs word %s', (code) => {
    expect(Object.keys(move)).toContain(code);
  });

  // A game update and its rollback end by writing the version into the library card; the update card,
  // its notification and the history word the stored error through the update table.
  it('update screens word library.state_unreadable', () => {
    const stored = 'typhon:library.state_unreadable: library not saved, its files could not be read';
    expect(updateErrorText(stored, 'fallback')).toBe(msg('library.state_unreadable'));
  });
});
