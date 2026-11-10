import { Window } from '@wailsio/runtime';
import { inWails } from './backend';

export type WindowAction = 'minimise' | 'toggleMaximise' | 'close';

export async function windowAction(action: WindowAction): Promise<void> {
  if (!inWails) throw new Error('unavailable in browser');
  if (action === 'minimise') await Window.Minimise();
  else if (action === 'toggleMaximise') await Window.ToggleMaximise();
  else await Window.Close();
}

export async function isWindowMaximised(): Promise<boolean> {
  if (!inWails) return false;
  return Window.IsMaximised();
}
