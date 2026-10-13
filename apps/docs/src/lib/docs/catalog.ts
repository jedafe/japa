import type { DocAudience, DocNavItem, DocNavTree, DocPageSummary } from '$lib/types';

/**
 * Portal derivations for the docs app landing, sidebar, and category pages.
 * Everything here derives from the audience-gated nav tree and page summaries
 * already fetched by the layout/page loads — no new API surface, no hardcoded
 * content (Rule 16). Pure functions so they stay unit-testable (vitest).
 */

/* ------------------------------------------------------------------ *
 * Categories — aggregated across every accessible audience tree.
 * Case-folds like groupByCategory: "Guides" and "guides" are one group;
 * first-seen casing wins for display.
 * ------------------------------------------------------------------ */

export interface DocsCategoryItem {
	audience: DocAudience;
	slug: string;
	title: string;
	subcategory?: string | null;
}

export interface DocsCategory {
	/** Display name (first-seen casing). */
	name: string;
	/** URL slug used in /docs/category/[name]. */
	slug: string;
	/** Subcategory display names in first-seen order (no empty). */
	subcategories: string[];
	/** Pages in this category across all accessible audiences. */
	items: DocsCategoryItem[];
	/** Managed-catalog description override (docs.categories). */
	description?: string;
	/** Managed-catalog icon name override (docs.categories). */
	icon?: string;
}

/** URL-safe slug for a category name ("API Reference" → "api-reference"). */
export function categorySlug(name: string): string {
	return encodeURIComponent(name.trim().toLowerCase().replace(/\s+/g, '-'));
}

export function categoryHref(name: string): string {
	return `/docs/category/${categorySlug(name)}`;
}

/**
 * Aggregate categories across all nav trees. Ordering contract mirrors
 * groupByCategory: categories in first-appearance order (walking trees then
 * items), subcategories likewise. Children are not scanned — they inherit
 * their parent's grouping, matching the nav tree's own rendering.
 */
export function collectCategories(nav: DocNavTree[]): DocsCategory[] {
	const out: DocsCategory[] = [];
	const idx = new Map<string, DocsCategory>();
	const subIdx = new Map<string, Set<string>>();

	for (const tree of nav) {
		for (const item of tree.items) {
			const name = item.category?.trim();
			if (!name) continue;
			const key = name.toLowerCase();
			let cat = idx.get(key);
			if (!cat) {
				cat = { name, slug: categorySlug(name), subcategories: [], items: [] };
				idx.set(key, cat);
				subIdx.set(key, new Set());
				out.push(cat);
			}
			const sub = item.subcategory?.trim();
			if (sub && !subIdx.get(key)!.has(sub.toLowerCase())) {
				subIdx.get(key)!.add(sub.toLowerCase());
				cat.subcategories.push(sub);
			}
			cat.items.push({
				audience: tree.audience,
				slug: item.slug,
				title: item.title,
				subcategory: item.subcategory ?? null
			});
		}
	}
	return out;
}

/** Find a category by its URL slug (case/whitespace-insensitive). */
export function findCategory(nav: DocNavTree[], slug: string): DocsCategory | null {
	const decoded = decodeURIComponent(slug).toLowerCase();
	return collectCategories(nav).find((c) => c.slug === categorySlug(decoded) || c.name.toLowerCase() === decoded) ?? null;
}

/* ------------------------------------------------------------------ *
 * Managed category catalog — docs.categories setting.
 * The page-level taxonomy stays free-text; this optional admin-managed
 * catalog adds display order, descriptions, icons, and a canonical
 * subcategory list. mergeManagedCategories() unions it with the derived
 * set: managed entries appear first in curated order (empty ones still
 * render so a freshly-created category shows on the portal), then any
 * page-derived categories the catalog doesn't cover.
 * ------------------------------------------------------------------ */

export interface ManagedCategory {
	name: string;
	description?: string;
	icon?: string;
	subcategories: string[];
}

/** Parse docs.categories — accepts a JSONB array or a raw JSON string. */
export function parseManagedCategories(raw: unknown): ManagedCategory[] {
	const list = typeof raw === 'string' ? safeJsonParse(raw) : raw;
	if (!Array.isArray(list)) return [];
	const out: ManagedCategory[] = [];
	for (const item of list) {
		if (!item || typeof item !== 'object') continue;
		const o = item as Record<string, unknown>;
		const name = typeof o.name === 'string' ? o.name.trim() : '';
		if (!name) continue;
		out.push({
			name,
			description:
				typeof o.description === 'string' && o.description.trim()
					? o.description.trim()
					: undefined,
			icon:
				typeof o.icon === 'string' && o.icon.trim()
					? o.icon.trim().toLowerCase()
					: undefined,
			subcategories: Array.isArray(o.subcategories)
				? o.subcategories
						.filter((s): s is string => typeof s === 'string' && !!s.trim())
						.map((s) => s.trim())
				: []
		});
	}
	return out;
}

