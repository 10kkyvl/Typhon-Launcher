import { writable } from 'svelte/store';

// EmulatorBusyPrompt offers to close an emulator that is already open and
// would stop the game. settle(true) means it was closed.
export interface EmulatorBusyPrompt {
  platform: string;
  settle: (closed: boolean) => void;
}

export const emulatorBusyPrompt = writable<EmulatorBusyPrompt | null>(null);

let open: EmulatorBusyPrompt | null = null;

export function askToCloseEmulator(platform: string): Promise<boolean> {
  open?.settle(false);
  return new Promise((resolve) => {
    const prompt: EmulatorBusyPrompt = {
      platform,
      settle(closed) {
        if (open !== prompt) return;
        open = null;
        emulatorBusyPrompt.set(null);
        resolve(closed);
      },
    };
    open = prompt;
    emulatorBusyPrompt.set(prompt);
  });
}
