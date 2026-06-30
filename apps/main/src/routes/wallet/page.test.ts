import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import Page from './+page.svelte';

vi.mock('$lib/api', () => ({
	getToken: vi.fn().mockReturnValue(null),
	api: {
		getWalletBalance: vi.fn().mockResolvedValue({ points: 0, credits: 0 }),
		getWalletLedger: vi.fn().mockResolvedValue([]),
		convertWallet: vi.fn().mockResolvedValue({}),
		initiatePayment: vi.fn().mockResolvedValue({}),
		getPaymentStatus: vi.fn().mockResolvedValue({})
	}
}));

vi.mock('$lib/stores/i18n.svelte', () => {
	const translations: Record<string, string> = {
		'wallet.title': 'Wallet',
		'wallet.points': 'Points',
		'wallet.credits': 'Credits',
		'wallet.add_credits': 'Add credits',
		'wallet.converting': 'Converting...',
		'wallet.convert_points': 'Convert {count} points',
		'wallet.convert_ratio': '1000 points = 1 credit',
		'wallet.recent_activity': 'Recent activity',
		'wallet.no_transactions': 'No transactions yet',
		'wallet.no_transactions_msg': 'Your transaction history will appear here.',
		'wallet.not_available': 'Wallet is not available',
		'wallet.choose_amount': 'Choose an amount',
		'wallet.custom_amount': 'Custom amount',
		'wallet.enter_amount': 'Enter amount',
		'wallet.amount': 'Amount',
		'wallet.processing': 'Processing...',
		'wallet.pay': 'Pay',
		'forum.feature_disabled': 'This feature is currently disabled.',
		'forum.go_home': 'Go home',
		'common.cancel': 'Cancel'
	};
	return { t: (key: string) => translations[key] ?? key };
});

vi.mock('$lib/stores/features.svelte', () => ({
	isFeatureEnabled: () => true,
	setFeatureFlags: () => {}
}));

vi.mock('$app/navigation', () => ({
	goto: vi.fn()
}));

vi.mock('$lib/components/ui', async () => {
	const { default: Comp } = await import('$lib/components/EmptyStateMock.svelte');
	return { EmptyState: Comp };
});

describe('Wallet +page.svelte', () => {
	it('renders the Wallet heading', () => {
		render(Page);
		expect(screen.getByRole('heading', { name: 'Wallet' })).toBeTruthy();
	});

	it('shows the Points label', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Points')).toBeTruthy();
		});
	});

	it('shows the Credits label', async () => {
		render(Page);
		await waitFor(() => {
			expect(screen.getByText('Credits')).toBeTruthy();
		});
	});
});
