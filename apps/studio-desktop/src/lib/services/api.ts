/**
 * JapaConnect API service layer for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { createClient } from '$lib/api/client';
import type {
	ContentTypeResponse,
	Persona,
	PersonaVoice,
	Space,
	ContentRecord
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
		},
		createContent(body: unknown) {
			return client.post<ContentRecord>('/content', body);
		},
		updateContent(id: string, body: unknown) {
			return client.patch<ContentRecord>(`/content/${id}`, body);
		},
		publishContent(id: string) {
			return client.post<ContentRecord>(`/content/${id}/publish`, {});
		},
		reassignAuthor(id: string, body: { new_author_id: string; reason?: string }) {
			return client.post<void>(`/content/${id}/reassign-author`, body);
		},
		addContentTags(id: string, body: { tag_slugs: string[] }) {
			return client.post<{ added: number }>(`/content/${id}/tags`, body);
		}
	};
}
