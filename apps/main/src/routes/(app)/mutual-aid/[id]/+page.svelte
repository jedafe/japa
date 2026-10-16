<script lang="ts">
	// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { goto, invalidateAll } from '$app/navigation';
	import { page } from '$app/state';
	import {
		ArrowLeft,
		HandHeart,
		MapPin,
		Clock,
		MessageSquare,
		CheckCircle2,
		RefreshCw,
		Pencil,
		Trash2
	} from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { relativeTime } from '$lib/feed/time';
	import AppealButton from '$lib/components/appeals/AppealButton.svelte';
	import type { MutualAidPost, MutualAidPostType, MutualAidCategory } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const post = $derived(data.post);

	// The backend enforces ownership on fulfill/renew/delete. We show the
	// management buttons only to the post author; the backend rejects
	// non-owners (the error is surfaced via actionError).
	const canManage = $derived(!!data.authed && !!post?.author_id && post.author_id === data.currentUserId);

	let responding = $state(false);
	let fulfilling = $state(false);
	let renewing = $state(false);
	let deleting = $state(false);
	let actionError = $state<string | null>(null);
	let confirmDelete = $state(false);

	const title = $derived(`${post.title} — ${t('mutual_aid.list_title', 'Mutual Aid')}`);
	const description = $derived((post.description ?? '').slice(0, 160));

	function postTypeBadgeClass(pt: MutualAidPostType): string {
		const base = 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold';
		switch (pt) {
			case 'need':
				return `${base} bg-red-500/10 text-red-600`;
			case 'offer':
				return `${base} bg-green-500/10 text-green-600`;
			case 'skill_swap':
				return `${base} bg-blue-500/10 text-blue-600`;
			default:
				return `${base} bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]`;
		}
	}

	function postTypeLabel(pt: MutualAidPostType): string {
		switch (pt) {
			case 'need':
				return t('mutual_aid.type_need', 'Need');
			case 'offer':
				return t('mutual_aid.type_offer', 'Offer');
			case 'skill_swap':
				return t('mutual_aid.type_skill_swap', 'Skill swap');
			default:
				return pt;
		}
	}

	function categoryLabel(cat: MutualAidCategory): string {
		const map: Record<MutualAidCategory, string> = {
			housing: t('mutual_aid.cat_housing', 'Housing'),
			transport: t('mutual_aid.cat_transport', 'Transport'),
			documents: t('mutual_aid.cat_documents', 'Documents'),
			job_search: t('mutual_aid.cat_job_search', 'Job search'),
			language: t('mutual_aid.cat_language', 'Language'),
			social: t('mutual_aid.cat_social', 'Social'),
			other: t('mutual_aid.cat_other', 'Other')
		};
		return map[cat] ?? cat;
	}

	function statusBadgeClass(status: string): string {
		const base = 'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium';
		switch (status) {
			case 'active':
				return `${base} bg-green-500/10 text-green-600`;
			case 'fulfilled':
				return `${base} bg-[var(--color-primary)]/10 text-[var(--color-primary)]`;
			case 'expired':
				return `${base} bg-[var(--color-surface-subtle)] text-[var(--color-muted)]`;
			default:
				return `${base} bg-[var(--color-surface-subtle)] text-[var(--color-text-subtle)]`;
		}
	}

	function statusLabel(status: string): string {
		switch (status) {
			case 'active':
				return t('mutual_aid.status_active', 'Active');
			case 'fulfilled':
				return t('mutual_aid.status_fulfilled', 'Fulfilled');
			case 'expired':
				return t('mutual_aid.status_expired', 'Expired');
			default:
				return status;
		}
	}

	async function respond() {
		if (!data.authed || responding) return;
		responding = true;
		actionError = null;
		const res = await api.post<{ dm_url?: string; conversation_id?: string }>(
			`/mutual-aid/${post.id}/respond`
		);
		responding = false;
		if (res.error) {
			actionError = res.error.message || t('mutual_aid.respond_error', 'Failed to respond.');
			return;
		}
		// The backend opens a DM — navigate to the conversation if a URL is returned.
		if (res.data?.dm_url) {
			goto(res.data.dm_url);
		} else if (res.data?.conversation_id) {
			goto(`/messages/${res.data.conversation_id}`);
		}
	}

	async function fulfill() {
		if (fulfilling) return;
		fulfilling = true;
		actionError = null;
		const res = await api.post<MutualAidPost>(`/mutual-aid/${post.id}/fulfill`);
		fulfilling = false;
		if (res.error) {
			actionError = res.error.message || t('mutual_aid.fulfill_error', 'Failed to mark fulfilled.');
			return;
		}
		await invalidateAll();
	}

	async function renew() {
		if (renewing) return;
		renewing = true;
		actionError = null;
		const res = await api.post<MutualAidPost>(`/mutual-aid/${post.id}/renew`);
		renewing = false;
		if (res.error) {
			actionError = res.error.message || t('mutual_aid.renew_error', 'Failed to renew.');
			return;
		}
		await invalidateAll();
	}

	async function deletePost() {
		if (deleting) return;
		deleting = true;
		actionError = null;
		const res = await api.delete(`/mutual-aid/${post.id}`);
		deleting = false;
		if (res.error) {
			actionError = res.error.message || t('mutual_aid.delete_error', 'Failed to delete.');
			confirmDelete = false;
			return;
		}
		goto('/mutual-aid');
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={post.title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
	<a
		href="/mutual-aid"
		class="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft size={16} /> {t('mutual_aid.back', 'Back to mutual aid')}
	</a>

	<article class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-6">
		<!-- Badges -->
		<div class="flex flex-wrap items-center gap-2">
			<span class={postTypeBadgeClass(post.post_type as MutualAidPostType)}>
				{postTypeLabel(post.post_type as MutualAidPostType)}
			</span>
			{#if post.category}
				<span class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
					{categoryLabel(post.category as MutualAidCategory)}
				</span>
			{/if}
			<span class={statusBadgeClass(post.status)}>
				{statusLabel(post.status)}
			</span>
		</div>

		<!-- Title -->
		<h1 class="mt-3 text-balance text-2xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-3xl">
			{post.title}
		</h1>

		<!-- Meta -->
		<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
			<span class="font-medium text-[var(--color-text-subtle)]">{post.author_name}</span>
			<span>·</span>
			<span>{relativeTime(post.created_at, new Date(), i18n.locale)}</span>
			{#if post.destination_space_name || post.city_space_name}
				<span>·</span>
				<span class="inline-flex items-center gap-1">
					<MapPin size={12} />
					{[post.city_space_name, post.destination_space_name].filter(Boolean).join(', ')}
				</span>
			{/if}
			{#if post.expires_at}
				<span>·</span>
				<span class="inline-flex items-center gap-1">
					<Clock size={12} />
					{t('mutual_aid.expires', 'Expires')} {relativeTime(post.expires_at, new Date(), i18n.locale)}
				</span>
			{/if}
		</div>

		<!-- Description -->
		<div class="prose mt-5 max-w-none whitespace-pre-wrap text-[var(--color-text)]">
			{post.description}
		</div>

		{#if actionError}
			<p class="mt-4 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-600" role="alert">
				{actionError}
			</p>
		{/if}

		<!-- Action buttons -->
		<div class="mt-6 flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] pt-4">
			{#if data.authed}
				<button
					type="button"
					onclick={respond}
					disabled={responding}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					<MessageSquare size={14} />
					{responding ? '…' : t('mutual_aid.respond', 'Respond')}
				</button>
			{:else}
				<a
					href="/login?redirect=/mutual-aid/{post.id}"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
				>
					<MessageSquare size={14} /> {t('mutual_aid.respond', 'Respond')}
				</a>
			{/if}

			{#if canManage && post.status === 'removed'}
				<AppealButton entityType="mutual_aid_post" entityId={post.id} />
			{/if}

			{#if canManage}
				{#if !post.is_fulfilled && post.status !== 'fulfilled'}
					<button
						type="button"
						onclick={fulfill}
						disabled={fulfilling}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						<CheckCircle2 size={14} />
						{fulfilling ? '…' : t('mutual_aid.mark_fulfilled', 'Mark Fulfilled')}
					</button>
				{/if}

				{#if post.status !== 'fulfilled'}
					<button
						type="button"
						onclick={renew}
						disabled={renewing}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					>
						<RefreshCw size={14} />
						{renewing ? '…' : t('mutual_aid.renew', 'Renew')}
					</button>
				{/if}

				<a
					href="/mutual-aid/{post.id}/edit"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<Pencil size={14} /> {t('common.edit', 'Edit')}
				</a>

				{#if confirmDelete}
					<span class="inline-flex items-center gap-2">
						<span class="text-sm text-[var(--color-text-subtle)]">
							{t('mutual_aid.confirm_delete', 'Delete this post?')}
						</span>
						<button
							type="button"
							onclick={deletePost}
							disabled={deleting}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-red-500 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-red-600 disabled:opacity-50"
						>
							<Trash2 size={14} /> {deleting ? '…' : t('common.confirm', 'Confirm')}
						</button>
						<button
							type="button"
							onclick={() => (confirmDelete = false)}
							class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{t('common.cancel', 'Cancel')}
						</button>
					</span>
				{:else}
					<button
						type="button"
						onclick={() => (confirmDelete = true)}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-red-500/30 px-3 py-1.5 text-sm font-medium text-red-600 transition-colors hover:bg-red-500/10"
					>
						<Trash2 size={14} /> {t('common.delete', 'Delete')}
					</button>
				{/if}
			{/if}
		</div>
	</article>
</div>
