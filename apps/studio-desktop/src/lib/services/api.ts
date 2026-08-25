/**
 * JapaConnect API service layer for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { createClient } from '$lib/api/client';
import type {
	ContentTypeResponse,
	Persona,
	PersonaVoice,
	Space
} from '$lib/types/prompt';

export function createApiClient(baseUrl: string, apiKey: string) {
	const client = createClient(baseUrl, apiKey);

	return {
		getMe() {
			return client.get<{ id: string; email: string }>('/me');
		},
		getStudioContentTypes() {
			return client.get<ContentTypeResponse>('/admin/studio/content-types');
		},
		getPersonas() {
			return client.get<Persona[]>('/admin/personas');
		},
		getPersonaVoices() {
			return client.get<PersonaVoice[]>('/admin/persona-voices');
		},
		getSpaces() {
			return client.get<Space[]>('/spaces');
		}
	};
}
