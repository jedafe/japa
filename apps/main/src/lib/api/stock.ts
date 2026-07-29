import { api } from '$lib/api/client';
import type {
	StockPhoto,
	StockSearchResult,
	StockImportResult,
	StockProvidersResponse
} from '$lib/types';

/**
 * Stock photo API client wrappers. Each returns the standard ClientResult
 * envelope ({ data, error, status }) so callers can handle failures uniformly.
 *
 * Ref: docs/superpowers/plans/2026-07-29-stock-photo-integration.md
 */

export const listStockProviders = () => api.get<StockProvidersResponse>('/stock/providers');

export const searchStock = (params: {
	q: string;
	provider?: string;
	page?: number;
	per_page?: number;
}) => {
	const searchParams = new URLSearchParams();
	searchParams.set('q', params.q);
	if (params.provider) searchParams.set('provider', params.provider);
	if (params.page) searchParams.set('page', String(params.page));
	if (params.per_page) searchParams.set('per_page', String(params.per_page));
	return api.get<StockSearchResult>(`/stock/search?${searchParams.toString()}`);
};

export const getStockPhoto = (provider: string, id: string) =>
	api.get<StockPhoto>(`/stock/photo/${provider}/${id}`);

export const importStockPhoto = (provider: string, id: string) =>
	api.post<StockImportResult>('/stock/import', { provider, id });
