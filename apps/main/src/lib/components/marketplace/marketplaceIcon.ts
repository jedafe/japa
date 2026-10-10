// §114 2026-10-10 — fixed whitelist mapping the listing page-content
// icon fields (selling_points[].icon, facts[].icon, included[].icon —
// kebab-case Lucide names, author-managed) to Lucide components. Unknown
// or absent names fall back to Sparkles so a bad server value can never
// break the detail page. Covers the icons used across the service,
// mentorship, and package detail-page mockups.
import {
	Award,
	BadgeCheck,
	BookOpen,
	Briefcase,
	Calendar,
	CalendarDays,
	CheckCircle2,
	ClipboardCheck,
	Clock,
	FileCheck,
	FileText,
	Globe,
	GraduationCap,
	HandCoins,
	Handshake,
	Headset,
	Heart,
	HeartPulse,
	Home,
	Landmark,
	Languages,
	MapPin,
	MessageCircle,
	Plane,
	Scale,
	ShieldCheck,
	Sparkles,
	Star,
	Target,
	TrendingUp,
	UserCheck,
	Users,
	UsersRound,
	Video,
	Wifi,
	Zap
} from 'lucide-svelte';
import type { IconComponent } from '$lib/components/ui/icons';

const MARKETPLACE_ICONS: Record<string, IconComponent> = {
	award: Award,
	'badge-check': BadgeCheck,
	'book-open': BookOpen,
	briefcase: Briefcase,
	calendar: Calendar,
	'calendar-days': CalendarDays,
	'check-circle': CheckCircle2,
	'check-circle-2': CheckCircle2,
	'clipboard-check': ClipboardCheck,
	clock: Clock,
	'file-check': FileCheck,
	'file-text': FileText,
	globe: Globe,
	'graduation-cap': GraduationCap,
	'hand-coins': HandCoins,
	handshake: Handshake,
	headset: Headset,
	heart: Heart,
	'heart-pulse': HeartPulse,
	home: Home,
	landmark: Landmark,
	languages: Languages,
	'map-pin': MapPin,
	'message-circle': MessageCircle,
	plane: Plane,
	scale: Scale,
	'shield-check': ShieldCheck,
	sparkles: Sparkles,
	star: Star,
	target: Target,
	'trending-up': TrendingUp,
	'user-check': UserCheck,
	users: Users,
	'users-round': UsersRound,
	video: Video,
	wifi: Wifi,
	zap: Zap
};

export function marketplaceIcon(name: string | undefined): IconComponent {
	return (name ? MARKETPLACE_ICONS[name] : undefined) ?? Sparkles;
}

/** Sorted whitelist keys — the editor's icon <select> renders these so
 *  authors can only pick names the detail page can resolve. */
export const MARKETPLACE_ICON_NAMES: string[] = Object.keys(MARKETPLACE_ICONS).sort();
