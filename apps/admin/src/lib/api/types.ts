export interface User {
	id: string;
	username: string;
	display_name: string;
	email: string;
	avatar_url: string | null;
	user_group_id: string;
	user_group_name: string;
	theme_preference: 'light' | 'dark' | 'system';
	timezone: string;
	is_verified: boolean;
	created_at: string;
	permissions?: string[];
}

export interface AdminUser {
	id: string;
	username: string;
	display_name: string;
	email: string;
	avatar_url: string | null;
	user_group_id: string;
	user_group_name: string;
	status: string;
	is_verified: boolean;
	created_at: string;
	last_seen_at: string | null;
	content_count: number;
	follower_count: number;
}

export interface UserWarning {
	id: string;
	reason: string;
	issued_by: string;
	issued_at: string;
}

export interface UserBan {
	id: string;
	reason: string;
	ban_type: 'temporary' | 'permanent';
	issued_by: string;
	issued_at: string;
	expires_at: string | null;
}

export interface ModerationQueueItem {
	id: string;
	entity_type: string;
	entity_id: string;
	entity_preview: string;
	report_count: number;
	reporter: { id: string; username: string } | null;
	is_ai_flag: boolean;
	priority: string;
	status: string;
	assigned_moderator: { id: string; username: string } | null;
	created_at: string;
}

export interface Report {
	id: string;
	entity_type: string;
	entity_id: string;
	reporter: { id: string; username: string; avatar_url: string | null };
	reason: string;
	notes: string;
	status: string;
	created_at: string;
}

export interface Appeal {
	id: string;
	user: { id: string; username: string; avatar_url: string | null };
	subject_type: string;
	subject_id: string;
	reason: string;
	status: string;
	resolution_note: string | null;
	created_at: string;
}

export interface Space {
	id: string;
	slug: string;
	name: string;
	description: string;
	logo_url: string | null;
	space_type: string;
	level: string;
	status: string;
	member_count: number;
	created_at: string;
}

export interface Tag {
	id: string;
	slug: string;
	name: string;
	description: string | null;
	is_system: boolean;
	follower_count: number;
	content_count: number;
	created_at: string;
}

export interface Organization {
	id: string;
	slug: string;
	name: string;
	logo_url: string | null;
	is_verified: boolean;
	member_count: number;
	created_at: string;
}

export interface Ticket {
	id: string;
	subject: string;
	status: string;
	priority: string;
	type_id: string;
	assigned_to: { id: string; username: string } | null;
	created_by: { id: string; username: string };
	sla_deadline: string | null;
	created_at: string;
}

export interface NotificationType {
	id: string;
	key: string;
	label: string;
	description: string;
	channels: { in_app: boolean; email: boolean; push: boolean };
	enabled: boolean;
}

export interface Broadcast {
	id: string;
	title: string;
	body: string;
	target_scope: string;
	status: string;
	send_at: string | null;
	sent_at: string | null;
	created_at: string;
}

export interface AdCampaign {
	id: string;
	title: string;
	status: string;
	impressions: number;
	clicks: number;
	spend: number;
	created_at: string;
}

export interface Payment {
	id: string;
	user: { id: string; username: string };
	amount: number;
	currency: string;
	purpose_type: string;
	status: string;
	created_at: string;
}

export interface MediaItem {
	id: string;
	type: string;
	url: string;
	uploader: { id: string; username: string } | null;
	flagged: boolean;
	confirmed: boolean;
	created_at: string;
}

export interface Webhook {
	id: string;
	name: string;
	url: string;
	event_types: string[];
	status: string;
	created_at: string;
}

export interface AnalyticsOverview {
	mau: number;
	dau: number;
	new_users: number;
	content_published: number;
	active_subscriptions: number;
	mrr: number;
	moderation_queue_depth: number;
}

export interface AuditLogEntry {
	id: string;
	timestamp: string;
	actor: { id: string; username: string } | null;
	action_type: string;
	entity_type: string;
	entity_id: string;
	ip_address: string;
	impersonated_user_id: string | null;
	metadata: Record<string, unknown>;
}

export interface PaginatedResponse<T> {
	data: T[];
	total: number;
	page: number;
	per_page: number;
	total_pages: number;
}
