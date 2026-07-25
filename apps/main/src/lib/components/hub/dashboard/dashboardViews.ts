import type { Component } from 'svelte';
import type { HubGroup } from '$lib/types';
import OverviewView from './views/OverviewView.svelte';
import ContentManagerView from './views/ContentManagerView.svelte';
import RelationshipsView from './views/RelationshipsView.svelte';
import LearningView from './views/LearningView.svelte';
import CertificatesView from './views/CertificatesView.svelte';
import KnowledgeBaseView from './views/KnowledgeBaseView.svelte';

/**
 * Client meta for one dashboard view. Mirrors the settings SectionMeta —
 * `component` is null for views listed in the rail but not yet built, and
 * `backendReady:false` gates both the rail (disabled) and the route (404).
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Dashboard hub)
 */
export interface ViewMeta {
	slug: string;
	labelKey: string;
	icon: string; // lucide name
	group: HubGroup;
	backendReady: boolean;
	component: Component<{ data: unknown }> | null;
}

export const DEFAULT_VIEW = 'overview';

export const DASHBOARD_VIEWS: ViewMeta[] = [
	{ slug: 'overview', labelKey: 'dashboard.nav.overview', icon: 'layout-dashboard', group: 'activity', backendReady: true, component: OverviewView },
	{ slug: 'analytics', labelKey: 'dashboard.nav.analytics', icon: 'bar-chart-3', group: 'activity', backendReady: false, component: null },
	{ slug: 'content', labelKey: 'dashboard.nav.content', icon: 'file-text', group: 'activity', backendReady: true, component: ContentManagerView },
	{ slug: 'relationships', labelKey: 'dashboard.nav.relationships', icon: 'users', group: 'activity', backendReady: true, component: RelationshipsView },
	{ slug: 'learning', labelKey: 'dashboard.nav.learning', icon: 'graduation-cap', group: 'activity', backendReady: true, component: LearningView },
	{ slug: 'certificates', labelKey: 'dashboard.nav.certificates', icon: 'award', group: 'activity', backendReady: true, component: CertificatesView },
	{ slug: 'knowledge-base', labelKey: 'dashboard.nav.knowledge_base', icon: 'book-open', group: 'activity', backendReady: true, component: KnowledgeBaseView }
];

export function resolveView(slug: string): ViewMeta | null {
	return DASHBOARD_VIEWS.find((v) => v.slug === slug) ?? null;
}

const GROUP_ORDER: HubGroup[] = ['activity'];

export function viewsByGroup(): [HubGroup, ViewMeta[]][] {
	return GROUP_ORDER.map((g) => [g, DASHBOARD_VIEWS.filter((v) => v.group === g)] as [HubGroup, ViewMeta[]])
		.filter(([, list]) => list.length > 0);
}
