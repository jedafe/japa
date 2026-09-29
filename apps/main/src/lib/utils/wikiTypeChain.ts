// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Hierarchical UX)
// Edge-driven hierarchy helpers for the wiki landing chooser: the drill-down
// chain is derived from the declared wiki_type_parents edges
// (GET /wiki/types → parent_type_ids), not a hardcoded level set — enabling
// a new child type (e.g. neighborhood under city) extends the chooser with
// zero code.
import type { WikiTypeItem } from '$lib/types';

/** Enabled types that are declared children of the given type id,
 *  position-ordered. */
export function wikiTypeChildren(types: WikiTypeItem[], parentTypeId: string): WikiTypeItem[] {
	return types
		.filter((t) => (t.parent_type_ids ?? []).includes(parentTypeId))
		.sort((a, b) => a.position - b.position);
}

/** Enabled types that are not a declared child of any enabled type —
 *  chooser roots (country today). */
export function wikiTypeRoots(types: WikiTypeItem[]): WikiTypeItem[] {
	const enabledIds = new Set(types.map((t) => t.space_type_id));
	return types
		.filter(
			(t) =>
				!(t.parent_type_ids ?? []).some((pid) => enabledIds.has(pid))
		)
		.sort((a, b) => a.position - b.position);
}

/** The type to drill into under a selected space: the "primary lineage"
 *  child — a type declaring this type as its single parent — wins over
 *  multi-parent cross-listed types (university sits under country+state+
 *  city, so it's a contextual extra, not the next drill level). Falls back
 *  to the first child by position when nothing is single-parented. */
export function wikiPrimaryChild(types: WikiTypeItem[], type: WikiTypeItem): WikiTypeItem | null {
	const kids = wikiTypeChildren(types, type.space_type_id);
	if (kids.length === 0) return null;
	return (
		kids.find((k) => (k.parent_type_ids ?? []).length <= 1) ?? kids[0]
	);
}

/** The full drill chain starting at the first root: [country, state, city]
 *  for the default seed. Stops at depth 5 — deeper nests (neighborhood)
 *  surface as chips/contextual pickers on the space page instead. */
export function wikiDrillChain(types: WikiTypeItem[]): WikiTypeItem[] {
	const root = wikiTypeRoots(types)[0];
	if (!root) return [];
	const chain = [root];
	for (let i = 0; i < 5; i++) {
		const next = wikiPrimaryChild(types, chain[chain.length - 1]);
		if (!next) break;
		chain.push(next);
	}
	return chain;
}

/** Child types of `type` other than the primary-lineage child — the
 *  contextual pickers shown once the parent space is chosen (e.g.
 *  universities under a selected country/state). */
export function wikiSidelinedChildren(types: WikiTypeItem[], type: WikiTypeItem): WikiTypeItem[] {
	const primary = wikiPrimaryChild(types, type);
	return wikiTypeChildren(types, type.space_type_id).filter(
		(k) => k.space_type_id !== primary?.space_type_id
	);
}
