// Admin API Response Types
// These types provide type safety for API responses

// Common response wrapper
export interface PaginatedResponse<T> {
	items: T[];
	total: number;
	page: number;
	limit: number;
}

// User types
export interface User {
	id: string;
	username: string;
	email: string;
	groups: string[];
	status: 'active' | 'suspended' | 'banned' | 'pending';
	profile_type: 'personal' | 'business' | 'organisation';
	created_at: string;
	avatar_url?: string;
	last_login_at?: string;
}

export interface UserDetail extends User {
	profile?: Record<string, unknown>;
	roles?: string[];
	active_bans?: Ban[];
	warning_history?: Warning[];
	recent_content?: Content[];
	ai_context_summary?: string;
	wallet_balances?: { points: number; credits: number };
	subscription_status?: string;
	profile_type: 'personal' | 'business' | 'organisation';
}

export interface Ban {
	id: string;
	reason: string;
	ban_type: 'temporary' | 'permanent';
	expires_at?: string;
	created_at: string;
	is_active: boolean;
}

export interface Warning {
	id: string;
	reason: string;
	created_at: string;
	expires_at?: string;
}

export interface Activity {
	type: string;
	content: string;
	created_at: string;
}

export interface AiContextEntry {
	id: string;
	entry_type: string;
	content: string;
	created_at: string;
}

export interface Session {
	id: string;
	ip_address: string;
	device: string;
	started_at: string;
}

// Space types
export interface Space {
	id: string;
	name: string;
	owner: string;
	owner_id: string;
	member_count: number;
	status: 'active' | 'suspended' | 'archived';
	space_type: string;
	created_at: string;
	slug?: string;
	description?: string;
}

// Content types
export interface Content {
	id: string;
	title: string;
	author: string;
	author_id: string;
	type: 'post' | 'comment' | 'blog' | 'forum';
	status: 'draft' | 'published' | 'removed';
	reports_count: number;
	is_featured: boolean;
	created_at: string;
	body_preview?: string;
	body?: string;
	audit_trail?: AuditEntry[];
}

export interface AuditEntry {
	action: string;
	actor: string;
	timestamp: string;
	details?: string;
}

// Tag types
export interface Tag {
	id: string;
	name: string;
	slug: string;
	post_count: number;
	follower_count: number;
	created_at: string;
}

// Ticket types
export interface Ticket {
	id: string;
	type: string;
	subject: string;
	status: 'open' | 'in_progress' | 'resolved' | 'closed';
	priority: 'low' | 'medium' | 'high' | 'urgent';
	assignee: string;
	created_at: string;
	description?: string;
}

export interface TicketType {
	id: string;
	name: string;
	slug: string;
	description: string;
	is_active: boolean;
	default_priority: string;
}

// Moderation types
export interface ModerationItem {
	id: string;
	type: 'content' | 'comment' | 'user' | 'space';
	title: string;
	status: 'pending' | 'reviewing' | 'resolved';
	priority: 'urgent' | 'normal';
	report_count: number;
	created_at: string;
}

export interface ModerationAlert {
	id: string;
	type: string;
	message: string;
	severity: 'low' | 'medium' | 'high' | 'critical';
	created_at: string;
	acknowledged: boolean;
}

// Analytics types
export interface AnalyticsOverview {
	total_users: number;
	active_users: number;
	total_spaces: number;
	total_content: number;
	revenue: number;
	growth_metrics: {
		users: number;
		spaces: number;
		content: number;
	};
}

export interface UserAnalytics {
	daily_active: number;
	weekly_active: number;
	monthly_active: number;
	new_users_today: number;
	retention_rate: number;
}

export interface ContentAnalytics {
	total_posts: number;
	total_comments: number;
	engagement_rate: number;
	top_content: Content[];
}

export interface SpaceAnalytics {
	total_spaces: number;
	active_spaces: number;
	top_spaces: Space[];
}

export interface RevenueAnalytics {
	total_revenue: number;
	revenue_today: number;
	revenue_this_month: number;
	top_transactions: Transaction[];
}

export interface Transaction {
	id: string;
	user_id: string;
	amount: number;
	type: string;
	status: string;
	created_at: string;
}

// Spam types
export interface IpBlock {
	id: string;
	cidr: string;
	block_type: 'permanent' | 'temporary';
	reason?: string;
	expires_at?: string;
	created_at: string;
}

export interface CensoredWord {
	id: string;
	pattern: string;
	action: 'replace' | 'block' | 'flag';
	replacement?: string;
	created_at: string;
}

// Wallet types
export interface WalletRule {
	id: string;
	event_type: string;
	points_value: number;
	description?: string;
	is_active: boolean;
}

export interface WalletSettings {
	points_to_credits_rate: number;
	daily_earning_cap: number;
	enable_earning: boolean;
}

// Blog types
export interface BlogPost {
	id: string;
	title: string;
	author: string;
	status: 'draft' | 'pending_review' | 'published' | 'rejected';
	created_at: string;
	rejection_reason?: string;
}

// Notification types
export interface NotificationType {
	key: string;
	label: string;
	description: string;
	delivery_channels: string[];
	is_active: boolean;
}

