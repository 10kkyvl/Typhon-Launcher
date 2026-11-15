import { sourcesUnder } from './sources';

const parts = sourcesUnder('routes/downloads/');

/**
 * The files the desktop Downloads screen is written in: the page and the parts it is split into, every
 * source under routes/downloads/. A source guard that pins what the screen does reads all of them, so
 * text that moves between the parts stays pinned, and a new part is read without being listed.
 */
export const DOWNLOADS_PAGE_PARTS = Object.keys(parts).sort();

/** The text of every part of the Downloads screen, in the order above. */
export const downloadsPageSource = DOWNLOADS_PAGE_PARTS.map((path) => parts[path]).join('\n');
