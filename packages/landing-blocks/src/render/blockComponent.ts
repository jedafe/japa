// packages/landing-blocks/src/render/blockComponent.ts
// Lazily-bound block type → BlockMeta resolver, registered by blockRegistry
// once it finishes initializing.
//
// Parent blocks that render nested children (explore_map) must NOT import
// `blockRegistry` directly: the registry imports every block component, so a
// parent → registry edge closes a circular import — which crashes with a
// "Cannot access 'X' before initialization" TDZ error whenever the parent
// module is evaluated before the registry (e.g. WikiExploreMap imports
// ExploreMapBlock directly). This indirection breaks the cycle: parents
// resolve child components at render time, after the registry has registered
// its resolver.
import type { BlockMeta } from './blockRegistry';

type Resolver = (type: string) => BlockMeta | undefined;

let resolver: Resolver | null = null;

/** Called once by blockRegistry after its map is built. */
export function setBlockComponentResolver(fn: Resolver): void {
	resolver = fn;
}

/** Resolve a block type to its registry meta (component, icon, label…). */
export function blockComponentFor(type: string): BlockMeta | undefined {
	return resolver?.(type);
}
