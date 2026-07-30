<script lang="ts">
	/**
	 * Notifications settings — Section 35 (Notification System).
	 *
	 * Renders every enabled notification_type returned by
	 * GET /me/notification-preferences (the API merges notification_types
	 * defaults with the user's overrides — Rule 16: no hardcoded type list).
	 *
	 * Types are grouped into category tabs (Social, Content, Messages,
	 * Account, Billing, System) for scannability. Each row has three
	 * channel toggles: in-app, email, push.
	 *
	 * Saves via PATCH /me/notification-preferences (bulk update).
	 *
	 * Ref: japaconnect-engineering/sections/35-notification-system.md
	 *      japaconnect-engineering/sections/50-user-dashboard-settings.md
	 */
	import { untrack } from 'svelte';
	import { beforeNavigate } from '$app/navigation';
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import { patchNotifications } from '$lib/api/hub';
	import { t } from '$lib/i18n';
	import { Bell, Mail, Smartphone, MessageSquare, AtSign, Heart, Send, Building2, CreditCard, Shield, GraduationCap, Megaphone, Users } from 'lucide-svelte';
	import type { NotificationPreference } from '$lib/types';

	let { data }: { data: unknown } = $props();
	const apiPrefs = $derived((data as NotificationPreference[] | null) ?? []);

	// ---- Category mapping (Section 35 seed catalog) ----
	// Maps each notification type key to a category tab. Derived from the
	// JEOS spec grouping, consolidated into 6 tabs for UX.
	const CATEGORIES = [
		'social',
		'content',
		'messages',
		'account',
		'billing',
		'system'
	] as const;
	type Category = (typeof CATEGORIES)[number];

	const TYPE_CATEGORY: Record<string, Category> = {
		// Social
		'follow.received': 'social',
		'new_follower': 'social',
		'mention': 'social',
		'mention.content': 'social',
		'mention.comment': 'social',
		// Content engagement
		'content.reaction': 'content',
		'reaction': 'content',
		'content.featured': 'content',
		'content.comment': 'content',
		'comment': 'content',
		'content.reshare': 'content',
		'comment.reply': 'content',
		'post_reply': 'content',
		'comment.vote': 'content',
		// Forum
		'forum.solved': 'content',
		'forum.vote': 'content',
		// Messages
		'dm.message': 'messages',
		'dm_message': 'messages',
		'dm.request': 'messages',
		'dm_request': 'messages',
		// Account / Org / Reviews
		'org.invite': 'account',
		'org.member_joined': 'account',
		'claim.approved': 'account',
		'claim.rejected': 'account',
		'review.received': 'account',
		// Billing
		'payment.succeeded': 'billing',
		'payment.failed': 'billing',
		'subscription.renewed': 'billing',
		'subscription.cancelled': 'billing',
		'subscription.expiring': 'billing',
		// System / Moderation / Learning / Tickets / Broadcasts
		'ban.issued': 'system',
		'warning.issued': 'system',
		'appeal.resolved': 'system',
		'report.actioned': 'system',
		'course.completed': 'system',
		'certificate.issued': 'system',
		'ticket.status_update': 'system',
		'broadcast': 'system',
		'system_announcement': 'system'
	};

	const CATEGORY_ICON: Record<Category, typeof Bell> = {
		social: AtSign,
		content: Heart,
		messages: Send,
		account: Building2,
		billing: CreditCard,
		system: Megaphone
	};

	function categoryOf(type: string): Category {
		return TYPE_CATEGORY[type] ?? 'system';
	}

	// ---- Form state: flat map of type → { in_app, email, push } ----
	type PrefMap = Record<string, { in_app: boolean; email: boolean; push: boolean }>;

	const form = new DirtyForm<PrefMap>(
		untrack(() => {
			const map: PrefMap = {};
			for (const p of apiPrefs) {
				map[p.type] = { in_app: p.in_app, email: p.email, push: p.push };
			}
			return map;
		}),
		// PATCH /me/notification-preferences — bulk update with the full
		// preferences array so the backend can upsert every type at once.
		async (v) => {
			const preferences = Object.entries(v).map(([type, ch]) => ({
				type,
				in_app: ch.in_app,
				email: ch.email,
				push: ch.push
			}));
			return patchNotifications({ preferences });
		}
	);

	beforeNavigate((nav) => {
		if (form.dirty && !confirm(t('settings.unsaved_warning'))) nav.cancel();
	});

	function toggle(type: string, channel: 'in_app' | 'email' | 'push', next: boolean) {
		const current = form.values[type] ?? { in_app: false, email: false, push: false };
		form.set({ [type]: { ...current, [channel]: next } });
	}

	// ---- Tab state ----
	let activeTab = $state<Category>('social');

	const grouped = $derived.by(() => {
		const groups: Record<Category, NotificationPreference[]> = {
			social: [],
			content: [],
			messages: [],
			account: [],
			billing: [],
			system: []
		};
		for (const p of apiPrefs) {
			groups[categoryOf(p.type)].push(p);
		}
		return groups;
	});

	const tabCounts = $derived.by(() => {
		const counts: Record<Category, number> = {
			social: 0,
			content: 0,
			messages: 0,
			account: 0,
			billing: 0,
			system: 0
		};
		for (const p of apiPrefs) {
			counts[categoryOf(p.type)]++;
		}
		return counts;
	});
</script>

