// Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md (§3.2)
// Fixed whitelist mapping service_categories.icon (kebab-case Lucide name,
// admin-managed) to Lucide components. Unknown/absent names fall back to
// Grid2x2 so a new server-side icon can never break the marketplace UI.
// Covers every icon seeded by migration 000803.
import {
	BookOpen,
	Briefcase,
	FileText,
	GraduationCap,
	Grid2x2,
	HeartPulse,
	Home,
	Landmark,
	Plane,
	Scale,
	ShieldCheck,
	Users,
	UsersRound
} from 'lucide-svelte';
import type { IconComponent } from '$lib/components/ui/icons';

const CATEGORY_ICONS: Record<string, IconComponent> = {
	'book-open': BookOpen,
	briefcase: Briefcase,
	'file-text': FileText,
	'graduation-cap': GraduationCap,
	'heart-pulse': HeartPulse,
	home: Home,
	landmark: Landmark,
	plane: Plane,
	scale: Scale,
	'shield-check': ShieldCheck,
	users: Users,
	'users-round': UsersRound
};

export function serviceCategoryIcon(name: string | undefined): IconComponent {
	return (name ? CATEGORY_ICONS[name] : undefined) ?? Grid2x2;
}
