import { CancelError, Events } from '@wailsio/runtime';
import { get, writable } from 'svelte/store';
import { TakePending, TakePendingLink } from '../../../bindings/typhon/navigationservice';
import { Resolve, ResolveTitle } from '../../../bindings/typhon/internal/deeplink/service';
import { inWails } from '../services/backend';
import { bigPictureActive } from '../bigpicture/mode';
import { errorCode, msg } from '../i18n';
import { metadataErrorText, REASONS as metadataReasons } from '../metadata/metadataErrors';
import { launchRoute } from '../navigation/launchRoute';
import { sourceErrorText, REASONS as sourceReasons } from '../sources/sourceErrors';
import { navigate, route } from './router';
import { dismissToast, toast } from './toasts';

const OPENING_DELAY_MS = 400;

export const bigPictureGame = writable<string | null>(null);
export const bigPictureSearch = writable<string | null>(null);

let linkRun = 0;
let searchNonce = 0;
let cancelLink: (() => void) | null = null;
let ready = false;

function dropLink() {
  linkRun++;
  cancelLink?.();
  cancelLink = null;
}

function linkErrorText(err: unknown): string {
  const code = errorCode(err);
  if (sourceReasons[code]) return sourceErrorText(err);
  if (metadataReasons[code]) return metadataErrorText(err);
  return msg('errSources.linkFailed');
}

function openGame(id: string) {
  if (get(bigPictureActive)) bigPictureGame.set(id);
  else navigate('game', { id });
}

async function openTitle(query: string) {
  dropLink();
  const run = linkRun;
  const request = ResolveTitle(query);
  cancelLink = () => request.cancel();
  const startKey = get(route).key;
  const stopWatching = route.subscribe((current) => {
    if (current.key !== startKey && run === linkRun) dropLink();
  });
  try {
    const result = await request;
    if (run !== linkRun) return;
    stopWatching();
    if (result.status === 'found' && result.gameId) openGame(result.gameId);
    else openSearchDraft(query);
  } catch (err) {
    if (err instanceof CancelError || run !== linkRun) return;
    console.error('resolve link title', err);
    openSearchDraft(query);
  } finally {
    stopWatching();
    if (run === linkRun) cancelLink = null;
  }
}

function openSearchDraft(query: string) {
  if (get(bigPictureActive)) bigPictureSearch.set(query);
  else navigate('catalog', { draft: query, nonce: String(++searchNonce) });
}

async function takeRoute() {
  try {
    const target = launchRoute(await TakePending());
    if (target) {
      dropLink();
      navigate(target);
    }
  } catch (err) {
    console.error('take pending navigation', err);
  }
}

async function takeLink() {
  let pending;
  try {
    pending = await TakePendingLink();
  } catch (err) {
    console.error('take pending link', err);
    toast(msg('errSources.linkFailed'), 'danger');
    return;
  }
  if (pending.invalid) {
    dropLink();
    toast(msg('errSources.linkInvalid'), 'danger');
    return;
  }
  if (pending.query) {
    await openTitle(pending.query);
    return;
  }
  if (!pending.provider) return;
  await openLink(pending.provider, pending.id);
}

async function openLink(provider: string, id: string) {
  dropLink();
  const run = linkRun;
  const request = Resolve({ provider, id });
  cancelLink = () => request.cancel();
  const startKey = get(route).key;
  const stopWatching = route.subscribe((current) => {
    if (current.key !== startKey && run === linkRun) dropLink();
  });
  let openingToast: number | null = null;
  const timer = setTimeout(() => {
    openingToast = toast(msg('errSources.linkOpening'), 'info');
  }, OPENING_DELAY_MS);
  try {
    const result = await request;
    if (run !== linkRun) return;
    stopWatching();
    if (result.status === 'found' && result.gameId) {
      openGame(result.gameId);
    } else if (result.status === 'not_found') {
      toast(msg('errSources.linkNotFound'), 'danger');
    } else if (result.status === 'not_in_catalog') {
      toast(msg('errSources.linkNotInCatalog'), 'danger');
    } else {
      toast(msg('errSources.linkFailed'), 'danger');
    }
  } catch (err) {
    if (err instanceof CancelError || run !== linkRun) return;
    toast(linkErrorText(err), 'danger');
  } finally {
    stopWatching();
    clearTimeout(timer);
    if (openingToast !== null) dismissToast(openingToast);
    if (run === linkRun) cancelLink = null;
  }
}

async function takePending() {
  if (!ready) return;
  await takeRoute();
  await takeLink();
}

export function setLinksReady(next: boolean) {
  if (!inWails || next === ready) return;
  ready = next;
  if (ready) void takePending();
  else dropLink();
}

export function initLaunchNavigation(): () => void {
  if (!inWails) return () => {};
  const off = Events.On('app:navigate', () => void takePending());
  return () => {
    off();
    ready = false;
    dropLink();
  };
}
