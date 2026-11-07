// §122 suggestion icon resolution — maps the admin-set payload.icon name to a
// lucide component. Unknown/empty names get a neutral fallback so a typo in a
// rule never breaks the renderer.
import {
	ClipboardCheck,
	Calculator,
	UserPlus,
	UserCheck,
	Bookmark,
	Zap,
	Star,
	Sparkles,
	Compass,
	Bell,
	BellRing,
	Target,
	Rocket,
	Gift,
	Mail,
	FileText,
	Plane,
	Route,
	Languages,
	ScrollText,
	ListChecks,
	Landmark,
	Timer,
	Users,
	GraduationCap,
	MapPin,
	Home,
	Send,
	BookOpen,
	ShieldAlert,
	Newspaper,
	PiggyBank,
	type Icon as LucideIcon
} from 'lucide-svelte';

const map: Record<string, typeof LucideIcon> = {
	'clipboard-check': ClipboardCheck,
	calculator: Calculator,
	'user-plus': UserPlus,
	'user-check': UserCheck,
	bookmark: Bookmark,
	zap: Zap,
	star: Star,
	sparkles: Sparkles,
	compass: Compass,
	bell: Bell,
	'bell-ring': BellRing,
	target: Target,
	rocket: Rocket,
	gift: Gift,
	mail: Mail,
	'file-text': FileText,
	plane: Plane,
	route: Route,
	languages: Languages,
	'scroll-text': ScrollText,
	'list-checks': ListChecks,
	landmark: Landmark,
	timer: Timer,
	users: Users,
	'graduation-cap': GraduationCap,
	'map-pin': MapPin,
	home: Home,
	send: Send,
	'book-open': BookOpen,
	'shield-alert': ShieldAlert,
	newspaper: Newspaper,
	'piggy-bank': PiggyBank
};

export function nextActionIcon(name: string | undefined): typeof LucideIcon {
	if (!name) return Sparkles;
	return map[name.toLowerCase().replace(/[\s_]+/g, '-')] ?? Sparkles;
}