// GDPR types
export interface DeletionRequest {
	id: string;
	user_id: string;
	reason: string;
	status: 'pending' | 'approved' | 'executed';
	created_at: string;
	executed_at?: string;
}

// Audit log types
export interface AuditLog {
	id: string;
	actor: string;
	action: string;
	entity_type: string;
	entity_id: string;
	details?: Record<string, unknown>;
	ip_address: string;
	created_at: string;
}

// Permission types
export interface Permission {
	node: string;
	description: string;
	category: string;
}

// Role types
export interface Role {
	id: string;
	name: string;
	description?: string;
	permissions: string[];
	created_at: string;
}

// Group types
export interface UserGroup {
	id: string;
	name: string;
	slug: string;
	max_spaces_creatable: number;
	member_count: number;
	created_at: string;
}

// Ad types
export interface AdCampaign {
	id: string;
	name: string;
	status: 'draft' | 'active' | 'paused' | 'completed';
	start_date: string;
	end_date?: string;
	budget: number;
	spent: number;
	impressions: number;
	clicks: number;
}

// Newsletter types
export interface NewsletterCampaign {
	id: string;
	subject: string;
	status: 'draft' | 'scheduled' | 'sent';
	scheduled_at?: string;
	sent_at?: string;
	recipients_count: number;
	open_rate?: number;
	click_rate?: number;
}

export interface Subscriber {
	id: string;
	email: string;
	name?: string;
	is_active: boolean;
	subscribed_at: string;
}

// Webhook types
export interface WebhookEndpoint {
	id: string;
	name: string;
	url: string;
	events: string[];
	is_active: boolean;
	secret: string;
	created_at: string;
}

export interface WebhookDelivery {
	id: string;
	event: string;
	status: 'success' | 'failed';
	status_code: number;
	created_at: string;
}

// Feature flag types
export interface FeatureFlag {
	id: string;
	name: string;
	key: string;
	description: string;
	is_enabled: boolean;
	environment: string;
	rollout_percentage: number;
	created_at: string;
	updated_at: string;
}

// Data export types
export interface DataExport {
	id: string;
	name: string;
	type: string;
	format: string;
	status: 'pending' | 'processing' | 'completed' | 'failed';
	file_size?: number;
	file_url?: string;
	created_at: string;
	completed_at?: string;
	error_message?: string;
}

// AI types
export interface AiModel {
	id: string;
	name: string;
	label: string;
	is_active: boolean;
}

export interface AiTool {
	id: string;
	name: string;
	description: string;
	is_active: boolean;
	usage_count: number;
}

export interface AiUsage {
	total_requests: number;
	total_tokens: number;
	total_cost: number;
	models: { model: string; requests: number; tokens: number }[];
}

// i18n types
export interface Language {
	code: string;
	name: string;
	flag?: string;
	completion?: number;
	is_default: boolean;
	is_enabled: boolean;
}

export interface Translation {
	key: string;
	value: string;
	language: string;
	updated_at: string;
}

// Professional profile types
export interface ProfileType {
	id: string;
	name: string;
	slug: string;
	description?: string;
}

export interface ProfileMember {
	id: string;
	user_id: string;
	role: string;
	created_at: string;
}

// Comparison types
export interface ComparisonType {
	id: string;
	name: string;
	slug: string;
	description?: string;
	is_public: boolean;
}

// Page builder types
export interface PageBlock {
	id: string;
	type: string;
	name: string;
	content: Record<string, unknown>;
	sort_order: number;
}

export interface PageTheme {
	id: string;
	name: string;
	is_default: boolean;
	created_at: string;
}

// Mentorship types
export interface MentorshipProgram {
	id: string;
	name: string;
	description: string;
	is_active: boolean;
	max_mentees: number;
	created_at: string;
}

// Learning types
export interface LearningTrack {
	id: string;
	title: string;
	slug: string;
	description: string;
	is_published: boolean;
	modules_count: number;
	enrolled_count: number;
	created_at: string;
}

export interface LearningModule {
	id: string;
	title: string;
	sort_order: number;
	lessons_count: number;
}

export interface LearningLesson {
	id: string;
	title: string;
	type: 'video' | 'text' | 'quiz';
	duration_minutes: number;
	sort_order: number;
}

// Coupon types
export interface Coupon {
	id: string;
	code: string;
	discount_type: 'percentage' | 'fixed';
	discount_value: number;
	max_uses?: number;
	used_count: number;
	expires_at?: string;
	is_active: boolean;
	created_at: string;
}

// Bot types
export interface Bot {
	id: string;
	name: string;
	description?: string;
	is_active: boolean;
	created_at: string;
}

// Broadcast types
export interface Broadcast {
	id: string;
	title: string;
	message: string;
	status: 'draft' | 'scheduled' | 'sent';
	scheduled_at?: string;
	sent_at?: string;
	recipients_count: number;
}

// Widget types
export interface Widget {
	id: string;
	name: string;
	type: string;
	content: Record<string, unknown>;
	is_active: boolean;
	created_at: string;
}

// Form types
export interface Form {
	id: string;
	name: string;
	slug: string;
	fields: FormField[];
	is_active: boolean;
	created_at: string;
}

export interface FormField {
	id: string;
	label: string;
	type: string;
	required: boolean;
	options?: string[];
	sort_order: number;
}