/**
 * Union the managed catalog with the nav-derived categories. A managed
 * entry matching a derived category (case-insensitive) inherits its page
 * items; managed-only entries render with zero items; derived-only
 * categories append after the managed block in derived order.
 */
export function mergeManagedCategories(
	derived: DocsCategory[],
	managed: ManagedCategory[]
): DocsCategory[] {
	if (!managed.length) return derived;
	const derivedByName = new Map(derived.map((c) => [c.name.toLowerCase(), c]));
	const managedNames = new Set<string>();
	const out: DocsCategory[] = managed.map((m) => {
		const key = m.name.toLowerCase();
		managedNames.add(key);
		const d = derivedByName.get(key);
		// Subcategories: managed list first, then any derived extras.
		const seen = new Set(m.subcategories.map((s) => s.toLowerCase()));
		const subcategories = [
			...m.subcategories,
			...(d?.subcategories ?? []).filter((s) => !seen.has(s.toLowerCase()))
		];
		return {
			name: m.name,
			slug: d?.slug ?? categorySlug(m.name),
			subcategories,
			items: d?.items ?? [],
			description: m.description,
			icon: m.icon
		};
	});
	for (const c of derived) {
		if (!managedNames.has(c.name.toLowerCase())) out.push(c);
	}
	return out;
}

/* ------------------------------------------------------------------ *
 * Get Started pins — docs.pinned_pages setting.
 * Accepts [{audience, slug, label?}] or shorthand ["audience/slug"].
 * Unresolvable pins (inaccessible audience, missing slug) are skipped so
 * the sidebar never renders a dead link.
 * ------------------------------------------------------------------ */

export interface DocsPin {
	audience: DocAudience;
	slug: string;
	label?: string;
	/** Optional icon name (file, book, rocket, compass, user, star). */
	icon?: string;
}

export interface ResolvedPin {
	href: string;
	label: string;
	icon?: string;
}

export function parsePinnedPages(raw: unknown): DocsPin[] {
	const list = typeof raw === 'string' ? safeJsonParse(raw) : raw;
	if (!Array.isArray(list)) return [];
	const pins: DocsPin[] = [];
	for (const entry of list) {
		if (typeof entry === 'string') {
			const [audience, slug] = entry.split('/');
			if (audience && slug) pins.push({ audience: audience as DocAudience, slug });
		} else if (entry && typeof entry === 'object') {
			const e = entry as Record<string, unknown>;
			if (typeof e.audience === 'string' && typeof e.slug === 'string' && e.slug) {
				pins.push({
					audience: e.audience as DocAudience,
					slug: e.slug,
					label: typeof e.label === 'string' && e.label ? e.label : undefined,
					icon: typeof e.icon === 'string' && e.icon ? e.icon : undefined
				});
			}
		}
	}
	return pins;
}

export function resolvePinnedPages(
	pins: DocsPin[],
	nav: DocNavTree[],
	accessibleAudiences: DocAudience[]
): ResolvedPin[] {
	const out: ResolvedPin[] = [];
	for (const pin of pins) {
		if (!accessibleAudiences.includes(pin.audience)) continue;
		const tree = nav.find((t) => t.audience === pin.audience);
		const item = findNavItem(tree?.items ?? [], pin.slug);
		if (!item) continue;
		out.push({ href: `/docs/${pin.audience}/${item.slug}`, label: pin.label ?? item.title, icon: pin.icon });
	}
	return out;
}

function findNavItem(items: DocNavItem[], slug: string): DocNavItem | null {
	for (const item of items) {
		if (item.slug === slug) return item;
		const child = item.children ? findNavItem(item.children, slug) : null;
		if (child) return child;
	}
	return null;
}

/* ------------------------------------------------------------------ *
 * Versions — docs.versions setting. Accepts [{label, url?}] or ["v1.0"].
 * Empty → the header renders no version dropdown.
 * ------------------------------------------------------------------ */

export interface DocsVersion {
	label: string;
	url?: string;
}

export function parseVersions(raw: unknown): DocsVersion[] {
	const list = typeof raw === 'string' ? safeJsonParse(raw) : raw;
	if (!Array.isArray(list)) return [];
	const out: DocsVersion[] = [];
	for (const entry of list) {
		if (typeof entry === 'string' && entry.trim()) {
			out.push({ label: entry.trim() });
		} else if (entry && typeof entry === 'object') {
			const e = entry as Record<string, unknown>;
			if (typeof e.label === 'string' && e.label.trim()) {
				out.push({
					label: e.label.trim(),
					url: typeof e.url === 'string' && e.url ? e.url : undefined
				});
			}
		}
	}
	return out;
}

/* ------------------------------------------------------------------ *
 * Link lists — docs.help_links / docs.contribute_links use {label, url}
 * rows; docs.dev_resources adds optional description + icon. An absent
 * setting keeps the caller's built-in defaults, a stored [] is explicit.
 * ------------------------------------------------------------------ */

