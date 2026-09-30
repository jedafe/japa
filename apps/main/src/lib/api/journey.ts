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

// The guest journey token is delivered via the jc_journey_token Set-Cookie
// header and a top-level envelope sidecar — it is not part of `data`.
export const setFeedContext = (input: Partial<JourneyContextInput>) =>
	api.post<VisitorContextSummary>('/feed/context', input);
