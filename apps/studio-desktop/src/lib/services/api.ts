/**
 * JapaConnect API service layer for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { createClient } from '$lib/api/client';
import type {
	ContentTypeResponse,
	PublicContentType,
	Persona,
	PersonaVoice,
	Space,
	ContentRecord
} from '$lib/types/prompt';

export function createApiClient(baseUrl: string) {
	const client = createClient(baseUrl);

	return {
		getMe() {
			return client.get<{ id: string; email: string }>('/me');
		},
		getStudioContentTypes() {
			return client.get<ContentTypeResponse>('/admin/studio/content-types');
		},
		getContentTypes() {
			// Public / member-safe route (Section 14).
			return client.get<PublicContentType[]>('/content-types');
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
		getMySpaces() {
			return client.get<Space[]>('/me/spaces');
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
		getContentTags(id: string) {
			return client.get<{ id: string; slug: string; name: string }[]>(`/content/${id}/tags`);
		},
		addContentTags(id: string, body: { tag_slugs: string[] }) {
			return client.post<{ added: number }>(`/content/${id}/tags`, body);
		},
		deleteContentTag(id: string, tagIdOrSlug: string) {
			return client.delete<unknown>(`/content/${id}/tags/${encodeURIComponent(tagIdOrSlug)}`);
		}
	};
}
