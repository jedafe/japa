/**
 * JapaConnect API service layer for the desktop app.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

import { createClient } from '$lib/api/client';
import type {
	PublicContentType,
	StudioContentType,
	Persona,
	PersonaVoice,
	Space,
	ContentRecord,
	ContentDetail,
	SelectOption
} from '$lib/types/prompt';

// ADR-130 Phase 3: unified Japa AI chat — the JSON contract returned by
// POST /ai/chat for non-SSE clients. sources are §69 RAG citations.
export interface AiChatResult {
	reply: string;
	conversation_id: string;
	message_id?: string;
	sources?: { title?: string; url?: string; chunk_text?: string }[];
}

export interface AiChatRequest {
	message: string;
	conversation_id?: string;
	model?: string;
	voice_id?: string;
	tools?: { enabled?: boolean };
}

export function mapPublicContentType(t: PublicContentType): StudioContentType {
	return {
		type: t.value,
		label: t.label,
		guide_categories: t.has_category
			? (t.categories ?? [])
					.filter((c) => !!c.Slug)
					.map((c) => ({ value: c.Slug.toLowerCase(), label: c.Label || c.Slug }))
			: undefined
	};
}

type Client = ReturnType<typeof createClient>;

async function loadAllPages<T extends { id?: string }>(
	client: Client,
	basePath: string,
	pageSize = 100
): Promise<{ ok: boolean; data?: T[]; error?: { code: string; message: string } }> {
	const all: T[] = [];
	let offset = 0;
	let total = 0;
	do {
		const res = await client.get<T[]>(`${basePath}?page_size=${pageSize}&page_offset=${offset}`);
		if (!res.ok) return res;
		if (res.data) all.push(...res.data);
		total = Number(res.meta?.total ?? 0);
		offset += res.data?.length ?? 0;
	} while (offset < total);
	return { ok: true, data: all };
}

export function createApiClient(baseUrl: string) {
	const client = createClient(baseUrl);

	return {
		getMe() {
			return client.get<{ id: string; email: string }>('/me');
		},
		getMyContent(id: string) {
			return client.get<ContentDetail>(`/me/content/${encodeURIComponent(id)}`);
		},
		getContentTypes() {
			// Public / member-safe route (Section 14).
			return client.get<PublicContentType[]>('/content-types');
		},
		async getPersonas() {
			return loadAllPages<Persona>(client, '/admin/personas');
		},
		async getPersonaVoices() {
			return loadAllPages<PersonaVoice>(client, '/admin/persona-voices');
		},
		async getSpaces() {
			return loadAllPages<Space>(client, '/spaces');
		},
		getMySpaces() {
			return client.get<Space[]>('/me/spaces');
		},
		getStatusOptions() {
			return client.get<SelectOption[]>('/content/status-options');
		},
		getVisibilityOptions() {
			return client.get<SelectOption[]>('/content/visibility-options');
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
		},
		chatAI(body: AiChatRequest) {
			// ADR-130: unified chat surface — unary JSON reply (no SSE).
			return client.post<AiChatResult>('/ai/chat', body);
		}
	};
}
