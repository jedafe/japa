// Ref: japaconnect-engineering/sections/80-destination-wiki.md
// Presentation-only mapping from wiki category slug → lucide icon + tint.
// The category set itself comes from GET /wiki/categories (Rule 16) — this
// map only decides how each known slug is rendered.
import {
	Globe,
	Briefcase,
	Wallet,
	HeartPulse,
	Landmark,
	Users,
	Home,
	ClipboardList,
	BookOpen,
	Coins,
	Award,
	BedDouble,
	Coffee,
	FileText
} from 'lucide-svelte';
import { Circle } from 'lucide-svelte';
import { resolveIcon } from '$lib/components/ui/icons';
import type { IconComponent } from '$lib/components/ui/icons';

const ICONS: Record<string, IconComponent> = {
	overview: Globe,
	visa: Briefcase,
	cost_of_living: Wallet,
	healthcare: HeartPulse,
	banking: Landmark,
	community: Users,
	housing: Home,
	admissions: ClipboardList,
	programs: BookOpen,
	tuition_fees: Coins,
	scholarships: Award,
	accommodation: BedDouble,
	campus_life: Coffee,
	careers: Briefcase,
	other: FileText
};

// Soft tint pairs (bg + fg) rotating through the design-token chart palette
// so icon chips stay dark-mode safe without hardcoded hex.
const TINTS: Array<{ bg: string; fg: string }> = [
	{ bg: 'color-mix(in srgb, var(--chart-1) 14%, var(--color-surface))', fg: 'var(--chart-1)' },
	{ bg: 'color-mix(in srgb, var(--chart-2) 14%, var(--color-surface))', fg: 'var(--chart-2)' },
	{ bg: 'color-mix(in srgb, var(--chart-3) 14%, var(--color-surface))', fg: 'var(--chart-3)' },
	{ bg: 'color-mix(in srgb, var(--chart-4) 14%, var(--color-surface))', fg: 'var(--chart-4)' },
	{ bg: 'color-mix(in srgb, var(--chart-5) 14%, var(--color-surface))', fg: 'var(--chart-5)' },
	{ bg: 'color-mix(in srgb, var(--color-primary) 12%, var(--color-surface))', fg: 'var(--color-primary)' }
];

const TINT_BY_SLUG: Record<string, number> = {
	overview: 5,
	visa: 0,
	cost_of_living: 1,
	healthcare: 3,
	banking: 4,
	community: 1,
	housing: 2,
	admissions: 5,
	programs: 0,
	tuition_fees: 4,
	scholarships: 3,
	accommodation: 2,
	campus_life: 3,
	careers: 0,
	other: 4
};

export function categoryIcon(slug: string): IconComponent {
	return ICONS[slug] ?? FileText;
}

/**
 * Category icon honoring the admin-configured icon name (§80 — the
 * `icon` field on GET /wiki/categories). A recognized lucide name wins
 * over the slug map; unknown names fall back to the slug map rather
 * than resolveIcon's Circle so a typo never degrades to a blank dot.
 */
export function categoryIconResolved(slug: string, icon?: string | null): IconComponent {
	if (icon) {
		const resolved = resolveIcon(icon);
		if (resolved !== Circle) return resolved;
	}
	return categoryIcon(slug);
}

export function categoryTint(slug: string): { bg: string; fg: string } {
	const i =
		TINT_BY_SLUG[slug] ??
		Math.abs([...slug].reduce((a, c) => a + c.charCodeAt(0), 0)) % TINTS.length;
	return TINTS[i];
}
