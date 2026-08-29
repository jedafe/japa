import { api } from '$lib/api/client';
import type { ClientResult } from '$lib/api/client';
import type { DraftRequest, DraftResponse } from './types';

/**
 * Ask the AI body assistant to draft or rewrite HTML body content.
 * Wraps POST /api/v1/ai/assist/draft using the shared API client.
 */
export async function draftContent(request: DraftRequest): Promise<ClientResult<DraftResponse>> {
	return api.post<DraftResponse>('/ai/assist/draft', request);
}