<section class="max-w-2xl">
	<!-- Category tabs — rendered above the title to match the section tab
	     bar position used by multi-section groups. -->
	<div class="mb-4 flex flex-wrap gap-1 border-b border-[var(--color-border)] px-1" role="tablist" aria-label={t('settings.notifications.categories_label', 'Notification categories')}>
		{#each CATEGORIES as cat (cat)}
			{@const Icon = CATEGORY_ICON[cat]}
			<button
				type="button"
				role="tab"
				aria-selected={activeTab === cat}
				onclick={() => (activeTab = cat)}
				class="inline-flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition-colors {activeTab === cat
					? 'border-[var(--color-primary)] text-[var(--color-primary)]'
					: 'border-transparent text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
			>
				<Icon size={14} />
				{t(`settings.notifications.cat_${cat}`)}
				<span class="ml-0.5 rounded-full bg-[var(--color-surface-subtle)] px-1.5 text-xs text-[var(--color-muted)]">{tabCounts[cat]}</span>
			</button>
		{/each}
	</div>

	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.notifications')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.notifications.desc')}</p>
	</header>

	<!-- Channel column headers -->
	<div class="mb-2 hidden items-center gap-4 border-b border-[var(--color-border)] pb-2 sm:flex">
		<span class="flex-1 text-xs font-medium text-[var(--color-muted)]">{t('settings.notifications.type', 'Type')}</span>
		<span class="flex w-32 items-center justify-center gap-1 text-xs font-medium text-[var(--color-muted)]">
			<Bell size={12} /> {t('settings.notifications.in_app', 'In-app')}
		</span>
		<span class="flex w-32 items-center justify-center gap-1 text-xs font-medium text-[var(--color-muted)]">
			<Mail size={12} /> {t('settings.notifications.email', 'Email')}
		</span>
		<span class="flex w-32 items-center justify-center gap-1 text-xs font-medium text-[var(--color-muted)]">
			<Smartphone size={12} /> {t('settings.notifications.push', 'Push')}
		</span>
	</div>

	<!-- Preference rows for the active tab -->
	<ul class="space-y-1">
		{#each grouped[activeTab] as pref (pref.type)}
			<li class="flex flex-col gap-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2.5 sm:flex-row sm:items-center sm:gap-4">
				<div class="flex-1 min-w-0">
					<p class="text-sm font-medium text-[var(--color-text)]">
						{t(`notif_type.${pref.type}.label`, pref.label)}
					</p>
					{#if pref.description}
						<p class="text-xs text-[var(--color-muted)]">{t(`notif_type.${pref.type}.desc`, pref.description)}</p>
					{/if}
				</div>
				<div class="flex items-center gap-4 sm:gap-0">
					<!-- In-app -->
					<label class="flex w-32 cursor-pointer items-center justify-center gap-1.5 text-sm text-[var(--color-text)]">
						<input
							type="checkbox"
							class="h-4 w-4 rounded border-[var(--color-border)] accent-[var(--color-primary)]"
							checked={form.values[pref.type]?.in_app ?? false}
							onchange={(e) => toggle(pref.type, 'in_app', e.currentTarget.checked)}
							aria-label="{t('settings.notifications.in_app', 'In-app')} — {pref.label}"
						/>
						<span class="sm:hidden">{t('settings.notifications.in_app', 'In-app')}</span>
					</label>
					<!-- Email -->
					<label class="flex w-32 cursor-pointer items-center justify-center gap-1.5 text-sm text-[var(--color-text)]">
						<input
							type="checkbox"
							class="h-4 w-4 rounded border-[var(--color-border)] accent-[var(--color-primary)]"
							checked={form.values[pref.type]?.email ?? false}
							onchange={(e) => toggle(pref.type, 'email', e.currentTarget.checked)}
							aria-label="{t('settings.notifications.email', 'Email')} — {pref.label}"
						/>
						<span class="sm:hidden">{t('settings.notifications.email', 'Email')}</span>
					</label>
					<!-- Push -->
					<label class="flex w-32 cursor-pointer items-center justify-center gap-1.5 text-sm text-[var(--color-text)]">
						<input
							type="checkbox"
							class="h-4 w-4 rounded border-[var(--color-border)] accent-[var(--color-primary)]"
							checked={form.values[pref.type]?.push ?? false}
							onchange={(e) => toggle(pref.type, 'push', e.currentTarget.checked)}
							aria-label="{t('settings.notifications.push', 'Push')} — {pref.label}"
						/>
						<span class="sm:hidden">{t('settings.notifications.push', 'Push')}</span>
					</label>
				</div>
			</li>
		{/each}
	</ul>

	{#if grouped[activeTab].length === 0}
		<p class="py-8 text-center text-sm text-[var(--color-muted)]">
			{t('settings.notifications.empty', 'No notification types in this category.')}
		</p>
	{/if}

	{#if form.dirty}
		<div
			class="sticky bottom-0 mt-6 flex items-center justify-end gap-2 border-t border-[var(--color-border)] bg-[var(--color-surface)] py-3"
		>
			{#if form.error}
				<span class="mr-auto text-xs text-[var(--color-danger,#dc2626)]">{form.error.message}</span>
			{/if}
			<button
				type="button"
				class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				onclick={() => form.reset()}>{t('settings.action.discard')}</button
			>
			<button
				type="button"
				disabled={form.saving}
				class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50"
				onclick={() => form.save()}>{t('settings.action.save')}</button
			>
		</div>
	{/if}
</section>
