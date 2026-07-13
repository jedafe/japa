import { PUBLIC_JC_API_URL } from '$env/static/public';

export const API_BASE = `${PUBLIC_JC_API_URL}/api/v1`;

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

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
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

// Custom-field types
export interface CustomField {
	id: string;
	group_id: string;
	key: string;
	label: string;
	field_type: string;
	required: boolean;
	options?: string[];
	placeholder?: string;
	description?: string;
	sort_order: number;
}

export interface CustomFieldGroup {
	id: string;
	name: string;
	entity_type: string;
	sort_order: number;
	fields?: CustomField[];
}

// Mirrors the backend's custom_field_values row (typed value columns).
export interface CustomFieldValueRow {
	id: string;
	field_id: string;
	entity_type: string;
	entity_id: string;
	value_text?: string | null;
	value_number?: number | string | null;
	value_bool?: boolean | null;
	value_date?: string | null;
	value_json?: string | null;
}

// Push subscription (mirrors the Web Push API PushSubscriptionJSON shape)
export interface PushSubscriptionJSON {
	endpoint: string;
	expirationTime?: number | null;
	keys: {
		p256dh: string;
		auth: string;
	};
}

// Organizations
export interface Organization {
	id: string;
	slug: string;
	name: string;
	headline?: string;
	bio?: string;
	logo_url?: string;
	is_public: boolean;
	member_count?: number;
	created_at?: string;
}

export interface OrganizationMember {
	user_id: string;
	username: string;
	display_name?: string;
	avatar_url?: string;
	role: string;
	joined_at?: string;
}

export interface OrganizationInvitation {
	id: string;
	email: string;
	role: string;
	status: string;
	created_at?: string;
}

// Coupons (TipTap coupon_embed render target)
export interface Coupon {
	id: string;
	code: string;
	title: string;
	description?: string;
	discount_label?: string;
	affiliate_url?: string;
	expires_at?: string | null;
	active: boolean;
}

// Affiliate program
export interface AffiliateLink {
	id: string;
	code: string;
	product_type: string;
	product_id: string;
	url: string;
	click_count?: number;
	conversion_count?: number;
	created_at?: string;
}

export interface AffiliateConversion {
	id: string;
	product_type: string;
	credits: number;
	converted_at: string;
}

// Mirrors the backend's EarningsSummary (internal/affiliate/service.go).
export interface AffiliateEarnings {
	total_clicks: number;
	total_conversions: number;
	total_earned: string;
	by_product: Array<{
		product_type: string;
		clicks: number;
		conversions: number;
		earned: string;
	}>;
	recent_events: Array<{
		id: string;
		event_type: string;
		commission_amount?: string;
		created_at: string;
		link_code: string;
		product_type: string;
	}>;
}

// Geo
export interface GeoLocation {
	id: string;
	name: string;
	country_code: string;
	region?: string;
	latitude?: number;
	longitude?: number;
}

// Space types & permission presets
export interface SpaceType {
	id: string;
	name: string;
	slug: string;
	description?: string;
	icon?: string;
}

export interface SpacePermissionPreset {
	name: string;
	label: string;
	description?: string;
	permissions: Record<string, boolean>;
}

