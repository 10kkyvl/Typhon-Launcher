import { enterLayer, topModalLayer, withinLayer } from '../layers';

/**
 * A message that held focus is gone (a toast, the live island's message): focus goes back where it
 * came from. Under a dialog that is done only if the place is in the dialog (or a layer it owns); when
 * it is unknown (the message was reached with a press), gone or under the dialog, focus goes into the
 * dialog instead of dropping to the page behind it. With no dialog and nowhere known to go back to,
 * focus is left alone.
 */
export function giveFocusBack(origin: HTMLElement | undefined): void {
  const dialog = topModalLayer();
  const known = origin?.isConnected ? origin : undefined;
  if (known && (!dialog || withinLayer(dialog, known))) known.focus({ preventScroll: true });
  else if (dialog) enterLayer(dialog);
}
