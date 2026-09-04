import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { ensureArt, getGameArt, isMetadataAvailable, type GameArt, type MetadataView } from '../services/metadata';
import type { CatalogGame } from '../services/sources';
import { msg } from '../i18n';
import { toast } from './toasts';

export const metadataAvailable = writable(false);
export const gameArt = writable<Record<string, GameArt>>({});
export const gameInfo = writable<Record<string, CatalogGame>>({});

const chunkSize = 48;
const retryDelay = 3000;
const retryAfter = 45000;
const maxIdleRounds = 5;

const pending = new Set<string>();
const taken = new Map<string, number>();
let pumping = false;

export async function initMetadata() {
  metadataAvailable.set(await isMetadataAvailable());
  if (!inWails) return;

  Events.On('metadata:updated', (event) => {
    const view = event.data as MetadataView;
    const id = view.game?.id;
    if (!id) return;
    gameInfo.update((map) => ({ ...map, [id]: view.game }));
    gameArt.update((map) => {
      if (!view.cover && !view.hero) {
        if (!(id in map)) return map;
        const next = { ...map };
        delete next[id];
        return next;
      }
      return { ...map, [id]: { cover: view.cover, hero: view.hero } };
    });
    if (pending.size > 0) pump();
  });
}

export async function loadArt(ids: string[]) {
  const known = get(gameArt);
  const missing = [...new Set(ids)].filter((id) => id && !(id in known));
  if (missing.length === 0) return;
  const art = await getGameArt(missing);
  if (Object.keys(art).length === 0) return;
  gameArt.update((map) => ({ ...map, ...art }));
}

export function requestArt(ids: string[]) {
  const known = get(gameArt);
  const now = Date.now();
  let added = false;
  for (const id of ids) {
    if (!id || pending.has(id)) continue;
    const attempt = taken.get(id);
    if (attempt !== undefined && (id in known || now - attempt < retryAfter)) continue;
    pending.add(id);
    added = true;
  }
  if (added || pending.size > 0) pump();
}

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

async function pump() {
  if (pumping) return;
  pumping = true;
  let idleRounds = 0;
  try {
    while (pending.size > 0 && idleRounds < maxIdleRounds) {
      const chunk = [...pending].slice(0, chunkSize);
      await loadArt(chunk);
      const accepted = await ensureArt(chunk);
      const now = Date.now();
      for (const id of accepted) {
        pending.delete(id);
        taken.set(id, now);
      }
      if (accepted.length === chunk.length) {
        idleRounds = 0;
        continue;
      }
      idleRounds += 1;
      await wait(retryDelay);
    }
  } catch (err) {
    pending.clear();
    toast(err instanceof Error && err.message ? err.message : msg('state.metadataLoadFailed'), 'danger');
  } finally {
    pumping = false;
  }
}
