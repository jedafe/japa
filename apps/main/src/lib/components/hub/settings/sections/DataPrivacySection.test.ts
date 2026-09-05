import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';

vi.mock('$lib/i18n', () => ({
	t: vi.fn((key: string, fallback?: string) => fallback ?? key)
}));

vi.mock('$env/static/public', () => ({
	PUBLIC_JC_API_URL: 'http://localhost:8080'
}));

vi.mock('$lib/api/client', () => ({
	api: {
		get: vi.fn(),
		post: vi.fn(),
		delete: vi.fn()
	},
	apiBaseUrl: 'http://localhost:8080'
}));

vi.mock('$lib/api/url', () => ({
	buildApiUrl: (path: string, base: string) => `${base.replace(/\/$/, '')}${path}`
}));

vi.mock('$lib/api/hub', () => ({
	exportData: vi.fn(),
	cancelDataExport: vi.fn()
}));

import DataPrivacySection from './DataPrivacySection.svelte';

describe('DataPrivacySection', () => {
	it('renders from SSR payload without an effect update depth error', () => {
		const data = {
			consents: [{ consent_type: 'marketing', granted: true, policy_version: '1.0' }],
			consentConfig: { consent_types: ['marketing'], policy_version: '1.0' },
			exportStatus: { status: 'no_pending_export' }
		};

		const { container } = render(DataPrivacySection, { props: { data } });
		expect(container.textContent).toContain('Consent preferences');
		expect(container.querySelectorAll('li').length).toBe(1);
	});
});
