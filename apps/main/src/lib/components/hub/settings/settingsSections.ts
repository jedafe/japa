import type { Component } from 'svelte';
import type { HubGroup } from '$lib/types';
import ProfileSection from './sections/ProfileSection.svelte';
import AccountSection from './sections/AccountSection.svelte';
import PrivacySection from './sections/PrivacySection.svelte';
import NotificationsSection from './sections/NotificationsSection.svelte';
import ConnectedSection from './sections/ConnectedSection.svelte';
import AffiliateSection from './sections/AffiliateSection.svelte';
import WalletSection from './sections/WalletSection.svelte';
import SubscriptionSection from './sections/SubscriptionSection.svelte';
import SecuritySection from './sections/SecuritySection.svelte';
import ApiKeysSection from './sections/ApiKeysSection.svelte';
import DangerSection from './sections/DangerSection.svelte';

/**
 * Client meta for one settings section. `component` is null for sections
 * listed in the rail but not yet built (Plan 2/3) — the rail renders them
 * disabled and the [section] dispatch 404s on direct visit.
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Settings hub)
 */
export interface SectionMeta {
	slug: string;
	labelKey: string;
	icon: string; // lucide name
	group: HubGroup;
	backendReady: boolean;
	component: Component<{ data: unknown }> | null; // null = not built in this plan (rail shows disabled)
}

export const DEFAULT_SECTION = 'profile';

export const SETTINGS_SECTIONS: SectionMeta[] = [
	{ slug: 'profile', labelKey: 'settings.nav.profile', icon: 'user', group: 'account', backendReady: true, component: ProfileSection },
	{ slug: 'account', labelKey: 'settings.nav.account', icon: 'settings', group: 'account', backendReady: true, component: AccountSection },
	{ slug: 'privacy', labelKey: 'settings.nav.privacy', icon: 'lock', group: 'account', backendReady: true, component: PrivacySection },
	{ slug: 'security', labelKey: 'settings.nav.security', icon: 'shield', group: 'security', backendReady: true, component: SecuritySection },
	{ slug: 'connected', labelKey: 'settings.nav.connected', icon: 'link', group: 'security', backendReady: true, component: ConnectedSection },
	{ slug: 'devices', labelKey: 'settings.nav.devices', icon: 'smartphone', group: 'security', backendReady: false, component: null },
	{ slug: 'notifications', labelKey: 'settings.nav.notifications', icon: 'bell', group: 'notifications', backendReady: true, component: NotificationsSection },
	{ slug: 'api-keys', labelKey: 'settings.nav.api_keys', icon: 'key', group: 'developer', backendReady: true, component: ApiKeysSection },
	{ slug: 'affiliate', labelKey: 'settings.nav.affiliate', icon: 'share-2', group: 'developer', backendReady: true, component: AffiliateSection },
	{ slug: 'wallet', labelKey: 'settings.nav.wallet', icon: 'wallet', group: 'billing', backendReady: true, component: WalletSection },
	{ slug: 'subscription', labelKey: 'settings.nav.subscription', icon: 'credit-card', group: 'billing', backendReady: true, component: SubscriptionSection },
	{ slug: 'danger', labelKey: 'settings.nav.danger', icon: 'alert-triangle', group: 'danger', backendReady: true, component: DangerSection }
];

export function resolveSection(slug: string): SectionMeta | null {
	return SETTINGS_SECTIONS.find((s) => s.slug === slug) ?? null;
}

const GROUP_ORDER: HubGroup[] = ['account', 'security', 'notifications', 'billing', 'developer', 'danger'];

export function sectionsByGroup(): [HubGroup, SectionMeta[]][] {
	return GROUP_ORDER.map((g) => [g, SETTINGS_SECTIONS.filter((s) => s.group === g)] as [HubGroup, SectionMeta[]])
		.filter(([, list]) => list.length > 0);
}
