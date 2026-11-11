import { cycleFocus, dismissTopLayer, enterLayer, topLayer } from '../layers';
import type { BigPictureCommand } from './input';

/**
 * While a shared dialog (an emulator or BIOS prompt, a confirmation) is open over Big Picture, the
 * shell's commands act on it instead of on the inert shell behind: back closes it as Escape would,
 * confirm presses the focused control, the directions move between its controls; the shelf and menu
 * commands do nothing. False when no layer is open and the shell handles the command itself.
 */
export function layerCommand(command: BigPictureCommand): boolean {
  const top = topLayer();
  if (!top) return false;
  switch (command) {
    case 'back':
      dismissTopLayer();
      break;
    case 'confirm': {
      const active = document.activeElement;
      if (active instanceof HTMLElement && active !== top && top.contains(active)) active.click();
      else enterLayer(top);
      break;
    }
    case 'up':
    case 'left':
      cycleFocus(true);
      break;
    case 'down':
    case 'right':
      cycleFocus(false);
      break;
  }
  return true;
}
