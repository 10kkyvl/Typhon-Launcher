export type LibraryFilter = 'all' | 'installed' | 'recent';

export function nextLibraryFilter(current: LibraryFilter, selected: LibraryFilter): LibraryFilter {
  return selected !== 'all' && selected === current ? 'all' : selected;
}
