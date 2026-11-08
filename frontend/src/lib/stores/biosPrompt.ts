import { writable } from 'svelte/store';

// BiosPrompt asks the player to put a BIOS dump in place. settle(true) means
// "check again", settle(false) gives the launch up.
export interface BiosPrompt {
  platform: string;
  stillMissing: boolean;
  settle: (retry: boolean) => void;
}

export const biosPrompt = writable<BiosPrompt | null>(null);

let open: BiosPrompt | null = null;

export function askForBios(platform: string, stillMissing: boolean): Promise<boolean> {
  open?.settle(false);
  return new Promise((resolve) => {
    const prompt: BiosPrompt = {
      platform,
      stillMissing,
      settle(retry) {
        if (open !== prompt) return;
        open = null;
        biosPrompt.set(null);
        resolve(retry);
      },
    };
    open = prompt;
    biosPrompt.set(prompt);
  });
}
