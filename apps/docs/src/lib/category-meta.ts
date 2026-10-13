import {
	BookMarked,
	BookOpen,
	Code2,
	Compass,
	CreditCard,
	FileText,
	GraduationCap,
	KeyRound,
	Layers,
	Link2,
	Megaphone,
	Package,
	Play,
	Puzzle,
	Rocket,
	ShieldCheck,
	SquareTerminal,
	Star,
	User
} from 'lucide-svelte';

/**
 * Presentation metadata for doc categories: a keyword → icon map so category
 * cards/nav rows get a sensible glyph without hardcoding category names to
 * icons (the categories themselves come from real page data — Rule 16).
 * Matching is a lowercase substring test in priority order; FileText is the
 * fallback for anything unrecognized.
 */
const CATEGORY_ICON_RULES: [RegExp, typeof FileText][] = [
	[/sdk|librar|client/, Package],
	[/api|endpoint|reference/, Code2],
	[/tutorial|course|learn|academy/, GraduationCap],
	[/integrat|connect|webhook|plugin/, Puzzle],
	[/platform|architect|feature|infrastructure/, Layers],
	[/securit|polic|privacy|complian|legal|terms/, ShieldCheck],
	[/billing|plan|pric|invoice|payment/, CreditCard],
	[/changelog|release|update|news|what/, Megaphone],
	[/guide|how.?to|start|onboard/, BookOpen],
	[/glossary|spec|schema|field/, BookMarked]
];

export function categoryIcon(name: string): typeof FileText {
	const lower = name.toLowerCase();
	for (const [re, icon] of CATEGORY_ICON_RULES) {
		if (re.test(lower)) return icon;
	}
	return FileText;
}

/**
 * Explicit icon names admins can pick in the managed category catalog
 * (docs.categories). Vocabulary mirrors the keyword rules plus the
 * pin/resource icon sets so one name resolves consistently across
 * editors. Unknown names fall back to the keyword heuristic.
 */
const NAMED_CATEGORY_ICONS: Record<string, typeof FileText> = {
	book: BookMarked,
	guides: BookOpen,
	code: Code2,
	api: Code2,
	compass: Compass,
	billing: CreditCard,
	card: CreditCard,
	file: FileText,
	graduation: GraduationCap,
	tutorials: GraduationCap,
	key: KeyRound,
	layers: Layers,
	platform: Layers,
	link: Link2,
	megaphone: Megaphone,
	changelog: Megaphone,
	package: Package,
	sdk: Package,
	play: Play,
	puzzle: Puzzle,
	integrations: Puzzle,
	rocket: Rocket,
	shield: ShieldCheck,
	star: Star,
	terminal: SquareTerminal,
	user: User
};

/** Resolve a managed icon name → component; null when unrecognized. */
export function namedCategoryIcon(name: string): typeof FileText | null {
	return NAMED_CATEGORY_ICONS[name.trim().toLowerCase()] ?? null;
}

/**
 * Keyword → i18n key for a short generic category description (portal
 * cards). Same priority order as the icon rules; unknown categories return
 * null so callers can fall back to a page count instead of fake copy.
 */
const CATEGORY_DESC_RULES: [RegExp, string][] = [
	[/api|endpoint|reference/, 'catdesc.api'],
	[/sdk|librar|client/, 'catdesc.sdk'],
	[/tutorial|course|learn|academy/, 'catdesc.tutorials'],
	[/integrat|connect|webhook|plugin/, 'catdesc.integrations'],
	[/platform|architect|feature|infrastructure/, 'catdesc.platform'],
	[/securit|polic|privacy|complian|legal|terms/, 'catdesc.policies'],
	[/billing|plan|pric|invoice|payment/, 'catdesc.billing'],
	[/changelog|release|update|news|what/, 'catdesc.changelog'],
	[/guide|how.?to|start|onboard/, 'catdesc.guides'],
	[/glossary|spec|schema|field/, 'catdesc.reference']
];

export function categoryDescKey(name: string): string | null {
	const lower = name.toLowerCase();
	for (const [re, key] of CATEGORY_DESC_RULES) {
		if (re.test(lower)) return key;
	}
	return null;
}
