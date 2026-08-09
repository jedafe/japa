import {
	Award,
	Bell,
	BookOpen,
	Bookmark,
	Briefcase,
	Building2,
	Calendar,
	Circle,
	Compass,
	Flag,
	Flame,
	GraduationCap,
	Hash,
	HeartHandshake,
	Home,
	Layers,
	LifeBuoy,
	LogIn,
	Mail,
	MessageSquare,
	MoreHorizontal,
	Newspaper,
	Scale,
	Search,
	Send,
	ShieldAlert,
	Target,
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
 */
const iconMap: Record<string, IconComponent> = {
	award: Award,
	bell: Bell,
	'book-open': BookOpen,
	bookmark: Bookmark,
	briefcase: Briefcase,
	'building-2': Building2,
	calendar: Calendar,
	compass: Compass,
	flag: Flag,
	flame: Flame,
	'graduation-cap': GraduationCap,
	hash: Hash,
	'heart-handshake': HeartHandshake,
	home: Home,
	layers: Layers,
	'life-buoy': LifeBuoy,
	'log-in': LogIn,
	mail: Mail,
	'message-square': MessageSquare,
	newspaper: Newspaper,
	scale: Scale,
	search: Search,
	send: Send,
	'shield-alert': ShieldAlert,
	target: Target,
	trophy: Trophy,
	user: User,
	users: Users
};

export function resolveIcon(name: string | null | undefined): IconComponent {
	return (name ? iconMap[name] : undefined) ?? Circle;
}

export { MoreHorizontal };
