import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null)
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

const defaultData = { onboarding: { current_step: 1, step_data: {} } };

describe('Onboarding +page.svelte', () => {
	it('shows the Skip for now button', () => {
		render(Page, { props: { data: defaultData } });
		expect(screen.getByText('Skip for now')).toBeTruthy();
	});

	it('renders the OnboardingProgress component', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('Identity')).toBeTruthy();
		});
	});

	it('renders the step labels', async () => {
		render(Page, { props: { data: defaultData } });
		await waitFor(() => {
			expect(screen.getByText('Destination')).toBeTruthy();
			expect(screen.getByText('Interests')).toBeTruthy();
		});
	});
});
