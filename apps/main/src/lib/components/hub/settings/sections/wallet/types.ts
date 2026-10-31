/**
 * Shared wallet types for the settings wallet tab suite.
 * Ref: japaconnect-engineering/sections/28-wallet-credits.md
 */

export interface Currency {
	code: string;
	name?: string;
	symbol?: string;
	credits_per_unit?: number;
	position?: number;
}

export interface CreditPack {
	id: string;
	name: string;
	description?: string | null;
	credit_amount: number;
	bonus_credits?: number;
	min_user_level?: string | null;
	position?: number;
}

export interface LedgerEntry {
	id: string;
	type: string;
	operation?: string;
	amount?: number;
	description?: string | null;
	created_at?: string;
}

export interface Withdrawal {
	id: string;
	credit_amount: number;
	currency_code: string;
	credits_per_unit: number;
	fiat_amount: number;
	fee_amount: number;
	fee_basis: string;
	fee_rate?: number;
	net_amount: number;
	status: string;
	payout_provider?: string | null;
	payout_method_id?: string | null;
	provider_ref?: string | null;
	created_at?: string;
}

export interface WithdrawalMethod {
	id: string;
	provider: string;
	label?: string | null;
	account: string;
	currency_code?: string | null;
	is_verified: boolean;
	is_default: boolean;
	is_active: boolean;
	metadata?: unknown;
}

export interface WithdrawalResult {
	id: string;
	credit_amount: number;
	currency_code: string;
	credits_per_unit: number;
	fiat_amount: number;
	fee_amount: number;
	net_amount: number;
	status: string;
	payout_provider?: string | null;
}

export interface WalletData {
	wallet: { points_balance?: number; credits_balance?: number; lifetime_points?: number } | null;
	ledger: LedgerEntry[];
	/** Entries fetched per ledger stream (page_size=50 each). The History
	 *  tab uses these to page further client-side — a count equal to the
	 *  page size means more may exist (the endpoints return no total). */
	pointsLoaded?: number;
	creditsLoaded?: number;
	settings?: {
		settings?: { enable_earning?: boolean; daily_earning_cap?: number };
		min_purchase_credits?: number;
		min_withdrawal_credits?: number;
	} | null;
	/** Server-configured minimums; null when /me/wallet/settings is
	 *  unavailable — tabs must treat null as "unknown" (no client-side
	 *  minimum check), not substitute an invented default (Rule 3). */
	minPurchaseCredits: number | null;
	minWithdrawCredits: number | null;
	currencies: Currency[];
	creditPacks: CreditPack[];
	preferredCurrency?: { preferred_currency?: string } | null;
	withdrawals: Withdrawal[];
	withdrawalMethods: WithdrawalMethod[];
	withdrawalProviders: string[];
	withdrawEnabled: boolean;
	/** feature.wallet — commerce endpoints (purchase/withdraw). Off means
	 *  the section shows only the points surfaces (Details/History). */
	walletEnabled: boolean;
}
