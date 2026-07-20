export const WEBHOOK_EVENT_TYPES = [
	'user.registered',
	'user.verified',
	'user.deleted',
	'user.updated',
	'content.created',
	'content.published',
	'content.deleted',
	'payment.succeeded',
	'payment.failed',
	'payment.refunded',
	'subscription.created',
	'subscription.cancelled'
] as const;

export type WebhookEventType = (typeof WEBHOOK_EVENT_TYPES)[number];
