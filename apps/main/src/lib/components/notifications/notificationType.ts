import {
	Bell,
	AtSign,
	Heart,
	UserPlus,
	MessageSquare,
	Send,
	CreditCard,
	Building2,
	Users,
	GraduationCap,
	ShieldAlert,
	Megaphone,
	Ticket,
	Award
} from 'lucide-svelte';
import type { IconComponent } from '$lib/components/ui/icons';

/**
 * Filterable notification categories used by the full notifications page.
 *
 * Ref: japaconnect-engineering/sections/35-notification-system.md
 */
export type NotificationFilterTab =
	| 'all'
	| 'mentions'
	| 'reactions'
	| 'follows'
	| 'replies'
	| 'dms'
	| 'system';

export interface NotificationFilterDef {
	key: NotificationFilterTab;
	icon: IconComponent;
	labelKey: string;
	fallback: string;
}

export const NOTIFICATION_FILTER_TABS: NotificationFilterDef[] = [
	{ key: 'all', icon: Bell, labelKey: 'notifications.filter_all', fallback: 'All' },
	{ key: 'mentions', icon: AtSign, labelKey: 'notifications.filter_mentions', fallback: 'Mentions' },
	{ key: 'reactions', icon: Heart, labelKey: 'notifications.filter_reactions', fallback: 'Reactions' },
	{ key: 'follows', icon: UserPlus, labelKey: 'notifications.filter_follows', fallback: 'Follows' },
	{ key: 'replies', icon: MessageSquare, labelKey: 'notifications.filter_replies', fallback: 'Replies' },
	{ key: 'dms', icon: Send, labelKey: 'notifications.filter_dms', fallback: 'DMs' },
	{ key: 'system', icon: Megaphone, labelKey: 'notifications.filter_system', fallback: 'System' }
];

/**
 * Maps a notification_type value to its filter tab.
 */
export function notificationFilterFor(type: string): NotificationFilterTab {
	if (type.startsWith('mention')) return 'mentions';
	if (
		type.startsWith('reaction') ||
		type === 'content.reaction' ||
		type === 'comment.vote' ||
		type === 'forum.vote'
	)
		return 'reactions';
	if (type.startsWith('follow') || type === 'new_follower') return 'follows';
	if (
		type.startsWith('comment') ||
		type === 'content.comment' ||
		type === 'post_reply' ||
		type === 'comment.reply' ||
		type === 'forum.solved'
	)
		return 'replies';
	if (type.startsWith('dm')) return 'dms';
	return 'system';
}

/**
 * Maps a notification_type value to a representative Lucide icon.
 */
export function notificationIconFor(type: string): IconComponent {
	if (type.startsWith('mention')) return AtSign;
	if (
		type.startsWith('reaction') ||
		type === 'content.reaction' ||
		type === 'comment.vote' ||
		type === 'forum.vote'
	)
		return Heart;
	if (type.startsWith('follow') || type === 'new_follower') return UserPlus;
	if (type.startsWith('dm')) return Send;
	if (
		type.startsWith('comment') ||
		type === 'content.comment' ||
		type === 'post_reply' ||
		type === 'comment.reply' ||
		type === 'forum.solved'
	)
		return MessageSquare;
	if (type.startsWith('org.') || type.startsWith('claim.') || type === 'review.received')
		return Users;
	if (type.startsWith('payment.') || type.startsWith('subscription.')) return CreditCard;
	if (type.startsWith('course.') || type.startsWith('certificate.')) return GraduationCap;
	if (
		type.startsWith('ban.') ||
		type.startsWith('warning.') ||
		type.startsWith('appeal.') ||
		type.startsWith('report.') ||
		type === 'scam_alert.confirmed'
	)
		return ShieldAlert;
	if (type === 'ticket.status_update') return Ticket;
	if (type === 'broadcast' || type === 'system_announcement' || type === 'content.featured')
		return Megaphone;
	if (type === 'content.reshare') return Award;
	return Bell;
}
