import { COMPARE_FAMILIES } from '#lib/components/compare/compareFamilies.js';

const slugs = new Set(COMPARE_FAMILIES.map((f) => f.slug));

// /compare/[category=compareCategory] sorts before /compare/[slug], so
// /compare/location renders the family browse page while comparison slugs
// keep hitting the quick-result route. A comparison literally slugged
// "location" would be shadowed — accepted (slugs are generated).
export function match(param: string): boolean {
	return slugs.has(param);
}
