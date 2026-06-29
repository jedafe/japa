const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080/api/v1';

interface ApiOk<T> {
	data: T;
}

interface ApiErr {
	error: { code: string; message: string };
}

export type ApiResult<T> = { ok: true; data: T } | { ok: false; error: string };

let jwtToken: string | null = null;

export function setToken(token: string | null) {
	jwtToken = token;
}

export function getToken(): string | null {
	return jwtToken;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
		...Object.fromEntries(Object.entries(init?.headers ?? {}))
	};
	if (jwtToken) {
		headers['Authorization'] = `Bearer ${jwtToken}`;
	}

	const res = await fetch(`${API_BASE}${path}`, {
		...init,
		headers
	});

	const body = (await res.json()) as ApiOk<T> | ApiErr;
	if (!res.ok || 'error' in body) {
		const err = 'error' in body ? body.error : { code: 'unknown', message: res.statusText };
		throw new Error(`${err.code}: ${err.message}`);
	}
	return (body as ApiOk<T>).data;
}

export const api = {
	// Content
	createContent(data: {
		content_type: string;
		title: string;
		content: string;
		status: string;
		field_values?: Record<string, unknown>;
	}) {
		return request<{ id: string; slug: string }>('/content', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updateContent(id: string, data: Record<string, unknown>) {
		return request<{ id: string }>('/content/' + id, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},

	// Onboarding
	getOnboardingStatus() {
		return request<{
			current_step: number;
			completed: boolean;
			step_data: Record<string, unknown>;
			tour_dismissed: boolean;
		}>('/onboarding/status');
	},
	updateOnboardingStep(data: { step: number; step_data?: Record<string, unknown> }) {
		return request<{ current_step: number }>('/onboarding/step', {
			method: 'PUT',
			body: JSON.stringify(data)
		});
	},
	completeOnboarding() {
		return request<{ completed: boolean }>('/onboarding/complete', { method: 'POST' });
	},
	dismissTour() {
		return request<{ tour_dismissed: boolean }>('/onboarding/tour/dismiss', { method: 'POST' });
	},

	// Follows
	toggleFollow(data: { entity_type: string; entity_id: string }) {
		return request<{ following: boolean }>('/follows', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},

	// Notifications
	getNotifications(params?: { limit?: number; offset?: number }) {
		const query = params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : '';
		return request<
			Array<{ id: string; title: string; body: string; read: boolean; created_at: string }>
		>('/notifications' + query);
	},
	getUnreadCount() {
		return request<{ count: number }>('/notifications/unread-count');
	},
	markNotificationRead(id: string) {
		return request<{ id: string }>('/notifications/' + id + '/read', { method: 'PATCH' });
	},
	markAllRead() {
		return request<{ marked: number }>('/notifications/read-all', { method: 'POST' });
	},

	// Universities / Spaces
	listSpaces(params?: { space_type_id?: string; limit?: number }) {
		const query = params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : '';
		return request<Array<{ id: string; name: string; slug: string; description: string }>>(
			'/spaces' + query
		);
	},
	getSpace(slug: string) {
		return request<{
			id: string;
			name: string;
			slug: string;
			description: string;
			rules: unknown[];
		}>('/spaces/' + slug);
	},

	// Forum
	getForumThreads(params?: {
		space_id?: string;
		tag?: string;
		solved?: boolean;
		limit?: number;
		offset?: number;
	}) {
		const p = new URLSearchParams();
		if (params?.space_id) p.set('space_id', params.space_id);
		if (params?.tag) p.set('tag', params.tag);
		if (params?.solved !== undefined) p.set('solved', String(params.solved));
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		const q = p.toString();
		return request<{
			threads: ForumThread[];
			total: number;
			page: number;
			total_pages: number;
		}>('/forum/threads' + (q ? '?' + q : ''));
	},
	getForumThread(slug: string) {
		return request<ForumThread>('/forum/threads/' + slug);
	},
	createForumThread(data: {
		title: string;
		body: string;
		space_id?: string;
		tags?: string[];
		anonymous?: boolean;
	}) {
		return request<ForumThread>('/forum/threads', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updateForumThread(id: string, data: { title?: string; body?: string }) {
		return request<ForumThread>('/forum/threads/' + id, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteForumThread(id: string) {
		return request<{ deleted: boolean }>('/forum/threads/' + id, { method: 'DELETE' });
	},
	solveForumThread(id: string) {
		return request<{ solved: boolean }>('/forum/threads/' + id + '/solve', { method: 'POST' });
	},
	getForumReplies(threadId: string, params?: { limit?: number; offset?: number }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		const q = p.toString();
		return request<{
			replies: ForumReply[];
			total: number;
		}>('/forum/threads/' + threadId + '/replies' + (q ? '?' + q : ''));
	},
	createForumReply(threadId: string, data: { body: string; anonymous?: boolean }) {
		return request<ForumReply>('/forum/threads/' + threadId + '/replies', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updateForumReply(id: string, data: { body: string }) {
		return request<ForumReply>('/forum/replies/' + id, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteForumReply(id: string) {
		return request<{ deleted: boolean }>('/forum/replies/' + id, { method: 'DELETE' });
	},
	voteForum(data: { entity_type: 'thread' | 'reply'; entity_id: string; value: 1 | -1 }) {
		return request<{ score: number }>('/forum/vote', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	createForumPoll(data: { thread_id: string; question: string; options: string[] }) {
		return request<ForumPoll>('/forum/polls', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	voteForumPoll(id: string, data: { option_index: number }) {
		return request<ForumPoll>('/forum/polls/' + id + '/vote', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	getForumPoll(id: string) {
		return request<ForumPoll>('/forum/polls/' + id);
	},
	createForumTag(data: { name: string; slug: string }) {
		return request<{ id: string; name: string; slug: string }>('/forum/tags', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	getForumTags() {
		return request<Array<{ id: string; name: string; slug: string; thread_count: number }>>(
			'/forum/tags'
		);
	},
	addForumBookmark(data: { thread_id: string }) {
		return request<{ id: string }>('/forum/bookmarks', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	removeForumBookmark(id: string) {
		return request<{ deleted: boolean }>('/forum/bookmarks/' + id, { method: 'DELETE' });
	},
	getForumBookmarks() {
		return request<Array<{ id: string; thread_id: string; created_at: string }>>('/forum/bookmarks');
	},

	// Tickets
	createTicket(data: { subject: string; description: string; type_id?: string; priority?: string; email?: string; field_values?: Record<string, unknown> }) {
		return request<Ticket>('/support', { method: 'POST', body: JSON.stringify(data) });
	},
	listTickets(params?: { status?: string; page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.status) p.set('status', params.status);
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<Ticket[]>('/support/my' + (q ? '?' + q : ''));
	},
	getTicket(id: string) {
		return request<{ ticket: Ticket; comments: TicketComment[]; attachments: unknown[] }>('/support/my/' + id);
	},
	addTicketReply(id: string, data: { body: string }) {
		return request<TicketComment>('/support/my/' + id + '/reply', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},

	// Direct Messages
	listDMConversations(params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<DMConversation[]>('/dm/conversations' + (q ? '?' + q : ''));
	},
	getDMMessages(conversationId: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<DMMessage[]>('/dm/conversations/' + conversationId + '/messages' + (q ? '?' + q : ''));
	},
	sendDMMessage(conversationId: string, data: { body: string }) {
		return request<{ success: boolean }>('/dm/conversations/' + conversationId + '/messages', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	markDMRead(conversationId: string) {
		return request<{ success: boolean }>('/dm/conversations/' + conversationId + '/read', {
			method: 'POST'
		});
	},
	acceptDMRequest(conversationId: string) {
		return request<{ success: boolean }>('/dm/message-requests/' + conversationId + '/accept', {
			method: 'POST'
		});
	},
	declineDMRequest(conversationId: string) {
		return request<{ success: boolean }>('/dm/message-requests/' + conversationId + '/decline', {
			method: 'POST'
		});
	},
	updateDMPrivacy(privacy: string) {
		return request<{ success: boolean }>('/dm/privacy', {
			method: 'PUT',
			body: JSON.stringify({ privacy })
		});
	},

	// Professional Profiles
	listProfessionalProfiles(params?: { limit?: number; offset?: number; search?: string }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		if (params?.search) p.set('search', params.search);
		const q = p.toString();
		return request<{
			profiles: ProfessionalProfile[];
			total: number;
			page: number;
			total_pages: number;
		}>('/professional-profiles' + (q ? '?' + q : ''));
	},
	getProfessionalProfile(slug: string) {
		return request<ProfessionalProfileDetail>('/professional-profiles/' + slug);
	},
	createProfessionalProfile(data: {
		name: string;
		headline: string;
		skills: string[];
		is_public?: boolean;
	}) {
		return request<ProfessionalProfileDetail>('/professional-profiles', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updateProfessionalProfile(id: string, data: {
		name?: string;
		headline?: string;
		skills?: string[];
		is_public?: boolean;
	}) {
		return request<ProfessionalProfileDetail>('/professional-profiles/' + id, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteProfessionalProfile(id: string) {
		return request<{ deleted: boolean }>('/professional-profiles/' + id, { method: 'DELETE' });
	},
	listRelocations(profileId: string) {
		return request<Relocation[]>('/professional-profiles/' + profileId + '/relocations');
	},
	addRelocation(profileId: string, data: {
		country: string;
		city?: string;
		status: string;
		year?: number;
	}) {
		return request<Relocation>('/professional-profiles/' + profileId + '/relocations', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updateRelocation(relocationId: string, data: {
		country?: string;
		city?: string;
		status?: string;
		year?: number;
	}) {
		return request<Relocation>('/professional-profiles/relocations/' + relocationId, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteRelocation(relocationId: string) {
		return request<{ deleted: boolean }>('/professional-profiles/relocations/' + relocationId, {
			method: 'DELETE'
		});
	},
	listDocuments(profileId: string) {
		return request<Document[]>('/professional-profiles/' + profileId + '/documents');
	},
	addDocument(profileId: string, data: {
		document_type: string;
		title: string;
		description?: string;
		file_url?: string;
	}) {
		return request<Document>('/professional-profiles/' + profileId + '/documents', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updateDocument(documentId: string, data: {
		document_type?: string;
		title?: string;
		description?: string;
		file_url?: string;
	}) {
		return request<Document>('/professional-profiles/documents/' + documentId, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteDocument(documentId: string) {
		return request<{ deleted: boolean }>('/professional-profiles/documents/' + documentId, {
			method: 'DELETE'
		});
	},
	listVerifiers(profileId: string) {
		return request<Verifier[]>('/professional-profiles/' + profileId + '/verifiers');
	},
	assignVerifier(profileId: string, data: { user_id: string; role?: string }) {
		return request<Verifier>('/professional-profiles/' + profileId + '/verifiers', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},

	// Developer API Keys
	listDeveloperKeys() {
		return request<DeveloperApiKey[]>('/developer/keys');
	},
	createDeveloperKey(data: { name: string; scopes: string[] }) {
		return request<{ key: string; key_info: DeveloperApiKey }>('/developer/keys', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	revokeDeveloperKey(id: string) {
		return request<{ deleted: boolean }>('/developer/keys/' + id, { method: 'DELETE' });
	},
	updateDeveloperKey(id: string, data: { name?: string; scopes?: string[] }) {
		return request<DeveloperApiKey>('/developer/keys/' + id, {
			method: 'PUT',
			body: JSON.stringify(data)
		});
	}
};

// Forum types
export interface ForumThread {
	id: string;
	title: string;
	body: string;
	slug: string;
	author_id: string;
	author_name: string;
	author_avatar?: string;
	space_id?: string;
	space_name?: string;
	score: number;
	reply_count: number;
	solved: boolean;
	anonymous: boolean;
	tags: string[];
	poll_id?: string;
	created_at: string;
	updated_at: string;
}

export interface ForumReply {
	id: string;
	thread_id: string;
	body: string;
	author_id: string;
	author_name: string;
	author_avatar?: string;
	score: number;
	anonymous: boolean;
	created_at: string;
	updated_at: string;
}

export interface ForumPoll {
	id: string;
	thread_id: string;
	question: string;
	options: { text: string; votes: number }[];
	total_votes: number;
	user_vote?: number;
	created_at: string;
}

// Ticket types
export interface Ticket {
	id: string;
	subject: string;
	description: string;
	status: string;
	category: string;
	author_id: string;
	author_name: string;
	created_at: string;
	updated_at: string;
}

export interface TicketComment {
	id: string;
	ticket_id: string;
	body: string;
	author_id: string;
	author_name: string;
	created_at: string;
}

// DM types
export interface DMConversation {
	id: string;
	other_user_id: string;
	other_username: string;
	other_display_name: string;
	other_avatar_url: string;
	last_message_at?: string;
	last_message_preview?: string;
	unread_count: number;
	created_at: string;
}

export interface DMMessage {
	id: string;
	sender_id: string;
	sender_username: string;
	sender_display_name: string;
	body: string;
	created_at: string;
	read_at?: string;
}

// Professional Profile types
export interface ProfessionalProfile {
	id: string;
	slug: string;
	name: string;
	headline: string;
	skills: string[];
	is_public: boolean;
	author_user_id: string;
	original_author_id?: string;
	created_at: string;
	updated_at: string;
}

export interface ProfessionalProfileDetail extends ProfessionalProfile {
	relocations: Relocation[];
	documents: Document[];
	verifiers: Verifier[];
}

export interface Relocation {
	id: string;
	profile_id: string;
	country: string;
	city?: string;
	status: string;
	year?: number;
	created_at: string;
}

export interface Document {
	id: string;
	profile_id: string;
	document_type: string;
	title: string;
	description?: string;
	file_url?: string;
	verified: boolean;
	created_at: string;
}

export interface Verifier {
	id: string;
	profile_id: string;
	user_id: string;
	user_name: string;
	role: string;
	status: string;
	created_at: string;
}

// Developer API Key types
export interface DeveloperApiKey {
	id: string;
	name: string;
	key_prefix: string;
	scopes: string[];
	status: string;
	last_used_at?: string;
	created_at: string;
}
