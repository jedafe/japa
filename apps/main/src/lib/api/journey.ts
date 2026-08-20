import { api } from '$lib/api/client';
import type { VisitorContextSummary } from '$lib/types';

export interface JourneyOption {
	value: string;
	label: string;
}

export interface JourneyOptions {
	purposes: JourneyOption[];
	stages: JourneyOption[];
	visa_routes: JourneyOption[];
}

export interface JourneyContextInput {
	origin_country: string;
	destination_country: string;
	japa_stage: string;
	visa_route: string;
	purpose: string;
	travel_date: string;
}

export const getFeedContext = () => api.get<VisitorContextSummary>('/feed/context');

export const getJourneyOptions = () => api.get<JourneyOptions>('/feed/journey-options');

export const setFeedContext = (input: Partial<JourneyContextInput>) =>
	api.post<VisitorContextSummary & { token: string }>('/feed/context', input);
