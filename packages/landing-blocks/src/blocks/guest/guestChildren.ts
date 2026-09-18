// Shared data-source resolution for guest list blocks (categories,
// opportunities, stories, tools, hero stats, hero destinations/community).
// Each block can render three ways, resolved identically:
//   - manual children  → child blocks rendered as-is
//   - context template → first child cloned once per context data item
//   - legacy items     → raw data arrays rendered by the block's own markup
import type { Block } from '../../types';

export interface GuestListResolution<T> {
  dataSource: 'context' | 'manual';
  /** Manual mode: child blocks rendered as-is (may be an empty array). */
  manualChildren?: Block[];
  /** Context mode with a template child: cloned blocks, one per data item. */
  contextChildren?: Block[];
  /** Fallback raw data items for the block's legacy markup path. */
  legacyItems?: T[];
}

export function resolveGuestItems<T>(args: {
  dataSource?: 'context' | 'manual';
  children?: Block[];
  /** Legacy `items` prop — used in manual mode when `children` is undefined. */
  items?: T[];
  /** Context data items (e.g. guestHome.categories). */
  contextItems: T[];
  maxItems?: number;
  /** Merge a context data item into the template child's props. */
  cloneForContext: (template: Block, item: T, index: number) => Block;
}): GuestListResolution<T> {
  const { children, items, contextItems, maxItems, cloneForContext } = args;
  const cap = (list: T[]) => list.slice(0, maxItems ?? list.length);

  // Children present without an explicit source behave as manual; no children
  // at all falls back to context so seeded blocks drive the preview.
  const dataSource = args.dataSource ?? (children && children.length > 0 ? 'manual' : 'context');

  if (dataSource === 'manual') {
    // `children === undefined` (never set) falls back to the legacy items
    // array; an explicitly empty array intentionally renders nothing.
    if (children === undefined) return { dataSource, legacyItems: cap(items ?? contextItems) };
    return { dataSource, manualChildren: children };
  }

  const template = children && children.length > 0 ? children[0] : undefined;
  if (template) {
    return { dataSource, contextChildren: cap(contextItems).map((item, i) => cloneForContext(template, item, i)) };
  }
  return { dataSource, legacyItems: cap(contextItems) };
}
