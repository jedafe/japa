import { api } from '$lib/api/client';
import type { VisitorContextSummary } from '$lib/types';

export interface JourneyContextInput {
	origin_country: string;
	destination_country: string;
	japa_stage: string;
	visa_route: string;
	purpose: string;
	travel_date: string;
}

export const getFeedContext = () => api.get<VisitorContextSummary>('/feed/context');

export const setFeedContext = (input: Partial<JourneyContextInput>) =>
	api.post<VisitorContextSummary & { token: string }>('/feed/context', input);
