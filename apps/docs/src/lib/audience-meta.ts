import {
	BookOpen,
	Building2,
	Code2,
	Crown,
	KeyRound,
	Settings,
	ShieldCheck
} from 'lucide-svelte';
import type { DocAudience } from '$lib/types';

/**
 * Presentation metadata per canonical docs audience (§44). Icons and the i18n
 * keys for short section descriptions shown on the landing + audience header.
 * The audience list itself is always server-resolved — this map only supplies
 * chrome for the canonical slugs.
 */
export const AUDIENCE_ICON: Record<DocAudience, typeof BookOpen> = {
	users: BookOpen,
	premium: Crown,
	organizations: Building2,
	moderators: ShieldCheck,
	api: Code2,
	admin: Settings,
	superadmin: KeyRound
};

/** i18n key for each audience's one-line section description. */
export const AUDIENCE_DESC_KEY: Record<DocAudience, string> = {
	users: 'audience.desc.users',
	premium: 'audience.desc.premium',
	organizations: 'audience.desc.organizations',
	moderators: 'audience.desc.moderators',
	api: 'audience.desc.api',
	admin: 'audience.desc.admin',
	superadmin: 'audience.desc.superadmin'
};
