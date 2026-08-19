// packages/landing-blocks/src/shell/blockContent.ts
// Context used by block shells to let child block components report whether
// they actually have content. This lets `hide_if_empty` work for dynamic
// blocks that only know their emptiness after client-side data fetches.

import { getContext, setContext } from 'svelte';

export interface BlockContentContext {
  /** Report whether this block has content worth showing. */
  set: (hasContent: boolean) => void;
}

const BLOCK_CONTENT_KEY = Symbol('block-content');

export function setBlockContentContext(ctx: BlockContentContext): BlockContentContext {
  setContext(BLOCK_CONTENT_KEY, ctx);
  return ctx;
}

export function getBlockContentContext(): BlockContentContext | undefined {
  return getContext<BlockContentContext>(BLOCK_CONTENT_KEY);
}
