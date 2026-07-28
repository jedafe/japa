import {
	Bell,
	Bookmark,
	Circle,
	Compass,
	GraduationCap,
	Hash,
	Home,
	Layers,
	LayoutDashboard,
	LogIn,
	PlayCircle,
	Scale,
	Search,
	Trophy,
	User,
	Users
} from 'lucide-svelte';

/** lucide-svelte v1 icons are legacy component classes; type them as such. */
export type IconComponent = typeof import('lucide-svelte').Icon;

/**
 * Maps the backend's kebab-case menu icon names (menu_items.icon) to Lucide
 * components. Unknown names fall back to a neutral dot so nav never breaks on a
 * new icon added server-side.
 *
 * Learn nav (ADR-060): Explore · My Learning · Paths · Certificates · Bookmarks.
 */
const iconMap: Record<string, IconComponent> = {
	bell: Bell,
	bookmark: Bookmark,
	compass: Compass,
	'graduation-cap': GraduationCap,
	hash: Hash,
	home: Home,
	layers: Layers,
	'layout-dashboard': LayoutDashboard,
	'log-in': LogIn,
	'play-circle': PlayCircle,
	scale: Scale,
	search: Search,
	trophy: Trophy,
	user: User,
	users: Users
};

export function resolveIcon(name: string | null | undefined): IconComponent {
	return (name ? iconMap[name] : undefined) ?? Circle;
}
