import { cycleFocus, dismissTopLayer, enterLayer, topLayer, topModalLayer, withinLayer } from '../layers';
import { focusableIn } from '../layers/focus';
import type { BigPictureCommand } from './input';

// The layer the commands act on: the top dialog (with the menus it owns), else a top layer with
// something to focus. A tooltip alone has nothing to answer, so the shell keeps its commands.
function commandRoot(): HTMLElement | null {
  const dialog = topModalLayer();
  if (dialog) return dialog;
  const top = topLayer();
  return top && focusableIn(top).length > 0 ? top : null;
}

/** A layer takes Big Picture's input: the shell's commands, its Tab trap and its focus recovery stand aside. */
export function layerTakesInput(): boolean {
  return commandRoot() !== null;
}

/**
 * While a shared dialog (an emulator or BIOS prompt, a confirmation) is open over Big Picture, the
 * shell's commands act on it instead of on the inert shell behind: back closes the top layer as Escape
 * would, confirm presses the focused control, the directions move between the dialog's controls; the
 * shelf and menu commands do nothing. False when no such layer is open and the shell handles the
 * command itself.
 */
export function layerCommand(command: BigPictureCommand): boolean {
  const root = commandRoot();
  if (!root) return false;
  switch (command) {
    case 'back':
      dismissTopLayer();
      break;
    case 'confirm': {
      const active = document.activeElement;
      if (active instanceof HTMLElement && active !== root && withinLayer(root, active)) active.click();
      else enterLayer(root);
      break;
    }
    case 'up':
    case 'left':
      cycleFocus(root, true);
      break;
    case 'down':
    case 'right':
      cycleFocus(root, false);
      break;
  }
  return true;
}
