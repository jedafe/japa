import type { Component } from 'svelte';
import OverviewView from './views/OverviewView.svelte';
import AnalyticsView from './views/AnalyticsView.svelte';
import RelationshipsView from './views/RelationshipsView.svelte';
import AcademyView from './views/AcademyView.svelte';
import CertificatesView from './views/CertificatesView.svelte';
import KnowledgeBaseView from './views/KnowledgeBaseView.svelte';

/**
 * Static component registry for dashboard [view] slugs. This mapping cannot
 * be API-driven because Svelte components cannot be resolved from a JSON
 * string; the *list* of visible views (order, labels, visibility) is served
 * by `GET /menus/dashboard_rail` (ADR-054).
 *
 * Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
 */
export const DASHBOARD_VIEW_COMPONENTS: Record<string, Component<{ data: unknown }>> = {
	overview: OverviewView,
	analytics: AnalyticsView,
	relationships: RelationshipsView,
	academy: AcademyView,
	certificates: CertificatesView,
	'knowledge-base': KnowledgeBaseView
};
