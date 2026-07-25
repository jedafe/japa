/** Derive a URL slug from a human title. Pure; safe to call on every keystroke. */
export function suggestSlug(title: string): string {
	const base = title
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, '-') // any run of non-alphanumerics → single hyphen
		.replace(/^-+|-+$/g, ''); // trim leading/trailing hyphens
	if (base.length <= 80) return base;
	return base.slice(0, 80).replace(/-+$/g, '');
}
