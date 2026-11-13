export type LibraryFilter = 'all' | 'installed' | 'uninstalled' | 'recent' | 'favorites' | 'hidden' | 'watched';

export function nextLibraryFilter(current: LibraryFilter, selected: LibraryFilter): LibraryFilter {
  return selected !== 'all' && selected === current ? 'all' : selected;
}
