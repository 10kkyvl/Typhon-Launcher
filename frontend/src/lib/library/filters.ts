export type LibraryFilter = 'all' | 'installed' | 'recent' | 'hidden' | 'watched';

export function nextLibraryFilter(current: LibraryFilter, selected: LibraryFilter): LibraryFilter {
  return selected !== 'all' && selected === current ? 'all' : selected;
}
