import type { Component } from 'svelte';
import type { HubGroup } from '$lib/types';
import ProfileSection from './sections/ProfileSection.svelte';
import AccountSection from './sections/AccountSection.svelte';
import PrivacySection from './sections/PrivacySection.svelte';
import NotificationsSection from './sections/NotificationsSection.svelte';
import NewsletterSection from './sections/NewsletterSection.svelte';
import ConnectedSection from './sections/ConnectedSection.svelte';
import ConnectedAppsSection from './sections/ConnectedAppsSection.svelte';
import AffiliateSection from './sections/AffiliateSection.svelte';
import WalletSection from './sections/WalletSection.svelte';
import TipsSection from './sections/TipsSection.svelte';
import SubscriptionSection from './sections/SubscriptionSection.svelte';
import PaymentsSection from './sections/PaymentsSection.svelte';
import SecuritySection from './sections/SecuritySection.svelte';
import SessionsSection from './sections/SessionsSection.svelte';
import DevicesSection from './sections/DevicesSection.svelte';
import ApiKeysSection from './sections/ApiKeysSection.svelte';
import DangerSection from './sections/DangerSection.svelte';
import LanguageSection from './sections/LanguageSection.svelte';
import DataPrivacySection from './sections/DataPrivacySection.svelte';
import LocationSection from './sections/LocationSection.svelte';
import MentorSection from './sections/MentorSection.svelte';
import AiSection from './sections/AiSection.svelte';

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
	/** Feature flag key that gates this section's visibility. When the flag
	 *  is disabled, the section is hidden from the settings rail and the
	 *  dispatch page returns 404. Empty = always visible (core section). */
	featureFlag?: string;
}

export const DEFAULT_SECTION = 'profile';

export const SETTINGS_SECTIONS: SectionMeta[] = [
	{ slug: 'profile', labelKey: 'settings.nav.profile', icon: 'user', group: 'account', backendReady: true, component: ProfileSection },
	{ slug: 'account', labelKey: 'settings.nav.account', icon: 'settings', group: 'account', backendReady: true, component: AccountSection },
	{ slug: 'privacy', labelKey: 'settings.nav.privacy', icon: 'lock', group: 'account', backendReady: true, component: PrivacySection },
	{ slug: 'language', labelKey: 'settings.nav.language', icon: 'globe', group: 'account', backendReady: true, component: LanguageSection },
	{ slug: 'location', labelKey: 'settings.nav.location', icon: 'map-pin', group: 'account', backendReady: true, component: LocationSection },
	{ slug: 'mentorship', labelKey: 'settings.nav.mentorship', icon: 'graduation-cap', group: 'mentorship', backendReady: true, component: MentorSection, featureFlag: 'mentorship.enabled' },
	{ slug: 'ai', labelKey: 'settings.nav.ai', icon: 'brain', group: 'ai', backendReady: true, component: AiSection },
	{ slug: 'data-privacy', labelKey: 'settings.nav.data_privacy', icon: 'shield-check', group: 'account', backendReady: true, component: DataPrivacySection },
	{ slug: 'security', labelKey: 'settings.nav.security', icon: 'shield', group: 'security', backendReady: true, component: SecuritySection },
	{ slug: 'sessions', labelKey: 'settings.nav.sessions', icon: 'monitor', group: 'security', backendReady: true, component: SessionsSection },
	{ slug: 'connected', labelKey: 'settings.nav.connected', icon: 'link', group: 'security', backendReady: true, component: ConnectedSection },
	{ slug: 'connected-apps', labelKey: 'settings.nav.connected_apps', icon: 'bot', group: 'security', backendReady: true, component: ConnectedAppsSection },
	{ slug: 'devices', labelKey: 'settings.nav.devices', icon: 'smartphone', group: 'security', backendReady: true, component: DevicesSection, featureFlag: 'feature.notifications' },
	{ slug: 'notifications', labelKey: 'settings.nav.notifications', icon: 'bell', group: 'notifications', backendReady: true, component: NotificationsSection, featureFlag: 'feature.notifications' },
	{ slug: 'newsletter', labelKey: 'settings.nav.newsletter', icon: 'mail', group: 'notifications', backendReady: true, component: NewsletterSection, featureFlag: 'feature.newsletter' },
	{ slug: 'api-keys', labelKey: 'settings.nav.api_keys', icon: 'key', group: 'developer', backendReady: true, component: ApiKeysSection },
	{ slug: 'affiliate', labelKey: 'settings.nav.affiliate', icon: 'share-2', group: 'developer', backendReady: true, component: AffiliateSection, featureFlag: 'feature.affiliate_program' },
	{ slug: 'wallet', labelKey: 'settings.nav.wallet', icon: 'wallet', group: 'wallet', backendReady: true, component: WalletSection, featureFlag: 'feature.points' },
	{ slug: 'tips', labelKey: 'settings.nav.tips', icon: 'gift', group: 'wallet', backendReady: true, component: TipsSection, featureFlag: 'tips.enabled' },
	{ slug: 'subscription', labelKey: 'settings.nav.subscription', icon: 'credit-card', group: 'billing', backendReady: true, component: SubscriptionSection },
	{ slug: 'payments', labelKey: 'settings.nav.payments', icon: 'receipt', group: 'billing', backendReady: true, component: PaymentsSection },
	{ slug: 'danger', labelKey: 'settings.nav.danger', icon: 'alert-triangle', group: 'danger', backendReady: true, component: DangerSection }
];

export function resolveSection(slug: string): SectionMeta | null {
	return SETTINGS_SECTIONS.find((s) => s.slug === slug) ?? null;
}

const GROUP_ORDER: HubGroup[] = ['account', 'mentorship', 'security', 'notifications', 'billing', 'wallet', 'ai', 'developer', 'danger'];

/** Returns true when the section's feature flag is enabled (or it has no flag). */
function sectionFeatureEnabled(s: SectionMeta, features?: Record<string, boolean>): boolean {
	if (!s.featureFlag) return true;
	return features?.[s.featureFlag] === true;
}

export function sectionsByGroup(features?: Record<string, boolean>): [HubGroup, SectionMeta[]][] {
	return GROUP_ORDER.map((g) => [g, SETTINGS_SECTIONS.filter((s) => s.group === g && sectionFeatureEnabled(s, features))] as [HubGroup, SectionMeta[]])
		.filter(([, list]) => list.length > 0);
}

/** Returns the group that owns the given section slug, or null if not found. */
export function groupForSection(slug: string): HubGroup | null {
	return resolveSection(slug)?.group ?? null;
}

/** Returns all ready sections within the given group, in registry order.
 *  When `features` is provided, sections whose feature flag is disabled
 *  are excluded so they don't appear in the settings tab rail. */
export function sectionsInGroup(group: HubGroup, features?: Record<string, boolean>): SectionMeta[] {
	return SETTINGS_SECTIONS.filter(
		(s) => s.group === group && s.backendReady && s.component && sectionFeatureEnabled(s, features)
	);
}

/** Lean list of groups that have at least one ready section, with the first slug for navigation.
 *  When `features` is provided, sections whose feature flag is disabled
 *  are excluded so empty groups collapse. */
export function readyGroups(features?: Record<string, boolean>): { group: HubGroup; firstSlug: string; count: number }[] {
	return sectionsByGroup(features)
		.map(([group, items]) => {
			const ready = items.filter((s) => s.backendReady && s.component);
			return { group, firstSlug: ready[0]?.slug ?? '', count: ready.length };
		})
		.filter((g) => g.count > 0);
}
