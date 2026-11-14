import { frontendSources } from './sources';

/**
 * The files the desktop game page is written in: the page and the parts it is split into. A source
 * guard that pins what the page does reads all of them, so text that moves between the parts stays
 * pinned. A new part of the page is added here.
 */
export const GAME_PAGE_PARTS = [
  'routes/game/GameDetails.svelte',
  'routes/game/gamePageData.svelte.ts',
  'routes/game/GamePageHero.svelte',
  'routes/game/GameActions.svelte',
  'routes/game/GameMetaNotice.svelte',
  'routes/game/GameSections.svelte',
  'routes/game/GameInstall.svelte',
  'routes/game/GameOtherVersions.svelte',
  'routes/game/GameDialogs.svelte',
] as const;

/** The text of every part of the game page, in the order above. */
export const gamePageSource = GAME_PAGE_PARTS.map((path) => {
  const text = frontendSources[path];
  if (typeof text !== 'string') throw new Error(`game page part not found: ${path}`);
  return text;
}).join('\n');
