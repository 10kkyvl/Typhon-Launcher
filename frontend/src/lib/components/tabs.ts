/** One tab of a tab strip. */
export interface TabItem {
  id: string;
  label: string;
  count?: number;
  disabled?: boolean;
}

/** The DOM id of a tab; `prefix` is the id the page gave its Tabs (`idPrefix`). */
export const tabId = (prefix: string, id: string): string => `${prefix}-tab-${id}`;

/** The DOM id of the panel a tab shows. */
export const panelId = (prefix: string, id: string): string => `${prefix}-panel-${id}`;

/**
 * What the element holding a tab's content carries, so the tab and its panel name each other:
 * `<div {...tabPanel(uid, tab)}>`. The panel takes focus on Tab from its tab, whatever is inside it.
 */
export function tabPanel(prefix: string, id: string) {
  return { id: panelId(prefix, id), role: 'tabpanel', 'aria-labelledby': tabId(prefix, id), tabindex: 0 } as const;
}
