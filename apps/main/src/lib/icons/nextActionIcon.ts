// §122 suggestion icon resolution — maps the admin-set payload.icon name to a
// lucide component. Unknown/empty names get a neutral fallback so a typo in a
// rule never breaks the renderer.
import {
	ClipboardCheck,
	Calculator,
	UserPlus,
	Bookmark,
	Zap,
	Star,
	Sparkles,
	Compass,
	Bell,
	Target,
	Rocket,
	Gift,
	type Icon as LucideIcon
} from 'lucide-svelte';

const map: Record<string, typeof LucideIcon> = {
	'clipboard-check': ClipboardCheck,
	calculator: Calculator,
	'user-plus': UserPlus,
	bookmark: Bookmark,
	zap: Zap,
	star: Star,
	sparkles: Sparkles,
	compass: Compass,
	bell: Bell,
	target: Target,
	rocket: Rocket,
	gift: Gift
};

export function nextActionIcon(name: string | undefined): typeof LucideIcon {
	if (!name) return Sparkles;
	return map[name.toLowerCase().replace(/[\s_]+/g, '-')] ?? Sparkles;
}
