import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import type { HealthStatus } from '$lib/types';
import Page from './+page.svelte';

const healthyData: HealthStatus = { healthy: true, flags: {} };
const degradedData: HealthStatus = { healthy: false, flags: {} };

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