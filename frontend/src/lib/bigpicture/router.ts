export type Section = 'home' | 'library' | 'catalog' | 'downloads' | 'settings' | 'profile' | 'social';
export type Page = { name: Section } | { name: 'game'; id: string };
export interface PageEntry { key: number; page: Page; focus: string }

let sequence = 0;
export const entry = (page: Page): PageEntry => ({ key: ++sequence, page, focus: '' });

export function pushPage(pages: PageEntry[], page: Page, focus: string): PageEntry[] {
  const previous = pages.map((item, index) => index === pages.length - 1 ? { ...item, focus } : item);
  return [...previous, entry(page)];
}

export function popPage(pages: PageEntry[]): PageEntry[] {
  return pages.length > 1 ? pages.slice(0, -1) : pages;
}
