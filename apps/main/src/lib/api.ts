export const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080/api/v1';

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
		return request<{ sent: boolean }>('/auth/forgot-password', {
			method: 'POST',
			body: JSON.stringify({ email })
		});
	},
	resetPassword(token: string, password: string) {
		return request<{ success: boolean }>('/auth/reset-password', {
			method: 'POST',
			body: JSON.stringify({ token, password })
		});
	},
	verifyEmail(token: string) {
		return request<{ verified: boolean }>('/auth/verify-email?token=' + encodeURIComponent(token));
	},
	resendVerification(email: string) {
		return request<{ sent: boolean }>('/auth/resend-verification', {
			method: 'POST',
			body: JSON.stringify({ email })
		});
	},
	setupTOTP() {
		return request<{ secret: string; qr_uri: string }>('/me/totp/enable', { method: 'POST' });
	},
	verifyTOTP(code: string) {
		return request<{ verified: boolean; backup_codes: string[] }>('/me/totp/confirm', {
			method: 'POST',
			body: JSON.stringify({ code })
		});
	},
	disableTOTP(password: string) {
		return request<{ disabled: boolean }>('/me/totp', {
			method: 'DELETE',
			body: JSON.stringify({ password })
		});
	},

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
		}>('/forum' + (q ? '?' + q : ''));
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
		return request<{ deleted: boolean }>('/forum/' + id, { method: 'DELETE' });
	},
	solveForumThread(id: string) {
		return request<{ solved: boolean }>('/forum/' + id + '/solve', { method: 'POST' });
	},
	getForumReplies(threadId: string, params?: { limit?: number; offset?: number }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		const q = p.toString();
		return request<{
			replies: ForumReply[];
			total: number;
		}>('/forum/' + threadId + '/replies' + (q ? '?' + q : ''));
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
		return request<Ticket>('/tickets', { method: 'POST', body: JSON.stringify(data) });
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
		return request<{ success: boolean }>('/dm/requests/' + conversationId + '/accept', {
			method: 'POST'
		});
	},
	declineDMRequest(conversationId: string) {
		return request<{ success: boolean }>('/dm/requests/' + conversationId + '/decline', {
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
		}>('/profiles' + (q ? '?' + q : ''));
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
		return request<{ deleted: boolean }>('/profiles/' + id, { method: 'DELETE' });
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
		return request<{ deleted: boolean }>('/profiles/relocations/' + relocationId, {
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
		return request<{ deleted: boolean }>('/profiles/documents/' + documentId, {
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
		return request<{ deleted: boolean }>('/me/api-keys/' + id, { method: 'DELETE' });
	},
	updateDeveloperKey(id: string, data: { name?: string; scopes?: string[] }) {
		return request<DeveloperApiKey>('/me/api-keys/' + id, {
			method: 'PUT',
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
	listReviews(params: { entity_type: string; entity_id: string; limit?: number; offset?: number }) {
		const p = new URLSearchParams();
		if (params.limit) p.set('limit', String(params.limit));
		if (params.offset) p.set('offset', String(params.offset));
		const q = p.toString();
		return request<{ reviews: Review[]; total: number }>(
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
		return request<{ deleted: boolean }>('/reviews/' + id, { method: 'DELETE' });
	},
	flagReview(id: string, data: { reason: string }) {
		return request<{ flagged: boolean }>('/reviews/' + id + '/flag', { method: 'POST', body: JSON.stringify(data) });
	},

	// Space membership
	joinSpace(slug: string) {
		return request<{ joined: boolean; member_count: number }>('/spaces/' + slug + '/join', { method: 'POST' });
	},

	// Followers / Following
	getFollowers(userId: string, params?: { limit?: number; offset?: number }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		const q = p.toString();
		return request<{ followers: FollowUser[]; total: number }>('/users/' + userId + '/followers' + (q ? '?' + q : ''));
	},
	getFollowing(userId: string, params?: { limit?: number; offset?: number }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		const q = p.toString();
		return request<{ following: FollowUser[]; total: number }>('/users/' + userId + '/following' + (q ? '?' + q : ''));
	},
	checkMutualFollow(userId: string, otherUserId: string) {
		return request<{ mutual: boolean }>('/users/' + userId + '/mutual/' + otherUserId);
	},

	// Content Reactions
	addReaction(contentId: string, reactionTypeId: string) {
		return request<{ id: string }>('/content/' + contentId + '/reactions', {
			method: 'POST', body: JSON.stringify({ reaction_type_id: reactionTypeId })
		});
	},
	removeReaction(contentId: string, reactionId: string) {
		return request<{ removed: boolean }>('/content/' + contentId + '/reactions/' + reactionId, { method: 'DELETE' });
	},
	getReactions(contentId: string) {
		return request<Array<{ id: string; reaction_type: { id: string; name: string; emoji: string }; user_id: string; created_at: string }>>('/content/' + contentId + '/reactions');
	},
	listReactionTypes() {
		return request<Array<{ id: string; name: string; emoji: string }>>('/reaction-types');
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
		return request<{ revoked: boolean }>('/consent/' + consentType, { method: 'DELETE' });
	},
	requestDataExport() {
		return request<{ request_id: string }>('/me/data-export');
	},
	requestAccountDeletion() {
		return request<{ request_id: string }>('/me', { method: 'DELETE' });
	},

	// Content Pinning
	pinContent(id: string) {
		return request<{ pinned: boolean }>('/content/' + id + '/pin', { method: 'POST' });
	},
	unpinContent(id: string) {
		return request<{ pinned: boolean }>('/content/' + id + '/unpin', { method: 'POST' });
	},

	// Content Revisions
	getRevisions(contentId: string) {
		return request<Array<{ id: string; version: number; created_at: string; author_name: string; summary?: string }>>('/content/' + contentId + '/revisions');
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
		return request<PaymentDetail>('/payments/' + id);
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
		return request<{ users: Array<{ user_id: string; username: string; last_seen: string }> }>('/ws/online');
	},

	// Wallet
	getWalletLedger(params?: { limit?: number }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
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
	listPayments(params?: { limit?: number }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
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
	getComparisonTypes(params?: { limit?: number }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		const q = p.toString();
		return request<ComparisonType[]>('/comparisons/types' + (q ? '?' + q : ''));
	},

	// Media
	listMedia(params?: { limit?: number }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
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
		return request<{ deleted: boolean }>('/media/' + id, { method: 'DELETE' });
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
		return request<{ token: string; user: UserProfile }>('/auth/login', {
			method: 'POST',
			body: JSON.stringify({ email, password })
		});
	},
	register(data: { username: string; email: string; password: string }) {
		return request<{ success: boolean }>('/auth/register', {
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
	getFeedPrimary(params?: { limit?: number; cursor?: string }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.cursor) p.set('cursor', params.cursor);
		const q = p.toString();
		return request<{ items: FeedItem[]; has_more: boolean }>('/feed' + (q ? '?' + q : ''));
	},
	getSpaceFeed(spaceId: string, params?: { limit?: number; offset?: number; cursor?: string }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		if (params?.cursor) p.set('cursor', params.cursor);
		const q = p.toString();
		return request<{ items: FeedItem[]; has_more: boolean }>('/feed/space/' + spaceId + (q ? '?' + q : ''));
	},
	getProfileFeed(userId: string, params?: { limit?: number; offset?: number; cursor?: string }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		if (params?.cursor) p.set('cursor', params.cursor);
		const q = p.toString();
		return request<{ items: FeedItem[]; has_more: boolean }>('/feed/profile/' + userId + (q ? '?' + q : ''));
	},
	getExploreFeed(params?: { limit?: number; offset?: number }) {
		const p = new URLSearchParams();
		if (params?.limit) p.set('limit', String(params.limit));
		if (params?.offset) p.set('offset', String(params.offset));
		const q = p.toString();
		return request<{ items: FeedItem[]; has_more: boolean }>('/explore' + (q ? '?' + q : ''));
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
		return request<Array<{ type: string; email: boolean; push: boolean; in_app: boolean }>>('/notification-preferences');
	},
	updateNotificationPreference(type: string, data: { email?: boolean; push?: boolean; in_app?: boolean }) {
		return request<{ type: string }>('/notification-preferences/' + type, {
			method: 'PUT', body: JSON.stringify(data)
		});
	},
	getNotificationTypes() {
		return request<Array<{ key: string; label: string; description: string }>>('/notification-types');
	},

	// Webhooks
	listWebhooks() {
		return request<Webhook[]>('/webhooks');
	},
	createWebhook(data: { url: string; events: string[]; secret?: string }) {
		return request<Webhook>('/webhooks', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	deleteWebhook(id: string) {
		return request<{ deleted: boolean }>('/webhooks/' + id, { method: 'DELETE' });
	},
	getWebhookDeliveries(id: string) {
		return request<WebhookDelivery[]>('/webhooks/' + id + '/deliveries');
	},

	// ── Trending (AUDIT-FE wiring) ────────────────────────────────────────────
	getTrendingContent(limit = 10) {
		return request<{ items: FeedItem[] }>(`/trending/content?limit=${limit}`);
	},
	getTrendingSpaces(limit = 10) {
		return request<{ items: unknown[] }>(`/trending/spaces?limit=${limit}`);
	},
	getTrendingTags(limit = 10) {
		return request<{ items: unknown[] }>(`/trending/tags?limit=${limit}`);
	},
	getTrendingUsers(limit = 10) {
		return request<{ items: unknown[] }>(`/trending/users?limit=${limit}`);
	},

	// ── Suggestions / Discovery (AUDIT-FE wiring) ─────────────────────────────
	getSuggestedSpaces() {
		return request<{ items: unknown[] }>('/suggestions/spaces');
	},
	getSuggestedUsers() {
		return request<{ items: unknown[] }>('/suggestions/users');
	},
	getSuggestedTags() {
		return request<{ items: unknown[] }>('/suggestions/tags');
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

	// ── Custom Fields (AUDIT-FE wiring) ──────────────────────────────────────
	getCustomFieldGroups(entityType: string) {
		return request<{ groups: CustomFieldGroup[] }>(`/custom-field-groups?entity_type=${entityType}`);
	},
	getCustomFieldGroupFields(groupId: string) {
		return request<{ fields: CustomField[] }>(`/custom-field-groups/${groupId}/fields`);
	},
	getCustomFieldValues(entityType: string, entityId: string) {
		return request<{ values: Record<string, unknown> }>(
			`/custom-field-values/${entityType}/${entityId}`
		);
	},
	setCustomFieldValues(entityType: string, entityId: string, values: Record<string, unknown>) {
		return request<{ saved: boolean }>('/custom-field-values', {
			method: 'POST',
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
		return request<{ deleted: boolean }>(`/comments/${id}`, { method: 'DELETE' });
	},
	voteComment(id: string, value: 1 | -1) {
		return request<{ score: number }>(`/comments/${id}/vote`, {
			method: 'POST',
			body: JSON.stringify({ value })
		});
	},
	removeCommentVote(id: string) {
		return request<{ score: number }>(`/comments/${id}/vote`, { method: 'DELETE' });
	},
	pinComment(id: string) {
		return request<{ pinned: boolean }>(`/comments/${id}/pin`, { method: 'POST' });
	},
	unpinComment(id: string) {
		return request<{ pinned: boolean }>(`/comments/${id}/pin`, { method: 'DELETE' });
	},

	// ── Organizations (AUDIT-FE closing gap) ──────────────────────────────────
	listOrganizations(limit = 50, page = 1) {
		return request<{ organizations: Organization[]; total: number; total_pages: number }>(
			`/organizations?limit=${limit}&page=${page}`
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
		return request<{ deleted: boolean }>(`/organizations/${id}`, { method: 'DELETE' });
	},
	getOrganizationMembers(id: string) {
		return request<{ members: OrganizationMember[] }>(`/organizations/${id}/members`);
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
		return request<{ removed: boolean }>(`/organizations/${id}/members/${userId}`, {
			method: 'DELETE'
		});
	},
	listOrganizationInvitations(id: string) {
		return request<{ invitations: OrganizationInvitation[] }>(`/organizations/${id}/invitations`);
	},
	inviteToOrganization(id: string, email: string, role: string) {
		return request<{ invited: boolean }>(`/organizations/${id}/invitations`, {
			method: 'POST',
			body: JSON.stringify({ email, role })
		});
	},
	acceptOrganizationInvitation(token: string) {
		return request<{ accepted: boolean; organization_slug: string }>(
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
	getCoupon(id: string) {
		return request<Coupon>(`/coupons/${id}`);
	},
	trackCouponClick(id: string) {
		return request<{ tracked: boolean }>(`/coupons/${id}/click`, { method: 'POST' });
	},

	// ── Affiliate program (AUDIT-FE closing gap) ──────────────────────────────
	enrollAffiliate() {
		return request<{ enrolled: boolean }>('/affiliate-program/enroll', { method: 'POST' });
	},
	getAffiliateLinks() {
		return request<{ links: AffiliateLink[] }>('/me/affiliate-links');
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
		return request<{ deleted: boolean }>(`/me/affiliate-links/${id}`, { method: 'DELETE' });
	},
	getAffiliateEarnings() {
		return request<{ total_credits: number; conversions: AffiliateConversion[] }>(
			'/me/affiliate-earnings'
		);
	},

	// ── Geo (AUDIT-FE closing gap — location resolution for onboarding/spaces) ─
	geoMe() {
		return request<{ country: string; country_code: string; region?: string; city?: string }>(
			'/geo/me'
		);
	},
	geoLookup(query: string) {
		return request<{ results: GeoLocation[] }>(`/geo/lookup?q=${encodeURIComponent(query)}`);
	},

	// ── Space types & permission presets (AUDIT-FE closing gap) ───────────────
	listSpaceTypes() {
		return request<{ space_types: SpaceType[] }>('/space-types');
	},
	getSpaceType(id: string) {
		return request<SpaceType>(`/space-types/${id}`);
	},
	listSpacePermissionPresets() {
		return request<{ presets: SpacePermissionPreset[] }>('/space-permission-presets');
	},
	getSpacePermissionPreset(name: string) {
		return request<SpacePermissionPreset>(`/space-permission-presets/${name}`);
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
	id: string;
	name: string;
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
	author_avatar?: string;
	space_name?: string;
	content_type: string;
	reaction_count: number;
	comment_count: number;
	created_at: string;
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
