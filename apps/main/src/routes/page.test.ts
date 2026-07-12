import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import type { HealthStatus } from '$lib/types';
import Page from './+page.svelte';

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'nav.home': 'Home',
		'landing.tagline': 'The community hub for the Nigerian diaspora and Nigerians planning to relocate abroad.',
		'landing.health': 'System health',
		'landing.operational': 'Services operational',
		'landing.degraded': 'Service degraded',
		'landing.get_started': 'Get started',
		'landing.explore_feed': 'Explore feed',
		'landing.feature_community': 'Community',
		'landing.feature_community_desc': 'desc',
		'landing.feature_spaces': 'Spaces',
		'landing.feature_spaces_desc': 'desc',
		'landing.feature_learn': 'Learn',
		'landing.feature_learn_desc': 'desc',
		'landing.feature_trusted': 'Trusted',
		'landing.feature_trusted_desc': 'desc'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/seo', () => ({
	MetadataHead: () => null,
	JsonLd: () => null,
	createWebsiteSchema: () => ({})
}));

const healthyData: HealthStatus & { translations: Record<string, string> } = {
	healthy: true,
	flags: {},
	translations: {}
};
const degradedData: HealthStatus & { translations: Record<string, string> } = {
	healthy: false,
	flags: {},
	translations: {}
};

describe('JapaConnect landing page', () => {
	it('renders the JapaConnect heading', () => {
		render(Page, { props: { data: healthyData } });
		expect(screen.getByRole('heading', { name: 'JapaConnect' })).toBeTruthy();
	});

	it('reports an operational status when healthy', () => {
		render(Page, { props: { data: healthyData } });
		expect(screen.getByTestId('health-status')?.textContent ?? '').toContain('operational');
	});

	it('reports a degraded status when unhealthy', () => {
		render(Page, { props: { data: degradedData } });
		expect(screen.getByTestId('health-status')?.textContent ?? '').toContain('degraded');
	});
});