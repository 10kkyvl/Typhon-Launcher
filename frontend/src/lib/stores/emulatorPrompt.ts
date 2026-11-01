import { writable } from 'svelte/store';

export interface EmulatorPrompt {
  platform: string;
  code: string;
  settle: (configured: boolean) => void;
}

export const emulatorPrompt = writable<EmulatorPrompt | null>(null);

let open: EmulatorPrompt | null = null;

export function askForEmulator(platform: string, code: string): Promise<boolean> {
  open?.settle(false);
  return new Promise((resolve) => {
    const prompt: EmulatorPrompt = {
      platform,
      code,
      settle(configured) {
        if (open !== prompt) return;
        open = null;
        emulatorPrompt.set(null);
        resolve(configured);
      },
    };
    open = prompt;
    emulatorPrompt.set(prompt);
  });
}
