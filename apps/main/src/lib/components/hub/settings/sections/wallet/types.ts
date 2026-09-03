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
	amount?: number;
	description?: string | null;
	created_at?: string;
}

export interface WalletData {
	wallet: { points_balance?: number; credits_balance?: number; lifetime_points?: number } | null;
	ledger: LedgerEntry[];
	settings?: {
		settings?: { points_to_credits_rate?: number };
		min_convert_points?: number;
		min_purchase_credits?: number;
	} | null;
	minConvertPoints: number;
	minPurchaseCredits: number;
	currencies: Currency[];
	creditPacks: CreditPack[];
	preferredCurrency?: { preferred_currency?: string } | null;
}