export const api = {
	// Auth
	forgotPassword(email: string) {
		return request<{ status: string }>('/auth/forgot-password', {
			method: 'POST',
			body: JSON.stringify({ email })
		});
	},
	resetPassword(token: string, password: string) {
		return request<{ status: string }>('/auth/reset-password', {
			method: 'POST',
			body: JSON.stringify({ token, password })
		});
	},
	verifyEmail(token: string) {
		return request<void>('/auth/verify-email?token=' + encodeURIComponent(token));
	},
	resendVerification(email: string) {
		return request<{ status: string }>('/auth/resend-verification', {
			method: 'POST',
			body: JSON.stringify({ email })
		});
	},
	setupTOTP() {
		return request<{ secret: string; otpauth_url: string }>('/me/totp/enable', { method: 'POST' });
	},
	verifyTOTP(code: string) {
		return request<{ status: string }>('/me/totp/confirm', {
			method: 'POST',
			body: JSON.stringify({ code })
		});
	},
	disableTOTP(code: string) {
		return request<{ status: string }>('/me/totp', {
			method: 'DELETE',
			body: JSON.stringify({ code })
		});
	},

	// Content
	createContent(data: {
		space_id?: string;
		content_type: string;
		title: string;
		body: string;
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
	deleteContent(id: string) {
		return request<{ status: string }>('/content/' + id, { method: 'DELETE' });
	},

	// Blog
	submitForReview(id: string) {
		return request<{ status: string }>('/blog/' + id + '/submit-for-review', { method: 'POST' });
	},
	createBlogSeries(data: { title: string; description?: string }) {
		return request<{ id: string; slug: string }>('/blog/series', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	updateBlogSeries(id: string, data: { title?: string; description?: string }) {
		return request<{ id: string }>('/blog/series/' + id, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	deleteBlogSeries(id: string) {
		return request<{ status: string }>('/blog/series/' + id, { method: 'DELETE' });
	},
	addPostToSeries(seriesId: string, data: { content_id: string; position?: number }) {
		return request<{ status: string }>('/blog/series/' + seriesId + '/posts', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	removePostFromSeries(seriesId: string, postId: string) {
		return request<{ status: string }>('/blog/series/' + seriesId + '/posts/' + postId, { method: 'DELETE' });
	},
	reorderSeriesPosts(seriesId: string, data: { post_ids: string[] }) {
		return request<{ status: string }>('/blog/series/' + seriesId + '/posts/reorder', {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	createBlogReview(data: { content_id: string; rating: number; title?: string; body?: string }) {
		return request<{ id: string }>('/blog/reviews', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	listBlogReviews(params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<Array<{ id: string; content_id: string; rating: number; title?: string; body?: string; author_id: string; status: string; created_at: string }>>(
			'/blog/reviews' + (q ? '?' + q : '')
		);
	},
	updateBlogReview(id: string, data: { status?: string; rating?: number; title?: string; body?: string }) {
		return request<{ id: string }>('/blog/reviews/' + id, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	featurePost(data: { content_id: string; featured_order?: number; featured_until?: string }) {
		return request<{ id: string }>('/blog/featured', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	unfeaturePost(contentId: string) {
		return request<{ status: string }>('/blog/featured/' + contentId, { method: 'DELETE' });
	},
	reorderFeatured(data: { content_ids: string[] }) {
		return request<{ status: string }>('/blog/featured/reorder', {
			method: 'PATCH', body: JSON.stringify(data)
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
	getNotifications(params?: { page_size?: number; page_offset?: number }) {
		const query = params ? '?' + new URLSearchParams(params as Record<string, string>).toString() : '';
		return request<
			Array<{ id: string; title: string; body: string; read: boolean; created_at: string }>
		>('/me/notifications' + query);
	},
	getUnreadCount() {
		return request<{ count: number }>('/me/notifications/unread-count');
	},
	markNotificationRead(id: string) {
		return request<{ status: string }>('/me/notifications/' + id + '/read', { method: 'POST' });
	},
	markAllRead() {
		return request<{ status: string }>('/me/notifications/read-all', { method: 'POST' });
	},

	// Universities / Spaces
	listSpaces(params?: { space_type_id?: string; page_size?: number }) {
		const p = new URLSearchParams();
		// Backend filter param is `type` (a space_type UUID).
		if (params?.space_type_id) p.set('type', params.space_type_id);
		if (params?.page_size) p.set('page_size', String(params.page_size));
		const q = p.toString();
		return request<Array<{ id: string; name: string; slug: string; description: string }>>(
			'/spaces' + (q ? '?' + q : '')
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
		page_size?: number;
		page_offset?: number;
	}) {
		const p = new URLSearchParams();
		if (params?.space_id) p.set('space_id', params.space_id);
		if (params?.tag) p.set('tag', params.tag);
		if (params?.solved !== undefined) p.set('solved', String(params.solved));
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		// Backend returns a bare array of threads (no pagination wrapper).
		return request<ForumThread[]>('/forum' + (q ? '?' + q : ''));
	},
	getForumThread(slug: string) {
		return request<ForumThread>('/forum/' + slug);
	},
	createForumThread(data: {
		title: string;
		body: string;
		space_id?: string;
		tags?: string[];
		anonymous?: boolean;
	}) {
		return request<ForumThread>('/forum', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updateForumThread(id: string, data: { title?: string; body?: string }) {
		return request<ForumThread>('/forum/' + id, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteForumThread(id: string) {
		return request<{ status: string }>('/forum/' + id, { method: 'DELETE' });
	},
	solveForumThread(id: string) {
		return request<{ status: string }>('/forum/' + id + '/solve', { method: 'POST' });
	},
	getForumReplies(threadId: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<ForumReply[]>('/forum/' + threadId + '/replies' + (q ? '?' + q : ''));
	},
	createForumReply(threadId: string, data: { body: string; anonymous?: boolean }) {
		return request<ForumReply>('/forum/' + threadId + '/replies', {
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
		return request<{ status: string }>('/forum/replies/' + id, { method: 'DELETE' });
	},
	voteForum(data: { entity_type: 'thread' | 'reply'; entity_id: string; value: 1 | -1 }) {
		return request<{ status: string }>('/forum/vote', {
			method: 'POST',
			body: JSON.stringify({ ...data, direction: data.value === 1 ? 'up' : 'down' })
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
		return request<{ status: string }>('/forum/bookmarks/' + id, { method: 'DELETE' });
	},
	getForumBookmarks() {
		return request<Array<{ id: string; thread_id: string; created_at: string }>>('/forum/bookmarks');
	},

	// Tickets
	createTicket(data: { subject: string; description: string; type_id?: string; priority?: string; email?: string; field_values?: Record<string, unknown> }) {
		return request<Ticket>('/tickets', { method: 'POST', body: JSON.stringify(data) });
	},
	listTicketTypes() {
		return request<Array<{ id: string; name: string; slug: string; description?: string; default_priority: string; is_active: boolean }>>('/ticket-types');
	},
	listTickets(params?: { status?: string; page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.status) p.set('status', params.status);
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<Ticket[]>('/me/tickets' + (q ? '?' + q : ''));
	},
	getTicket(id: string) {
		return request<{ ticket: Ticket; comments: TicketComment[]; attachments: unknown[] }>('/me/tickets/' + id);
	},
	addTicketReply(id: string, data: { body: string }) {
		return request<TicketComment>('/me/tickets/' + id + '/comments', {
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
	createDMConversation(data: { user_id: string; body?: string }) {
		return request<{ id: string }>('/dm/conversations', {
			method: 'POST', body: JSON.stringify(data)
		});
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
	editDMMessage(conversationId: string, messageId: string, data: { body: string }) {
		return request<{ success: boolean }>('/dm/conversations/' + conversationId + '/messages/' + messageId, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	deleteDMMessage(conversationId: string, messageId: string) {
		return request<{ success: boolean }>('/dm/conversations/' + conversationId + '/messages/' + messageId, {
			method: 'DELETE'
		});
	},
	reportDMMessage(conversationId: string, messageId: string, data: { reason: string }) {
		return request<{ success: boolean }>('/dm/conversations/' + conversationId + '/messages/' + messageId + '/report', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	markDMRead(conversationId: string) {
		return request<{ success: boolean }>('/dm/conversations/' + conversationId + '/read', {
			method: 'POST'
		});
	},
	listDMRequests() {
		return request<DMConversation[]>('/dm/requests');
	},
	acceptDMRequest(conversationId: string) {
		return request<{ success: boolean }>('/dm/requests/' + conversationId + '/accept', {
			method: 'POST'
		});
	},
	declineDMRequest(conversationId: string) {
		return request<{ success: boolean }>('/dm/requests/' + conversationId + '/decline', {
			method: 'POST'
		});
	},
	blockDMUser(conversationId: string) {
		return request<{ success: boolean }>('/dm/requests/' + conversationId + '/block', {
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
	listProfessionalProfiles(params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<ProfessionalProfile[]>('/profiles' + (q ? '?' + q : ''));
	},
	getProfessionalProfile(slug: string) {
		return request<ProfessionalProfileDetail>('/profiles/' + slug);
	},
	createProfessionalProfile(data: {
		name: string;
		headline: string;
		skills: string[];
		is_public?: boolean;
	}) {
		return request<ProfessionalProfileDetail>('/profiles', {
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
		return request<ProfessionalProfileDetail>('/profiles/' + id, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteProfessionalProfile(id: string) {
		return request<{ status: string }>('/profiles/' + id, { method: 'DELETE' });
	},
	listRelocations(profileId: string) {
		return request<Relocation[]>('/profiles/' + profileId + '/relocations');
	},
	addRelocation(profileId: string, data: {
		country: string;
		city?: string;
		status: string;
		year?: number;
	}) {
		return request<Relocation>('/profiles/' + profileId + '/relocations', {
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
		return request<Relocation>('/profiles/relocations/' + relocationId, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteRelocation(relocationId: string) {
		return request<{ status: string }>('/profiles/relocations/' + relocationId, {
			method: 'DELETE'
		});
	},
	listDocuments(profileId: string) {
		return request<Document[]>('/profiles/' + profileId + '/documents');
	},
	addDocument(profileId: string, data: {
		document_type: string;
		title: string;
		description?: string;
		file_url?: string;
	}) {
		return request<Document>('/profiles/' + profileId + '/documents', {
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
		return request<Document>('/profiles/documents/' + documentId, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteDocument(documentId: string) {
		return request<{ status: string }>('/profiles/documents/' + documentId, {
			method: 'DELETE'
		});
	},
	listVerifiers(profileId: string) {
		return request<Verifier[]>('/profiles/' + profileId + '/verifiers');
	},
	assignVerifier(profileId: string, data: { user_id: string; role?: string }) {
		return request<Verifier>('/profiles/' + profileId + '/verifiers', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	followProfile(profileId: string) {
		return request<{ following: boolean }>('/profiles/' + profileId + '/follow', { method: 'POST' });
	},
	unfollowProfile(profileId: string) {
		return request<{ following: boolean }>('/profiles/' + profileId + '/follow', { method: 'DELETE' });
	},
	listProfileMembers(profileId: string) {
		return request<ProfileMember[]>('/profiles/' + profileId + '/members');
	},
	addProfileMember(profileId: string, data: { user_id: string; role: string }) {
		return request<ProfileMember>('/profiles/' + profileId + '/members', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	updateProfileMember(profileId: string, userId: string, data: { role: string }) {
		return request<ProfileMember>('/profiles/' + profileId + '/members/' + userId, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	removeProfileMember(profileId: string, userId: string) {
		return request<{ removed: boolean }>('/profiles/' + profileId + '/members/' + userId, { method: 'DELETE' });
	},
	inviteProfileMember(profileId: string, data: { email: string; role: string }) {
		return request<ProfileInvitation>('/profiles/' + profileId + '/members/invite', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	listProfileInvitations(profileId: string) {
		return request<ProfileInvitation[]>('/profiles/' + profileId + '/invitations');
	},
	sendProfileInvitation(profileId: string, data: { email: string; role: string }) {
		return request<ProfileInvitation>('/profiles/' + profileId + '/invitations', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	acceptProfileInvitation(token: string) {
		return request<{ accepted: boolean }>('/profiles/invitations/' + token + '/accept', { method: 'POST' });
	},
	getProfileContent(profileId: string) {
		return request<FeedItem[]>('/profiles/' + profileId + '/content');
	},

	// Developer API Keys
	listDeveloperKeys() {
		return request<DeveloperApiKey[]>('/me/api-keys');
	},
	createDeveloperKey(data: { name: string; scopes: string[] }) {
		return request<{ key: string; key_info: DeveloperApiKey }>('/me/api-keys', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	revokeDeveloperKey(id: string) {
		return request<{ status: string }>('/me/api-keys/' + id, { method: 'DELETE' });
	},
	updateDeveloperKey(id: string, data: { name?: string; scopes?: string[] }) {
		return request<DeveloperApiKey>('/me/api-keys/' + id, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},

	// Reviews & Ratings (catalog: /reviews/:entity_type/:entity_id[/aggregate])
	createReview(data: { entity_type: string; entity_id: string; rating: number; title?: string; body?: string }) {
		const { entity_type, entity_id, ...body } = data;
		return request<Review>(`/reviews/${entity_type}/${entity_id}`, {
			method: 'POST',
			body: JSON.stringify(body)
		});
	},
	listReviews(params: { entity_type: string; entity_id: string; page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params.page_size) p.set('page_size', String(params.page_size));
		if (params.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<Review[]>(
			`/reviews/${params.entity_type}/${params.entity_id}` + (q ? '?' + q : '')
		);
	},
	getReviewAggregate(params: { entity_type: string; entity_id: string }) {
		return request<{ average: number; count: number; distribution: number[] }>(
			`/reviews/${params.entity_type}/${params.entity_id}/aggregate`
		);
	},
	updateReview(id: string, data: { rating?: number; title?: string; body?: string }) {
		return request<Review>('/reviews/' + id, { method: 'PATCH', body: JSON.stringify(data) });
	},
	deleteReview(id: string) {
		return request<{ status: string }>('/reviews/' + id, { method: 'DELETE' });
	},
	flagReview(id: string, data: { reason: string }) {
		return request<{ status: string }>('/reviews/' + id + '/flag', { method: 'POST', body: JSON.stringify(data) });
	},

	// Space membership
	joinSpace(slug: string) {
		return request<{ joined: boolean; member_count: number }>('/spaces/' + slug + '/join', { method: 'POST' });
	},

	// Followers / Following
	getFollowers(userId: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FollowUser[]>('/users/' + userId + '/followers' + (q ? '?' + q : ''));
	},
	getFollowing(userId: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FollowUser[]>('/users/' + userId + '/following' + (q ? '?' + q : ''));
	},
	checkMutualFollow(userId: string, otherUserId: string) {
		return request<{ mutual: boolean }>('/users/' + userId + '/mutual/' + otherUserId);
	},

	// Content Reactions
	addReaction(contentId: string, reactionKey: string) {
		return request<{ id: string }>('/content/' + contentId + '/reactions', {
			method: 'POST', body: JSON.stringify({ key: reactionKey })
		});
	},
	removeReaction(contentId: string, reactionKey: string) {
		return request<{ status: string }>('/content/' + contentId + '/reactions/' + reactionKey, { method: 'DELETE' });
	},
	getReactions(contentId: string) {
		return request<Array<{ id: string; reaction_type: { key: string; label: string; emoji: string }; user_id: string; created_at: string }>>('/content/' + contentId + '/reactions');
	},
	listReactionTypes() {
		return request<Array<{ key: string; label: string; emoji: string }>>('/reaction-types');
	},

	// GDPR / Consent
	getConsents() {
		return request<Array<{ category: string; granted: boolean; granted_at?: string }>>('/consent');
	},
	updateConsent(data: { category: string; granted: boolean }) {
		return request<{ category: string; granted: boolean }>('/consent', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	revokeConsent(consentType: string) {
		return request<{ status: string }>('/consent/' + consentType, { method: 'DELETE' });
	},
	requestDataExport() {
		return request<{ request_id: string }>('/me/data-export', { method: 'POST' });
	},
	requestAccountDeletion() {
		return request<{ request_id: string }>('/me', { method: 'DELETE' });
	},

	// Content Pinning
	pinContent(id: string) {
		return request<{ status: string }>('/content/' + id + '/pin', { method: 'POST' });
	},
	unpinContent(id: string) {
		return request<{ status: string }>('/content/' + id + '/pin', { method: 'DELETE' });
	},

	// Content Revisions
	getRevisions(contentId: string) {
		return request<Array<{ id: string; version: number; created_at: string; author_name: string; summary?: string }>>('/content/' + contentId + '/revisions');
	},

	// Forms
	listForms(params?: { space_id?: string }) {
		const q = params?.space_id ? '?space_id=' + params.space_id : '';
		return request<Array<{ id: string; title: string; slug: string; space_id?: string; submission_count: number }>>('/forms' + q);
	},
	getForm(idOrSlug: string) {
		return request<{ id: string; title: string; slug: string; fields: Array<{ id: string; key: string; label: string; field_type: string; required: boolean; options?: string[] }> }>('/forms/' + idOrSlug);
	},
	createForm(data: { title: string; space_id?: string; fields: Array<{ key: string; label: string; field_type: string; required?: boolean; options?: string[] }> }) {
		return request<{ id: string; slug: string }>('/forms', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	updateForm(id: string, data: { title?: string; fields?: Array<{ key: string; label: string; field_type: string; required?: boolean; options?: string[] }> }) {
		return request<{ id: string }>('/forms/' + id, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	deleteForm(id: string) {
		return request<{ status: string }>('/forms/' + id, { method: 'DELETE' });
	},
	submitForm(id: string, data: { values: Record<string, unknown> }) {
		return request<{ id: string }>('/forms/' + id + '/submissions', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	getFormSubmissions(id: string) {
		return request<Array<{ id: string; values: Record<string, unknown>; created_at: string }>>('/forms/' + id + '/submissions');
	},

	// Learning
	listLearnTracks() {
		return request<Array<{ id: string; title: string; slug: string; description: string; module_count: number; status: string }>>('/learn/tracks');
	},
	getLearnTrack(slugOrId: string) {
		return request<{ id: string; title: string; slug: string; description: string; modules: Array<{ slug: string; title: string; lessons: Array<{ slug: string; title: string }> }> }>('/learn/tracks/' + slugOrId);
	},
	enrollInTrack(trackId: string) {
		return request<{ enrolled: boolean }>('/learn/tracks/' + trackId + '/enroll', { method: 'POST' });
	},
	getLearnProgress(trackId: string) {
		return request<{ completed_lessons: string[]; total_lessons: number; percentage: number }>('/learn/progress/' + trackId);
	},
	getLearnLesson(id: string) {
		return request<{ id: string; title: string; body: string; body_html: string; track_slug: string; module_slug: string; slug: string }>('/learn/lessons/' + id);
	},
	completeLesson(lessonId: string) {
		return request<{ completed: boolean }>('/learn/lessons/' + lessonId + '/complete', { method: 'POST' });
	},
	updateLessonProgress(lessonId: string, data: { progress: number }) {
		return request<{ updated: boolean }>('/learn/lessons/' + lessonId + '/progress', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	getLearnQuiz(id: string) {
		return request<{ id: string; title: string; questions: Array<{ id: string; question: string; options: string[]; correct_index: number }> }>('/learn/quizzes/' + id);
	},
	submitQuiz(quizId: string, data: { answers: number[] }) {
		return request<{ score: number; total: number; passed: boolean }>('/learn/quizzes/' + quizId + '/submit', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	getCertificates() {
		return request<Array<{ id: string; track_title: string; issued_at: string; download_url: string }>>('/me/certificates');
	},
	getCertificate(id: string) {
		return request<{ id: string; track_title: string; issued_at: string; download_url: string }>('/certificates/' + id);
	},
	downloadCertificate(id: string) {
		return request<{ download_url: string }>('/certificates/' + id + '/download');
	},

	// Payments
	initiatePayment(data: {
		amount: number;
		currency: string;
		provider: 'paystack' | 'stripe' | 'nowpayments' | 'credit';
		purpose: string;
		metadata?: Record<string, unknown>;
	}) {
		return request<{ payment_id: string; checkout_url?: string; reference: string; provider: string }>(
			'/payments/intent',
			{ method: 'POST', body: JSON.stringify(data) }
		);
	},
	getPayment(id: string) {
		return request<PaymentDetail>('/me/payments/' + id);
	},
	requestRefund(id: string, data: { reason: string }) {
		return request<{ refund_id: string; status: string }>(
			'/payments/' + id + '/refund',
			{ method: 'POST', body: JSON.stringify(data) }
		);
	},

	// Subscriptions
	createSubscription(data: {
		plan_id: string;
		discount_code?: string;
		payment_provider: string;
	}) {
		return request<{ subscription_id: string; checkout_url?: string }>(
			'/subscriptions',
			{ method: 'POST', body: JSON.stringify(data) }
		);
	},

	// Discount Codes
	validateDiscountCode(code: string) {
		return request<{
			valid: boolean;
			discount_type: string;
			discount_value: number;
			expires_at?: string;
		}>('/payments/discount-codes/validate', {
			method: 'POST',
			body: JSON.stringify({ code })
		});
	},

	// Wallet
	getWalletBalance() {
		return request<{ points: number; credits: number }>('/me/wallet');
	},

	// Pricing Plans
	getPricingPlans() {
		return request<Array<{
			id: string;
			name: string;
			description: string;
			features: Array<{ text: string; included: boolean }>;
			monthly: { amount: number; currency: string };
			yearly: { amount: number; currency: string };
			highlighted: boolean;
		}>>('/pricing/plans');
	},

	// Payment Providers
	getPaymentProviders() {
		return request<Array<{
			id: string;
			name: string;
			description: string;
			enabled: boolean;
		}>>('/payment-providers');
	},

	// WebSocket
	getWsTicket() {
		return request<{ ticket: string }>('/auth/ws-ticket', { method: 'POST' });
	},
	getOnlineUsers() {
		return request<Array<{ user_id: string; username: string; last_seen: string }>>('/ws/online');
	},

	// Wallet
	getWalletLedger(params?: { page_size?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		const q = p.toString();
		return request<WalletLedgerEntry[]>('/me/wallet/ledger' + (q ? '?' + q : ''));
	},
	convertWallet(data: { points: number }) {
		return request<{ points: number; credits: number }>('/me/wallet/convert', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},

	// Payments (list)
	listPayments(params?: { page_size?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		const q = p.toString();
		return request<PaymentListItem[]>('/me/payments' + (q ? '?' + q : ''));
	},

	// Subscriptions
	getActiveSubscription() {
		return request<SubscriptionDetail | null>('/me/subscription');
	},
	cancelSubscription() {
		return request<{ success: boolean }>('/me/subscription/cancel', { method: 'POST' });
	},

	// Search
	search(query: string, params?: { limit?: number }) {
		const p = new URLSearchParams({ q: query });
		if (params?.limit) p.set('limit', String(params.limit));
		return request<SearchResult[]>('/search?' + p.toString());
	},

	// Users
	getUser(username: string) {
		return request<UserProfile>('/users/' + username);
	},
	listUsers(params?: { limit?: number; search?: string }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.search) p.set('search', params.search);
		const q = p.toString();
		return request<UserProfile[]>('/users' + (q ? '?' + q : ''));
	},
	getMe() {
		return request<UserProfile>('/me');
	},
	updateMe(data: { display_name?: string; bio?: string; location?: string }) {
		return request<UserProfile>('/me/profile', {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},

	// Comparisons
	getComparisonTypes(params?: { page_size?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		const q = p.toString();
		return request<ComparisonType[]>('/comparisons/types' + (q ? '?' + q : ''));
	},
	createComparisonType(data: { name: string; slug: string; description: string; is_public: boolean }) {
		return request<ComparisonType>('/comparisons/types', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	updateComparisonType(id: string, data: { name?: string; description?: string; is_public?: boolean }) {
		return request<ComparisonType>('/comparisons/types/' + id, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	deleteComparisonType(id: string) {
		return request<{ deleted: boolean }>('/comparisons/types/' + id, { method: 'DELETE' });
	},
	getComparisonTypeAccess(id: string) {
		return request<{ allowed_groups: string[]; allowed_roles: string[] }>('/comparisons/types/' + id + '/access');
	},
	setComparisonTypeAccess(id: string, data: { allowed_groups: string[]; allowed_roles: string[] }) {
		return request<{ updated: boolean }>('/comparisons/types/' + id + '/access', {
			method: 'PUT', body: JSON.stringify(data)
		});
	},
	getComparison(id: string) {
		return request<{ id: string; title: string; slug: string; type_id: string; item_count: number; vote_count: number; created_at: string }>('/comparisons/' + id);
	},
	updateComparison(id: string, data: { title?: string }) {
		return request<{ id: string }>('/comparisons/' + id, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	deleteComparison(id: string) {
		return request<{ deleted: boolean }>('/comparisons/' + id, { method: 'DELETE' });
	},
	addComparisonItem(comparisonId: string, data: { name: string; values?: Record<string, string> }) {
		return request<{ id: string }>('/comparisons/' + comparisonId + '/items', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	updateComparisonItem(itemId: string, data: { name?: string; values?: Record<string, string> }) {
		return request<{ id: string }>('/comparisons/items/' + itemId, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	deleteComparisonItem(itemId: string) {
		return request<{ deleted: boolean }>('/comparisons/items/' + itemId, { method: 'DELETE' });
	},
	reorderComparisonItems(comparisonId: string, data: { item_ids: string[] }) {
		return request<{ status: string }>('/comparisons/' + comparisonId + '/items/reorder', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	addComparisonDimension(comparisonId: string, data: { name: string; description?: string }) {
		return request<{ id: string }>('/comparisons/' + comparisonId + '/dimensions', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	updateComparisonDimension(dimensionId: string, data: { name?: string; description?: string }) {
		return request<{ id: string }>('/comparisons/dimensions/' + dimensionId, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	deleteComparisonDimension(dimensionId: string) {
		return request<{ deleted: boolean }>('/comparisons/dimensions/' + dimensionId, { method: 'DELETE' });
	},
	setDimensionValues(dimensionId: string, data: { values: Record<string, string> }) {
		return request<{ updated: boolean }>('/comparisons/dimensions/' + dimensionId + '/values', {
			method: 'PUT', body: JSON.stringify(data)
		});
	},

	// Media
	listMedia(params?: { page_size?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		const q = p.toString();
		return request<MediaItem[]>('/media' + (q ? '?' + q : ''));
	},
	uploadMedia(file: File) {
		const formData = new FormData();
		formData.append('file', file);
		const headers: Record<string, string> = {};
		if (jwtToken) headers['Authorization'] = `Bearer ${jwtToken}`;
		return fetch(`${API_BASE}/media/upload`, {
			method: 'POST',
			headers,
			body: formData
		}).then(async (res) => {
			const body = (await res.json()) as ApiOk<MediaItem> | ApiErr;
			if (!res.ok || 'error' in body) {
				const err = 'error' in body ? body.error : { code: 'unknown', message: res.statusText };
				throw new Error(`${err.code}: ${err.message}`);
			}
			return (body as ApiOk<MediaItem>).data;
		});
	},
	deleteMedia(id: string) {
		return request<{ status: string }>('/media/' + id, { method: 'DELETE' });
	},

	// Analytics
	getDashboardAnalytics() {
		return request<DashboardData>('/analytics/dashboard');
	},
	trackAnalyticsEvent(data: { event: string; data: Record<string, unknown>; timestamp: string }) {
		return request<{ tracked: boolean }>('/analytics/events', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},

	// Auth (login/register)
	login(email: string, password: string) {
		return request<{ access_token: string; refresh_token: string; user: UserProfile }>('/auth/login', {
			method: 'POST',
			body: JSON.stringify({ email, password })
		});
	},
	register(data: { username: string; email: string; password: string }) {
		return request<{ access_token: string; refresh_token: string; user: UserProfile }>('/auth/register', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},

	// Link Preview
	getLinkPreview(url: string) {
		return request<LinkPreviewData>('/link-preview?url=' + encodeURIComponent(url));
	},

	// Content Render
	renderContent(html: string) {
		return request<{ rendered: string }>('/content/render', {
			method: 'POST',
			body: JSON.stringify({ html })
		});
	},

	// Ads
	renderAd(slot: string) {
		return request<AdRender>('/ads/serve?slot=' + encodeURIComponent(slot));
	},
	trackAdEvent(adId: string, event: 'impression' | 'click') {
		return request<{ tracked: boolean }>('/ads/events', {
			method: 'POST',
			body: JSON.stringify({ ad_id: adId, event })
		});
	},

	// Feed
	getFeedPrimary(params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/feed' + (q ? '?' + q : ''));
	},
	getSpaceFeed(spaceId: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/feed/space/' + spaceId + (q ? '?' + q : ''));
	},
	getProfileFeed(userId: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/feed/profile/' + userId + (q ? '?' + q : ''));
	},
	getExploreFeed(params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/explore' + (q ? '?' + q : ''));
	},
	getCardStyle() {
		return request<{ card_style: string }>('/feed/card-style');
	},

	// Onboarding recommendations
	getOnboardingRecommendations() {
		return request<Array<{ id: string; name: string; slug: string; description: string }>>('/onboarding/recommendations');
	},

	// Comparison create + matrix
	createComparison(data: { type_id: string; title: string; items: Array<{ name: string; values: Record<string, string> }> }) {
		return request<{ id: string; slug: string }>('/comparisons', {
			method: 'POST', body: JSON.stringify(data)
		});
	},
	getComparisonMatrix(id: string) {
		return request<{ dimensions: Array<{ id: string; name: string }>; items: Array<{ id: string; name: string; values: Record<string, string> }> }>('/comparisons/' + id + '/matrix');
	},

	// Space creation
	createSpace(data: { name: string; slug: string; description: string; space_type_id?: string; is_public?: boolean }) {
		return request<{ id: string; slug: string }>('/spaces', {
			method: 'POST', body: JSON.stringify(data)
		});
	},

	// Notification preferences
	getNotificationPreferences() {
		return request<Array<{ type: string; email: boolean; push: boolean; in_app: boolean }>>('/me/notification-preferences');
	},
	updateNotificationPreference(type: string, data: { email?: boolean; push?: boolean; in_app?: boolean }) {
		return request<{ type: string }>('/me/notification-preferences/' + type, {
			method: 'PATCH', body: JSON.stringify(data)
		});
	},
	getNotificationTypes() {
		return request<Array<{ key: string; label: string; description: string }>>('/me/notification-types');
	},

	// Webhooks
	listWebhooks() {
		return request<Webhook[]>('/admin/webhooks');
	},
	createWebhook(data: { url: string; events: string[]; secret?: string }) {
		return request<Webhook>('/admin/webhooks', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	deleteWebhook(id: string) {
		return request<{ status: string }>('/admin/webhooks/' + id, { method: 'DELETE' });
	},
	getWebhookDeliveries(id: string) {
		return request<WebhookDelivery[]>('/admin/webhooks/' + id + '/deliveries');
	},

	// ── Trending (bare arrays from the backend) ──────────────────────────────
	getTrendingContent(limit = 10) {
		return request<FeedItem[]>(`/trending/content?limit=${limit}`);
	},
	getTrendingSpaces(limit = 10) {
		return request<unknown[]>(`/trending/spaces?limit=${limit}`);
	},
	getTrendingTags(limit = 10) {
		return request<{ id: string; slug: string; name: string; post_count?: number }[]>(`/trending/tags?limit=${limit}`);
	},
	getTrendingUsers(limit = 10) {
		return request<{ id: string; username: string; display_name?: string; avatar_url?: string }[]>(`/trending/users?limit=${limit}`);
	},

	// ── Suggestions / Discovery (bare arrays from the backend) ────────────────
	getSuggestedSpaces() {
		return request<{ id: string; slug: string; name: string; member_count?: number }[]>('/suggestions/spaces');
	},
	getSuggestedUsers() {
		return request<{ id: string; username: string; display_name?: string; avatar_url?: string }[]>('/suggestions/users');
	},
	getSuggestedTags() {
		return request<{ id: string; slug: string; name: string; post_count?: number }[]>('/suggestions/tags');
	},
	followUser(userId: string) {
		return request<{ followed: boolean }>(`/users/${userId}/follow`, { method: 'POST' });
	},

	// ── Web Push (AUDIT-FE wiring) ────────────────────────────────────────────
	getPushVapidKey() {
		return request<{ public_key: string }>('/push/vapid-key');
	},
	subscribePush(subscription: PushSubscriptionJSON) {
		return request<{ subscribed: boolean }>('/me/push-tokens', {
			method: 'POST',
			body: JSON.stringify({ subscription })
		});
	},

	// ── AI Editor Assist (AUDIT-FE wiring) ───────────────────────────────────
	aiImprove(content: string, instruction?: string) {
		return request<{ result: string }>('/ai/assist/improve', {
			method: 'POST',
			body: JSON.stringify({ content, instruction })
		});
	},
	aiSummarize(content: string) {
		return request<{ result: string }>('/ai/assist/summarize', {
			method: 'POST',
			body: JSON.stringify({ content })
		});
	},
	aiTranslate(content: string, target_language: string) {
		return request<{ result: string }>('/ai/assist/translate', {
			method: 'POST',
			body: JSON.stringify({ content, target_language })
		});
	},
	aiGenerateTags(content: string, title?: string) {
		return request<{ tags: string[] }>('/ai/assist/generate-tags', {
			method: 'POST',
			body: JSON.stringify({ content, title })
		});
	},
	aiGenerateOG(content: string, title?: string) {
		return request<{ og_title: string; og_description: string; og_image_prompt?: string }>(
			'/ai/assist/generate-og',
			{ method: 'POST', body: JSON.stringify({ content, title }) }
		);
	},

	// ── Custom Fields (bare arrays from the backend) ─────────────────────────
	getCustomFieldGroups(entityType: string) {
		return request<CustomFieldGroup[]>(`/custom-field-groups?entity_type=${entityType}`);
	},
	getCustomFieldGroupFields(groupId: string) {
		return request<CustomField[]>(`/custom-field-groups/${groupId}/fields`);
	},
	getCustomFieldValues(entityType: string, entityId: string) {
		return request<CustomFieldValueRow[]>(`/custom-field-values/${entityType}/${entityId}`);
	},
	setCustomFieldValues(entityType: string, entityId: string, values: Record<string, unknown>) {
		return request<{ saved: boolean }>('/custom-field-values', {
			method: 'PUT',
			body: JSON.stringify({ entity_type: entityType, entity_id: entityId, values })
		});
	},

	// ── Comments mutations (AUDIT-FE wiring — edit/delete/vote on individual comments) ──
	updateComment(id: string, body: string) {
		return request<{ id: string }>(`/comments/${id}`, {
			method: 'PATCH',
			body: JSON.stringify({ body })
		});
	},
	deleteComment(id: string) {
		return request<{ status: string }>(`/comments/${id}`, { method: 'DELETE' });
	},
	voteComment(id: string, value: 1 | -1) {
		return request<{ status: string }>(`/comments/${id}/vote`, {
			method: 'POST',
			body: JSON.stringify({ direction: value === 1 ? 'up' : 'down' })
		});
	},
	removeCommentVote(id: string) {
		return request<{ status: string }>(`/comments/${id}/vote`, { method: 'DELETE' });
	},
	pinComment(id: string) {
		return request<{ status: string }>(`/comments/${id}/pin`, { method: 'POST' });
	},
	unpinComment(id: string) {
		return request<{ status: string }>(`/comments/${id}/pin`, { method: 'DELETE' });
	},
	reportComment(id: string, data: { reason: string }) {
		return request<{ reported: boolean }>(`/comments/${id}/report`, {
			method: 'POST', body: JSON.stringify(data)
		});
	},

	// ── Organizations ──────────────────────────────────────────────────────────
	listOrganizations(pageSize = 50, pageOffset = 0) {
		return request<Organization[]>(
			`/organizations?page_size=${pageSize}&page_offset=${pageOffset}`
		);
	},
	getOrganization(slug: string) {
		return request<Organization>(`/organizations/${slug}`);
	},
	createOrganization(data: { name: string; slug?: string; headline?: string; bio?: string }) {
		return request<Organization>('/organizations', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updateOrganization(id: string, data: Record<string, unknown>) {
		return request<Organization>(`/organizations/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deleteOrganization(id: string) {
		return request<{ status: string }>(`/organizations/${id}`, { method: 'DELETE' });
	},
	getOrganizationMembers(id: string) {
		return request<OrganizationMember[]>(`/organizations/${id}/members`);
	},
	addOrganizationMember(id: string, userId: string, role: string) {
		return request<{ added: boolean }>(`/organizations/${id}/members`, {
			method: 'POST',
			body: JSON.stringify({ user_id: userId, role })
		});
	},
	updateOrganizationMember(id: string, userId: string, role: string) {
		return request<{ updated: boolean }>(`/organizations/${id}/members/${userId}`, {
			method: 'PATCH',
			body: JSON.stringify({ role })
		});
	},
	removeOrganizationMember(id: string, userId: string) {
		return request<{ status: string }>(`/organizations/${id}/members/${userId}`, {
			method: 'DELETE'
		});
	},
	listOrganizationInvitations(id: string) {
		return request<OrganizationInvitation[]>(`/organizations/${id}/invitations`);
	},
	inviteToOrganization(id: string, email: string, role: string) {
		return request<{ invited: boolean }>(`/organizations/${id}/invitations`, {
			method: 'POST',
			body: JSON.stringify({ email, role })
		});
	},
	acceptOrganizationInvitation(token: string) {
		return request<{ message: string }>(
			`/organizations/invitations/${token}/accept`,
			{ method: 'POST' }
		);
	},

	// ── Claim ownership (AUDIT-FE closing gap) ────────────────────────────────
	submitOwnershipClaim(data: { entity_type: string; entity_id: string; evidence: string }) {
		return request<{ id: string; status: string }>('/claim-ownership', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updateOwnershipClaim(id: string, status: string) {
		return request<{ id: string; status: string }>(`/claim-ownership/${id}`, {
			method: 'PATCH',
			body: JSON.stringify({ status })
		});
	},

	// ── Coupons (AUDIT-FE closing gap — powers the TipTap coupon_embed render) ─
	listPublicCoupons(params?: { provider_id?: string; coupon_type?: string; search?: string; page?: number; page_size?: number }) {
		const p = new URLSearchParams();
		if (params?.provider_id) p.set('provider_id', params.provider_id);
		if (params?.coupon_type) p.set('coupon_type', params.coupon_type);
		if (params?.search) p.set('search', params.search);
		if (params?.page) p.set('page', String(params.page));
		if (params?.page_size) p.set('page_size', String(params.page_size));
		const q = p.toString();
		return request<{ coupons: Coupon[]; total: number }>(`/coupons${q ? '?' + q : ''}`);
	},
	getCoupon(id: string) {
		return request<Coupon>(`/coupons/${id}`);
	},
	trackCouponClick(id: string) {
		return request<{ tracked: boolean }>(`/coupons/${id}/click`, { method: 'POST' });
	},

	// ── Affiliate program (AUDIT-FE closing gap) ──────────────────────────────
	getProgramInfo() {
		return request<{ name: string; description: string; commission_rates: Record<string, number>; requirements: string[] }>('/affiliate-program');
	},
	enrollAffiliate() {
		return request<{ enrolled: boolean }>('/affiliate-program/enroll', { method: 'POST' });
	},
	getAffiliateLinks() {
		return request<AffiliateLink[]>('/me/affiliate-links');
	},
	getAffiliateLink(id: string) {
		return request<AffiliateLink>(`/me/affiliate-links/${id}`);
	},
	createAffiliateLink(data: { product_type: string; product_id: string }) {
		return request<AffiliateLink>('/me/affiliate-links', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	deleteAffiliateLink(id: string) {
		return request<{ status: string }>(`/me/affiliate-links/${id}`, { method: 'DELETE' });
	},
	getAffiliateEarnings() {
		return request<AffiliateEarnings>('/me/affiliate-earnings');
	},

	// ── Geo (AUDIT-FE closing gap — location resolution for onboarding/spaces) ─
	geoMe() {
		return request<{ country: string; country_code: string; region?: string; city?: string }>(
			'/geo/me'
		);
	},
	geoLookup(ip: string) {
		// Backend looks up a single IP (?ip=) and returns one geo record.
		return request<{ country: string; country_code: string; region?: string; city?: string }>(
			`/geo/lookup?ip=${encodeURIComponent(ip)}`
		);
	},
	geoCountries() {
		return request<Array<{ code: string; name: string }>>('/geo/countries');
	},

	// ── Space types & permission presets ───────────────────────────────────────
	listSpaceTypes() {
		return request<SpaceType[]>('/space-types');
	},
	getSpaceType(id: string) {
		return request<SpaceType>(`/space-types/${id}`);
	},
	listSpacePermissionPresets() {
		return request<SpacePermissionPreset[]>('/space-permission-presets');
	},
	async getSpacePermissionPreset(name: string) {
		// No GET-by-name endpoint exists; fetch the list and select.
		const presets = await request<SpacePermissionPreset[]>('/space-permission-presets');
		return presets.find((p) => p.name === name) ?? null;
	},

	// ── Webhook events (AUDIT-FE-014) ─────────────────────────────────────
	listWebhookEvents() {
		return request<Array<{ key: string; label: string; description: string }>>('/webhook-events');
	},

	// ── Content types (AUDIT-FE-016) ──────────────────────────────────────
	listContentTypes() {
		return request<Array<{ value: string; label: string }>>('/content-types');
	},

	// ── Custom field entity types (AUDIT-FE-019) ──────────────────────────
	listCustomFieldEntityTypes() {
		return request<Array<{ value: string; label: string }>>('/custom-field-entity-types');
	},

	// ── AI categories (AUDIT-FE-020) ──────────────────────────────────────
	listAICategories() {
		return request<Array<{ value: string; label: string; description: string }>>('/ai/categories');
	},

	// ── Developer API scopes (AUDIT-FE-015) ───────────────────────────────
	listDeveloperScopes() {
		return request<Array<{ value: string; label: string; description: string }>>('/developer/scopes');
	},

	// ── Page Builder ─────────────────────────────────────────────────────
	createPage(data: { title: string; slug: string; page_schema: Record<string, unknown> }) {
		return request<{ id: string; title: string; slug: string }>('/pages', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	updatePage(id: string, data: { title?: string; page_schema?: Record<string, unknown> }) {
		return request<{ id: string }>(`/pages/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		});
	},
	deletePage(id: string) {
		return request<{ status: string }>(`/pages/${id}`, { method: 'DELETE' });
	},

	// Bookmarks
	getBookmarkFolders() {
		return request<{ id: string; name: string; description?: string; item_count: number; created_at: string }[]>('/me/bookmark-folders');
	},
	createBookmarkFolder(data: { name: string; description?: string }) {
		return request<{ id: string }>('/me/bookmark-folders', { method: 'POST', body: JSON.stringify(data) });
	},
	updateBookmarkFolder(id: string, data: { name: string; description?: string }) {
		return request<{ id: string }>(`/me/bookmark-folders/${id}`, { method: 'PATCH', body: JSON.stringify(data) });
	},
	deleteBookmarkFolder(id: string) {
		return request<{ status: string }>(`/me/bookmark-folders/${id}`, { method: 'DELETE' });
	},
	getBookmarks(folderId?: string) {
		const q = folderId ? `?folder_id=${folderId}` : '';
		return request<{ id: string; content_id: string; title: string; content_type: string; folder_id?: string; created_at: string }[]>(`/me/bookmarks${q}`);
	},

	// Drafts
	getDrafts() {
		return request<{ id: string; title: string; content_type: string; status: string; updated_at: string; created_at: string; scheduled_at?: string }[]>('/me/drafts');
	},
	getScheduled() {
		return request<{ id: string; title: string; content_type: string; status: string; updated_at: string; created_at: string; scheduled_at?: string }[]>('/me/scheduled');
	},

	// ── /me endpoints (missing from prior audit) ─────────────────────────────
	getMePermissions() {
		return request<{ permissions: string[]; roles: string[] }>('/me/permissions');
	},
	updateMeAccount(data: { display_name?: string; bio?: string; location?: string; avatar_url?: string }) {
		return request<UserProfile>('/me/account', { method: 'PATCH', body: JSON.stringify(data) });
	},
	changePassword(data: { current_password: string; new_password: string }) {
		return request<{ status: string }>('/me/change-password', { method: 'POST', body: JSON.stringify(data) });
	},
	getMePrivacy() {
		return request<{ profile_visibility: string; show_email: boolean; show_location: boolean; allow_dms: string }>('/me/privacy');
	},
	updateMePrivacy(data: { profile_visibility?: string; show_email?: boolean; show_location?: boolean; allow_dms?: string }) {
		return request<{ updated: boolean }>('/me/privacy', { method: 'PATCH', body: JSON.stringify(data) });
	},
	getMeSessions() {
		return request<Array<{ id: string; ip: string; user_agent: string; created_at: string; last_active_at: string; current: boolean }>>('/me/sessions');
	},
	deleteMeSession(id: string) {
		return request<{ status: string }>('/me/sessions/' + id, { method: 'DELETE' });
	},
	deleteAllMeSessions() {
		return request<{ status: string }>('/me/sessions', { method: 'DELETE' });
	},
	getMeConnectedAccounts() {
		return request<Array<{ provider: string; provider_id: string; connected_at: string }>>('/me/connected-accounts');
	},
	deleteMeConnectedAccount(provider: string) {
		return request<{ status: string }>('/me/connected-accounts/' + provider, { method: 'DELETE' });
	},
	getMeContent(params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/me/content' + (q ? '?' + q : ''));
	},
	getMeSpaces() {
		return request<Array<{ id: string; name: string; slug: string; role: string }>>('/me/spaces');
	},
	getMeSpacesModerated() {
		return request<Array<{ id: string; name: string; slug: string }>>('/me/spaces/moderated');
	},
	getMeSpacesOwned() {
		return request<Array<{ id: string; name: string; slug: string }>>('/me/spaces/owned');
	},
	getMeFollowedTags() {
		return request<Array<{ id: string; name: string; slug: string; followed_at: string }>>('/me/followed-tags');
	},
	getMeFollowing() {
		return request<Array<{ id: string; username: string; display_name?: string; avatar_url?: string }>>('/me/following');
	},
	getMeMutedUsers() {
		return request<Array<{ id: string; username: string; display_name?: string; muted_at: string }>>('/me/muted-users');
	},
	getMeMutedSpaces() {
		return request<Array<{ id: string; name: string; slug: string; muted_at: string }>>('/me/muted-spaces');
	},
	getMeBlockedUsers() {
		return request<Array<{ id: string; username: string; display_name?: string; blocked_at: string }>>('/me/blocked-users');
	},
	getMeAnalytics() {
		return request<{ views: number; followers: number; content_count: number; engagement_rate: number }>('/me/analytics');
	},
	getMeReports() {
		return request<Array<{ id: string; entity_type: string; entity_id: string; reason: string; status: string; created_at: string }>>('/me/reports');
	},
	getDataExportStatus() {
		return request<{ status: string; requested_at: string; download_url?: string }>('/me/data-export/status');
	},
	downloadDataExport() {
		return request<{ download_url: string }>('/me/data-export/download');
	},
	getMePreferences() {
		return request<Record<string, unknown>>('/me/preferences');
	},
	updateMePreferences(data: Record<string, unknown>) {
		return request<{ updated: boolean }>('/me/preferences', { method: 'PUT', body: JSON.stringify(data) });
	},
	updateNotificationPreferencesBulk(data: Array<{ type: string; email?: boolean; push?: boolean; in_app?: boolean }>) {
		return request<{ updated: boolean }>('/me/notification-preferences', { method: 'PATCH', body: JSON.stringify(data) });
	},
	reactivateSubscription() {
		return request<{ success: boolean }>('/me/subscription/reactivate', { method: 'POST' });
	},
	getMeCertificates() {
		return request<Array<{ id: string; track_title: string; issued_at: string; download_url: string }>>('/me/certificates');
	},

	// ── API Key management (missing methods) ────────────────────────────────
	getApiKeyById(id: string) {
		return request<DeveloperApiKey>('/me/api-keys/' + id);
	},
	rotateApiKey(id: string) {
		return request<{ key: string }>('/me/api-keys/' + id + '/rotate', { method: 'POST' });
	},
	getApiKeyIpAllowlist(id: string) {
		return request<Array<{ id: string; ip: string; label?: string; created_at: string }>>('/me/api-keys/' + id + '/ip-allowlist');
	},
	addApiKeyIpAllowlist(id: string, data: { ip: string; label?: string }) {
		return request<{ id: string }>('/me/api-keys/' + id + '/ip-allowlist', { method: 'POST', body: JSON.stringify(data) });
	},
	removeApiKeyIpAllowlist(keyId: string, ipId: string) {
		return request<{ status: string }>('/me/api-keys/' + keyId + '/ip-allowlist/' + ipId, { method: 'DELETE' });
	},

	// ── Space management (missing methods) ──────────────────────────────────
	updateSpace(id: string, data: Record<string, unknown>) {
		return request<{ id: string }>('/spaces/' + id, { method: 'PATCH', body: JSON.stringify(data) });
	},
	deleteSpace(id: string) {
		return request<{ status: string }>('/spaces/' + id, { method: 'DELETE' });
	},
	leaveSpace(slug: string) {
		return request<{ left: boolean }>('/spaces/' + slug + '/leave', { method: 'DELETE' });
	},
	followSpace(slug: string) {
		return request<{ following: boolean }>('/spaces/' + slug + '/follow', { method: 'POST' });
	},
	unfollowSpace(slug: string) {
		return request<{ following: boolean }>('/spaces/' + slug + '/follow', { method: 'DELETE' });
	},
	muteSpace(slug: string) {
		return request<{ muted: boolean }>('/spaces/' + slug + '/mute', { method: 'POST' });
	},
	unmuteSpace(slug: string) {
		return request<{ muted: boolean }>('/spaces/' + slug + '/mute', { method: 'DELETE' });
	},
	getMuteStatus(slug: string) {
		return request<{ muted: boolean }>('/spaces/' + slug + '/mute-status');
	},
	getSpaceContent(slug: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/spaces/' + slug + '/content' + (q ? '?' + q : ''));
	},
	getSpacePinned(slug: string) {
		return request<Array<{ id: string; content_id: string; position: number }>>('/spaces/' + slug + '/pinned');
	},
	pinToSpace(slug: string, contentId: string) {
		return request<{ id: string }>('/spaces/' + slug + '/pinned', { method: 'POST', body: JSON.stringify({ content_id: contentId }) });
	},
	unpinFromSpace(slug: string, contentId: string) {
		return request<{ status: string }>('/spaces/' + slug + '/pinned/' + contentId, { method: 'DELETE' });
	},
	reorderPinned(spaceId: string, data: { pinned_ids: string[] }) {
		return request<{ status: string }>('/spaces/' + spaceId + '/pinned/reorder', { method: 'PATCH', body: JSON.stringify(data) });
	},
	reportSpace(id: string, data: { reason: string }) {
		return request<{ reported: boolean }>('/spaces/' + id + '/report', { method: 'POST', body: JSON.stringify(data) });
	},
	getSpaceRules(slug: string) {
		return request<Array<{ id: string; title: string; description: string; position: number }>>('/spaces/' + slug + '/rules');
	},
	createSpaceRule(slug: string, data: { title: string; description: string; position?: number }) {
		return request<{ id: string }>('/spaces/' + slug + '/rules', { method: 'POST', body: JSON.stringify(data) });
	},
	updateSpaceRule(slug: string, ruleId: string, data: { title?: string; description?: string; position?: number }) {
		return request<{ id: string }>('/spaces/' + slug + '/rules/' + ruleId, { method: 'PATCH', body: JSON.stringify(data) });
	},
	deleteSpaceRule(slug: string, ruleId: string) {
		return request<{ status: string }>('/spaces/' + slug + '/rules/' + ruleId, { method: 'DELETE' });
	},
	getSpaceModerators(slug: string) {
		return request<Array<{ user_id: string; username: string; display_name?: string }>>('/spaces/' + slug + '/moderators');
	},
	addSpaceModerator(slug: string, userId: string) {
		return request<{ added: boolean }>('/spaces/' + slug + '/moderators', { method: 'POST', body: JSON.stringify({ user_id: userId }) });
	},
	removeSpaceModerator(slug: string, userId: string) {
		return request<{ removed: boolean }>('/spaces/' + slug + '/moderators/' + userId, { method: 'DELETE' });
	},
	getSpaceParents(slug: string) {
		return request<Array<{ id: string; name: string; slug: string }>>('/spaces/' + slug + '/parents');
	},
	addSpaceParent(slug: string, parentId: string) {
		return request<{ added: boolean }>('/spaces/' + slug + '/parents', { method: 'POST', body: JSON.stringify({ parent_id: parentId }) });
	},
	removeSpaceParent(slug: string, parentId: string) {
		return request<{ removed: boolean }>('/spaces/' + slug + '/parents/' + parentId, { method: 'DELETE' });
	},
	suggestSpaceParents(query: string) {
		return request<Array<{ id: string; name: string; slug: string }>>('/spaces/suggest-parents?q=' + encodeURIComponent(query));
	},
	getSpaceAnalytics(id: string) {
		return request<{ member_count: number; content_count: number; views: number; growth: number }>('/spaces/' + id + '/analytics');
	},
	getSpaceMembers(slug: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<Array<{ user_id: string; username: string; display_name?: string; role: string; joined_at: string }>>('/spaces/' + slug + '/members' + (q ? '?' + q : ''));
	},

	// ── Content mutations (missing methods) ─────────────────────────────────
	publishContent(id: string) {
		return request<{ status: string }>('/content/' + id + '/publish', { method: 'POST' });
	},
	archiveContent(id: string) {
		return request<{ status: string }>('/content/' + id + '/archive', { method: 'POST' });
	},
	scheduleContent(id: string, data: { scheduled_at: string }) {
		return request<{ status: string }>('/content/' + id + '/schedule', { method: 'POST', body: JSON.stringify(data) });
	},
	viewContent(id: string) {
		return request<{ viewed: boolean }>('/content/' + id + '/view', { method: 'POST' });
	},
	getShareCount(id: string) {
		return request<{ count: number }>('/content/' + id + '/share-count');
	},
	reportContent(id: string, data: { reason: string }) {
		return request<{ reported: boolean }>('/content/' + id + '/report', { method: 'POST', body: JSON.stringify(data) });
	},
	bookmarkContent(id: string) {
		return request<{ bookmarked: boolean }>('/content/' + id + '/bookmark', { method: 'POST' });
	},
	unbookmarkContent(id: string) {
		return request<{ bookmarked: boolean }>('/content/' + id + '/bookmark', { method: 'DELETE' });
	},
	getBookmarkStatus(id: string) {
		return request<{ bookmarked: boolean }>('/content/' + id + '/bookmark');
	},
	getContentTags(id: string) {
		return request<Array<{ id: string; name: string; slug: string }>>('/content/' + id + '/tags');
	},
	addContentTag(id: string, tagId: string) {
		return request<{ added: boolean }>('/content/' + id + '/tags', { method: 'POST', body: JSON.stringify({ tag_id: tagId }) });
	},
	removeContentTag(id: string, tagId: string) {
		return request<{ removed: boolean }>('/content/' + id + '/tags/' + tagId, { method: 'DELETE' });
	},
	shareContent(id: string, data?: { platform?: string }) {
		return request<{ share_url: string }>('/content/' + id + '/share', { method: 'POST', body: JSON.stringify(data ?? {}) });
	},
	getRelatedContent(id: string) {
		return request<FeedItem[]>('/content/' + id + '/related');
	},
	getContentAnalytics(id: string) {
		return request<{ views: number; shares: number; bookmarks: number; comments: number }>('/content/' + id + '/analytics');
	},
	getContentDiscussions(id: string) {
		return request<Array<{ id: string; title: string; reply_count: number }>>('/content/' + id + '/discussions');
	},
	createContentDiscussion(id: string, data: { title: string; body: string }) {
		return request<{ id: string }>('/content/' + id + '/discussions', { method: 'POST', body: JSON.stringify(data) });
	},
	getContentComments(id: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<Array<{ id: string; body: string; author_id: string; author_name: string; created_at: string }>>('/content/' + id + '/comments' + (q ? '?' + q : ''));
	},
	createContentComment(id: string, data: { body: string }) {
		return request<{ id: string }>('/content/' + id + '/comments', { method: 'POST', body: JSON.stringify(data) });
	},

	// ── Tags (missing methods) ──────────────────────────────────────────────
	getTag(slug: string) {
		return request<{ id: string; name: string; slug: string; description?: string; post_count: number }>('/tags/' + slug);
	},
	getTagContent(slug: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/tags/' + slug + '/content' + (q ? '?' + q : ''));
	},
	followTag(slug: string) {
		return request<{ following: boolean }>('/tags/' + slug + '/follow', { method: 'POST' });
	},
	unfollowTag(slug: string) {
		return request<{ following: boolean }>('/tags/' + slug + '/follow', { method: 'DELETE' });
	},
	getTagFollowers(slug: string) {
		return request<Array<{ id: string; username: string; display_name?: string }>>('/tags/' + slug + '/followers');
	},
	searchTags(slug: string, query: string) {
		return request<Array<{ id: string; name: string; slug: string }>>('/tags/' + slug + '/search?q=' + encodeURIComponent(query));
	},

	// ── Media (missing methods) ─────────────────────────────────────────────
	getMedia(id: string) {
		return request<MediaItem>('/media/' + id);
	},
	updateMedia(id: string, data: { alt_text?: string; caption?: string }) {
		return request<MediaItem>('/media/' + id, { method: 'PATCH', body: JSON.stringify(data) });
	},
	confirmMedia(id: string) {
		return request<{ confirmed: boolean }>('/media/confirm/' + id, { method: 'POST' });
	},
	bulkDeleteMedia(ids: string[]) {
		return request<{ deleted: number }>('/media/bulk-delete', { method: 'POST', body: JSON.stringify({ ids }) });
	},

	// ── Tickets (missing attachment methods) ────────────────────────────────
	uploadTicketAttachment(ticketId: string, file: File) {
		const formData = new FormData();
		formData.append('file', file);
		const headers: Record<string, string> = {};
		if (jwtToken) headers['Authorization'] = `Bearer ${jwtToken}`;
		return fetch(`${API_BASE}/me/tickets/${ticketId}/attachments`, {
			method: 'POST',
			headers,
			body: formData
		}).then(async (res) => {
			const body = (await res.json()) as ApiOk<{ id: string; filename: string; url: string }> | ApiErr;
			if (!res.ok || 'error' in body) {
				const err = 'error' in body ? body.error : { code: 'unknown', message: res.statusText };
				throw new Error(`${err.code}: ${err.message}`);
			}
			return (body as ApiOk<{ id: string; filename: string; url: string }>).data;
		});
	},
	listTicketAttachments(ticketId: string) {
		return request<Array<{ id: string; filename: string; url: string; size: number; created_at: string }>>('/me/tickets/' + ticketId + '/attachments');
	},

	// ── Forum (missing methods) ─────────────────────────────────────────────
	deleteForumVote(id: string) {
		return request<{ status: string }>('/forum/' + id + '/vote', { method: 'DELETE' });
	},
	getForumTagBySlug(slug: string) {
		return request<{ id: string; name: string; slug: string; thread_count: number }>('/forum/tags/' + slug);
	},
	addForumTagToThread(threadId: string, tagId: string) {
		return request<{ added: boolean }>('/forum/' + threadId + '/tags/' + tagId, { method: 'POST' });
	},
	removeForumTagFromThread(threadId: string, tagId: string) {
		return request<{ removed: boolean }>('/forum/' + threadId + '/tags/' + tagId, { method: 'DELETE' });
	},
	updateForumTag(id: string, data: { name?: string; slug?: string }) {
		return request<{ id: string }>('/forum/tags/' + id, { method: 'PATCH', body: JSON.stringify(data) });
	},
	deleteForumTag(id: string) {
		return request<{ status: string }>('/forum/tags/' + id, { method: 'DELETE' });
	},
	unsolveForumThread(id: string) {
		return request<{ status: string }>('/forum/' + id + '/solve', { method: 'DELETE' });
	},

	// ── Blog (missing methods) ──────────────────────────────────────────────
	getBlogSeriesList() {
		return request<Array<{ id: string; title: string; slug: string; description?: string; post_count: number; created_at: string }>>('/blog/series');
	},
	getBlogSeries(slug: string) {
		return request<{ id: string; title: string; slug: string; description?: string; posts: Array<{ id: string; title: string; slug: string; position: number }> }>('/blog/series/' + slug);
	},
	getFeaturedPosts() {
		return request<Array<{ content_id: string; title: string; slug: string; featured_order: number; featured_until?: string }>>('/blog/featured');
	},

	// ── AI (missing methods) ────────────────────────────────────────────────
	aiGenerate(data: { prompt: string; type: string }) {
		return request<{ result: string }>('/ai/generate', { method: 'POST', body: JSON.stringify(data) });
	},
	aiHitlSubmit(data: { content: string; type: string; metadata?: Record<string, unknown> }) {
		return request<{ id: string }>('/ai/hitl', { method: 'POST', body: JSON.stringify(data) });
	},
	aiHitlPending() {
		return request<Array<{ id: string; content: string; type: string; status: string; created_at: string }>>('/ai/hitl/pending');
	},
	aiHitlApprove(id: string) {
		return request<{ approved: boolean }>('/ai/hitl/' + id + '/approve', { method: 'POST' });
	},
	aiHitlReject(id: string, data?: { reason?: string }) {
		return request<{ rejected: boolean }>('/ai/hitl/' + id + '/reject', { method: 'POST', body: JSON.stringify(data ?? {}) });
	},
	getAIConversations() {
		return request<Array<{ id: string; title: string; created_at: string; updated_at: string }>>('/ai/conversations');
	},
	getAIConversation(id: string) {
		return request<{ id: string; title: string; messages: Array<{ role: string; content: string; created_at: string }> }>('/ai/conversations/' + id);
	},

	// ── Reviews (missing methods) ───────────────────────────────────────────
	getReviewById(id: string) {
		return request<Review>('/reviews/' + id);
	},

	// ── Comparisons (missing methods) ───────────────────────────────────────
	getComparisonBySlug(slug: string) {
		return request<{ id: string; title: string; slug: string; type_id: string; item_count: number; vote_count: number; created_at: string }>('/comparisons/' + slug);
	},

	// ── Subscriptions (missing methods) ─────────────────────────────────────
	getSubscriptions() {
		return request<Array<{ id: string; plan_id: string; status: string; current_period_end: string }>>('/subscriptions');
	},
	deleteSubscription(id: string) {
		return request<{ deleted: boolean }>('/subscriptions/' + id, { method: 'DELETE' });
	},

	// ── Follow (missing methods) ────────────────────────────────────────────
	getFollowStatus(userId: string) {
		return request<{ following: boolean }>('/users/' + userId + '/follow-status');
	},
	getSpaceFollowers(spaceId: string) {
		return request<Array<{ user_id: string; username: string; display_name?: string }>>('/spaces/' + spaceId + '/followers');
	},

	// ── User blocks & mutes (missing methods) ───────────────────────────────
	blockUser(userId: string) {
		return request<{ blocked: boolean }>('/users/' + userId + '/block', { method: 'POST' });
	},
	unblockUser(userId: string) {
		return request<{ blocked: boolean }>('/users/' + userId + '/block', { method: 'DELETE' });
	},
	muteUser(userId: string) {
		return request<{ muted: boolean }>('/users/' + userId + '/mute', { method: 'POST' });
	},
	unmuteUser(userId: string) {
		return request<{ muted: boolean }>('/users/' + userId + '/mute', { method: 'DELETE' });
	},
	getUserMuteStatus(userId: string) {
		return request<{ muted: boolean }>('/users/' + userId + '/mute-status');
	},
	reportUser(userId: string, data: { reason: string }) {
		return request<{ reported: boolean }>('/users/' + userId + '/report', { method: 'POST', body: JSON.stringify(data) });
	},

	// ── Users (missing methods) ─────────────────────────────────────────────
	getUserContent(userId: string, params?: { page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/users/' + userId + '/content' + (q ? '?' + q : ''));
	},
	getUserAnalytics(userId: string) {
		return request<{ views: number; followers: number; content_count: number }>('/users/' + userId + '/analytics');
	},
	unfollowUser(userId: string) {
		return request<{ followed: boolean }>('/users/' + userId + '/follow', { method: 'DELETE' });
	},

	// ── Search (missing methods) ────────────────────────────────────────────
	autocompleteSearch(query: string) {
		return request<Array<{ type: string; id: string; title: string; subtitle?: string }>>('/search/autocomplete?q=' + encodeURIComponent(query));
	},

	// ── Geo (missing methods) ───────────────────────────────────────────────
	geoDetect() {
		return request<{ country: string; country_code: string; region?: string; city?: string }>('/geo/detect');
	},
	geoStates(countryCode: string) {
		return request<Array<{ id: string; name: string; code: string }>>('/geo/countries/' + countryCode + '/states');
	},
	geoCities(stateId: string) {
		return request<Array<{ id: string; name: string }>>('/geo/states/' + stateId + '/cities');
	},

	// ── I18n (missing methods) ──────────────────────────────────────────────
	getI18nPacks() {
		return request<Array<{ lang_code: string; name: string; loaded: boolean }>>('/i18n/packs');
	},
	getI18nPack(langCode: string) {
		return request<Record<string, string>>('/i18n/packs/' + langCode);
	},

	// ── Push (missing methods) ──────────────────────────────────────────────
	deletePushToken(id: string) {
		return request<{ status: string }>('/me/push-tokens/' + id, { method: 'DELETE' });
	},
	deleteAllPushTokens() {
		return request<{ status: string }>('/me/push-tokens', { method: 'DELETE' });
	},

	// ── Notifications (missing methods) ─────────────────────────────────────
	deleteNotification(id: string) {
		return request<{ status: string }>('/me/notifications/' + id, { method: 'DELETE' });
	},

	// ── Content reactions (missing methods) ─────────────────────────────────
	reactToContent(id: string, data: { key: string }) {
		return request<{ id: string }>('/content/' + id + '/react', { method: 'POST', body: JSON.stringify(data) });
	},
	unreactToContent(id: string) {
		return request<{ status: string }>('/content/' + id + '/react', { method: 'DELETE' });
	},
	updateContentSpaces(id: string, data: { space_ids: string[] }) {
		return request<{ updated: boolean }>('/content/' + id + '/spaces', { method: 'PATCH', body: JSON.stringify(data) });
	},

	// ── Content by slug ───────────────────────────────────────────────────
	getContentBySlug(slug: string) {
		return request<FeedItem>('/content/' + slug);
	},

	// ── Tag content search ────────────────────────────────────────────────
	searchTagContent(slug: string, params?: { q?: string; page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.q) p.set('q', params.q);
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/tags/' + slug + '/search' + (q ? '?' + q : ''));
	},

	// ── Spaces search ─────────────────────────────────────────────────────
	searchSpaces(params?: { q?: string; page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.q) p.set('q', params.q);
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<Array<{ id: string; name: string; slug: string; description: string }>>(
			'/spaces/search' + (q ? '?' + q : '')
		);
	},

	// ── Space content search ──────────────────────────────────────────────
	searchSpaceContent(spaceId: string, params?: { q?: string; page_size?: number; page_offset?: number }) {
		const p = new URLSearchParams();
		if (params?.q) p.set('q', params.q);
		if (params?.page_size) p.set('page_size', String(params.page_size));
		if (params?.page_offset) p.set('page_offset', String(params.page_offset));
		const q = p.toString();
		return request<FeedItem[]>('/spaces/' + spaceId + '/search' + (q ? '?' + q : ''));
	},

	// ── University ────────────────────────────────────────────────────────
	getUniversity(slug: string) {
		return request<{
			id: string;
			name: string;
			slug: string;
			description?: string;
			country?: string;
			logo_url?: string;
		}>('/universities/' + slug);
	},

	// ── Suggested content ─────────────────────────────────────────────────
	getSuggestedContent() {
		return request<FeedItem[]>('/suggestions/content');
	},

	// ── My media ──────────────────────────────────────────────────────────
	getMyMedia() {
		return request<MediaItem[]>('/me/media');
	},

	// ── My enrollments ────────────────────────────────────────────────────
	getMyEnrollments() {
		return request<Array<{
			id: string;
			track_id: string;
			track_title: string;
			progress_percent: number;
			enrolled_at: string;
		}>>('/me/enrollments');
	},

	// ── Tag feed (RSS/Atom) ───────────────────────────────────────────────
	getTagFeed(slug: string) {
		return request<string>('/tags/' + slug + '/feed.xml');
	},

	// ── User feed (RSS/Atom) ──────────────────────────────────────────────
	getUserFeed(username: string) {
		return request<string>('/users/' + username + '/feed.xml');
	},

	// ── Content share count ───────────────────────────────────────────────
	getContentShareCount(id: string) {
		return request<{ count: number }>('/content/' + id + '/share-count');
	},
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

export interface ProfileMember {
	user_id: string;
	username: string;
	display_name: string;
	role: string;
	joined_at: string;
}

export interface ProfileInvitation {
	id: string;
	email: string;
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

// Review types
export interface Review {
	id: string;
	entity_type: string;
	entity_id: string;
	author_id: string;
	author_name: string;
	author_avatar?: string;
	rating: number;
	title?: string;
	body?: string;
	flagged: boolean;
	created_at: string;
	updated_at: string;
}

// Follow types
export interface FollowUser {
	id: string;
	username: string;
	display_name: string;
	avatar_url?: string;
	mutual?: boolean;
	following?: boolean;
}

// Content Reaction types
export interface ReactionType {
	key: string;
	label: string;
	emoji: string;
}

export interface ContentReaction {
	id: string;
	reaction_type: ReactionType;
	user_id: string;
	created_at: string;
}

// Consent types
export interface ConsentCategory {
	category: string;
	granted: boolean;
	granted_at?: string;
}

// Content Revision types
export interface ContentRevision {
	id: string;
	version: number;
	created_at: string;
	author_name: string;
	summary?: string;
}

// Payment types
export interface PaymentDetail {
	id: string;
	amount: number;
	currency: string;
	status: string;
	provider: string;
	reference: string;
	purpose: string;
	created_at: string;
	updated_at: string;
	refundable: boolean;
	refunded_at?: string;
}

export interface PaymentListItem {
	id: string;
	amount: number;
	currency: string;
	status: string;
	description: string;
	provider: string;
	created_at: string;
	refundable?: boolean;
	refunded_at?: string;
}

export interface SubscriptionDetail {
	id: string;
	plan: string;
	status: string;
	current_period_end: string;
	cancel_at: string | null;
}

// Wallet types
export interface WalletLedgerEntry {
	id: string;
	amount: number;
	balance_after: number;
	type: string;
	description: string;
	created_at: string;
}

// Search types
export interface SearchResult {
	id: string;
	title: string;
	type: string;
	excerpt: string;
}

// User types
export interface UserProfile {
	id: string;
	username: string;
	display_name: string;
	avatar_url: string | null;
	bio: string | null;
	location: string | null;
	following?: boolean;
	follower_count?: number;
	following_count?: number;
	content_count?: number;
}

// Comparison types
export interface ComparisonType {
	id: string;
	name: string;
	slug: string;
	description: string;
	comparison_count: number;
}

// Media types
export interface MediaItem {
	id: string;
	filename: string;
	url: string;
	mime_type: string;
	file_size: number;
	alt_text: string;
	created_at: string;
}

// Analytics types
export interface DashboardData {
	top_content: Array<{
		content_id: string;
		total_views: number;
		total_reactions: number;
		total_comments: number;
		total_unique_viewers: number;
	}>;
	user_engagement: Record<string, unknown>[];
	content_over_time: Record<string, unknown>[];
}

// Link Preview types
export interface LinkPreviewData {
	title: string;
	description: string;
	image_url: string;
	favicon_url: string;
	site_name: string;
}

// Ad types
export interface AdRender {
	id: string;
	title: string;
	image_url: string;
	click_url: string;
	provider: string;
}

// Feed types
export interface FeedItem {
	id: string;
	title: string;
	slug?: string;
	excerpt?: string;
	author_id: string;
	author_name: string;
	author_avatar_url?: string;
	space_name?: string;
	content_type: string;
	vote_score: number;
	comment_count: number;
	bookmark_count?: number;
	view_count?: number;
	created_at: string;
	reading_time_minutes?: number;
	cover_image_url?: string;
	tags?: string[];
}

// Webhook types
export interface Webhook {
	id: string;
	url: string;
	events: string[];
	secret?: string;
	status: string;
	created_at: string;
}

export interface WebhookDelivery {
	id: string;
	webhook_id: string;
	event_type: string;
	status_code: number;
	delivered_at: string;
	response_body?: string;
}