export interface DocsLink {
	label: string;
	url: string;
}

export interface DocsResource extends DocsLink {
	description?: string;
	icon?: string;
}

export function parseLinkList(raw: unknown): DocsLink[] {
	const list = typeof raw === 'string' ? safeJsonParse(raw) : raw;
	if (!Array.isArray(list)) return [];
	const out: DocsLink[] = [];
	for (const entry of list) {
		if (!entry || typeof entry !== 'object') continue;
		const e = entry as Record<string, unknown>;
		if (typeof e.label === 'string' && e.label.trim() && typeof e.url === 'string' && e.url.trim()) {
			out.push({ label: e.label.trim(), url: e.url.trim() });
		}
	}
	return out;
}

export function parseLinkCards(raw: unknown): DocsResource[] {
	const list = typeof raw === 'string' ? safeJsonParse(raw) : raw;
	if (!Array.isArray(list)) return [];
	const out: DocsResource[] = [];
	for (const entry of list) {
		if (!entry || typeof entry !== 'object') continue;
		const e = entry as Record<string, unknown>;
		if (typeof e.label === 'string' && e.label.trim() && typeof e.url === 'string' && e.url.trim()) {
			out.push({
				label: e.label.trim(),
				url: e.url.trim(),
				description: typeof e.description === 'string' && e.description.trim() ? e.description.trim() : undefined,
				icon: typeof e.icon === 'string' && e.icon.trim() ? e.icon.trim() : undefined
			});
		}
	}
	return out;
}

/* ------------------------------------------------------------------ *
 * Changelog ("What's new") — pages whose category matches the
 * docs.changelog_category csv, newest first. `isNew` is relative to
 * docs.changelog_new_days.
 * ------------------------------------------------------------------ */

export interface ChangelogEntry {
	audience: DocAudience;
	slug: string;
	title: string;
	createdAt: string;
	isNew: boolean;
	/** Plain-text preview from the backend (may be empty). */
	excerpt?: string;
}

export function parseCsvSetting(raw: unknown, fallback: string[] = []): string[] {
	if (Array.isArray(raw)) {
		return raw.filter((v): v is string => typeof v === 'string').map((v) => v.trim()).filter(Boolean);
	}
	if (typeof raw === 'string' && raw.trim()) {
		return raw.split(',').map((v) => v.trim()).filter(Boolean);
	}
	return fallback;
}

export function parseIntSetting(raw: unknown, fallback: number): number {
	const n = typeof raw === 'number' ? raw : typeof raw === 'string' ? parseInt(raw, 10) : NaN;
	return Number.isFinite(n) && n >= 0 ? n : fallback;
}

export function collectChangelog(
	summaries: DocPageSummary[],
	categories: string[],
	newDays: number,
	now: Date = new Date()
): ChangelogEntry[] {
	const wanted = new Set(categories.map((c) => c.toLowerCase()));
	const cutoff = now.getTime() - newDays * 24 * 60 * 60 * 1000;
	return summaries
		.filter((p) => p.category && wanted.has(p.category.trim().toLowerCase()))
		.map((p) => ({
			audience: p.audience,
			slug: p.slug,
			title: p.title,
			createdAt: p.created_at,
			isNew: Date.parse(p.created_at) >= cutoff,
			excerpt: p.excerpt || undefined
		}))
		.sort((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt));
}

/* ------------------------------------------------------------------ *
 * Landing derivations — audience stats + popular pages.
 * ------------------------------------------------------------------ */

export interface AudienceStats {
	/** Top-level nav entries for the audience ("guides"). */
	guides: number;
	/** Total pages including nested children. */
	pages: number;
}

export function audienceStats(tree: DocNavTree | undefined): AudienceStats {
	const count = (items: DocNavItem[] | undefined): number =>
		(items ?? []).reduce((acc, i) => acc + 1 + count(i.children), 0);
	const top = tree?.items ?? [];
	return { guides: top.length, pages: count(top) };
}

export interface PopularPage {
	audience: DocAudience;
	slug: string;
	title: string;
	category?: string | null;
}

/**
 * Popular pages for the landing list: up to `perAudience` leading nav items
 * per audience, flattened in canonical tree order, capped at `total`.
 */
export function popularPages(nav: DocNavTree[], perAudience = 2, total = 6): PopularPage[] {
	return nav
		.flatMap((tree) =>
			tree.items.slice(0, perAudience).map((item) => ({
				audience: tree.audience,
				slug: item.slug,
				title: item.title,
				category: item.category ?? null
			}))
		)
		.slice(0, total);
}

/* ------------------------------------------------------------------ */

function safeJsonParse(raw: string): unknown {
	try {
		return JSON.parse(raw);
	} catch {
		return null;
	}
}
