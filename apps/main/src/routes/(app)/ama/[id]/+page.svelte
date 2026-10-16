<script lang="ts">
	// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import { ArrowLeft, Circle, CircleSlash, MessageSquare, Lock, Loader2 } from 'lucide-svelte';
	import { t, i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { tiptapBodyToHtml, tiptapExcerpt } from '$lib/tiptap/serialize';
	import { api } from '$lib/api/client';
	import { EmptyState } from '$lib/components/ui';
	import ReplyThread from '$lib/components/forum/ReplyThread.svelte';
	import { hydrateLinkPreviews } from '$lib/tiptap/hydrateLinkPreviews';
	import { hydrateMentions } from '$lib/tiptap/hydrateMentions';
	import { hydrateVideoEmbeds } from '$lib/tiptap/hydrateVideoEmbeds';
	import type { AMAThread, ForumReply } from '$lib/types';
	import type { PageData } from './$types';
import { safeJsonLdScript } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	const session = $derived(data.session);
	const replies = $derived(data.replies);

	// Seed reactive state from the server payload so the UI renders
	// synchronously during SSR and stays in sync on navigation.
	let liveReplies = $state<ForumReply[]>(untrack(() => data.replies));
	let closing = $state(false);
	let closeError = $state<string | null>(null);

	$effect(() => {
		const seed = data.replies;
		untrack(() => {
			liveReplies = seed;
			closing = false;
			closeError = null;
		});
	});

	type AmaConfig = {
		closes_at?: string;
		host_profession?: string;
		host_dest_country_id?: string;
		host_years_abroad?: number | null;
	};

	const cfg = $derived((session?.ama_config as AmaConfig | null) ?? null);

	const isLive = $derived.by(() => {
		if (!cfg?.closes_at) return true;
		return new Date(cfg.closes_at).getTime() > Date.now();
	});

	// BUG-007: resolve host_dest_country_id to a human-readable country name.
	const countryName = $derived.by(() => {
		const id = cfg?.host_dest_country_id;
		if (!id) return '';
		for (const c of data.countries ?? []) {
			if (c.id === id) return c.name;
		}
		return id;
	});

	const bodyHtml = $derived(tiptapBodyToHtml(session?.body));
	const time = $derived(
		session ? relativeTime(session.created_at, new Date(), i18n.locale) : ''
	);

	const hostName = $derived(session?.author_name || t('ama.host', 'Host'));

	const title = $derived(
		session
			? `${session.title} — ${t('ama.detail_title_suffix', 'Ask an Expat')}`
			: t('ama.detail_title_missing', 'AMA — JapaConnect')
	);
	const description = $derived(session ? tiptapExcerpt(session.body, 160) : '');
	// Section 26 — self-referencing canonical on the clean ID URL (never
	// the query-string form) so filtered/paginated hits consolidate.
	const canonical = $derived(`${page.url.origin}/ama/${session?.id ?? ''}`);

	// FAQPage JSON-LD for completed AMAs (§88 / §26). Each host reply that
	// answered a top-level question becomes an `acceptedAnswer`. Only emitted
	// once the AMA is closed (`!isLive`) — live sessions are not FAQ candidates.
	const faqJsonLd = $derived.by(() => {
		if (!session || isLive) return '';
		const hostId = session.author_user_id;
		if (!hostId) return '';
		const questions = replies.filter(
			(r) => !r.parent_reply_id && r.author_user_id !== hostId && !r.deleted_at
		);
		const faqs: { '@type': 'Question'; name: string; acceptedAnswer: { '@type': 'Answer'; text: string } }[] = [];
		for (const q of questions) {
			const answer = replies.find(
				(r) => r.parent_reply_id === q.id && r.author_user_id === hostId && !r.deleted_at
			);
			if (!answer) continue;
			faqs.push({
				'@type': 'Question',
				name: tiptapExcerpt(q.body, 160),
				acceptedAnswer: { '@type': 'Answer', text: tiptapExcerpt(answer.body, 320) }
			});
		}
		if (faqs.length === 0) return '';
		return JSON.stringify({ '@context': 'https://schema.org', '@type': 'FAQPage', mainEntity: faqs });
	});

	// Host-only control: close the AMA via PATCH /forum/ama/{id}. The backend
	// sets closes_at = now() server-side (no body needed) and enforces the
	// host-or-moderator permission. Ref: sections/88-ask-an-expat.md
	// Close is gated on host-or-moderator (backend enforces CanClose; the
	// server payload exposes isHost and the layout exposes the moderator
	// permission so guests never see the control).
	const canClose = $derived(
		!!session && isLive && (data.isHost || (page.data.user?.permissions?.includes('forum.ama.moderate') ?? false))
	);

	async function closeAma() {
		if (!session || closing) return;
		closing = true;
		closeError = null;
		const res = await api.patch<AMAThread>(`/forum/ama/${session.id}`, { closed: true });
		closing = false;
		if (res.error) {
			closeError = res.error.message || t('ama.close_error', 'Failed to close the AMA.');
			return;
		}
		await invalidateAll();
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={session?.title ?? title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<link rel="canonical" href={canonical} />
	<meta property="og:url" content={canonical} />
	{#if faqJsonLd}
		{@html safeJsonLdScript(faqJsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
	<a
		href="/ama"
		class="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft size={16} /> {t('ama.back', 'Back to AMAs')}
	</a>

	{#if data.loadError || !session}
		<EmptyState
			icon={MessageSquare}
			title={t('ama.load_error_title_detail', 'Failed to load AMA')}
			body={t('ama.load_error_body', 'Please try again later.')}
		/>
	{:else}
		<article
			class={`rounded-[var(--radius-card)] border p-4 sm:p-6 ${isLive ? 'border-[var(--color-primary)]/30 bg-[var(--color-primary)]/[0.03]' : 'border-[var(--color-border)] bg-[var(--color-surface)]'}`}
		>
			<!-- Header -->
			<div class="min-w-0 flex-1">
				<div class="flex flex-wrap items-center gap-2">
					<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-semibold text-[var(--color-primary)]">
						{t('ama.badge', 'AMA')}
					</span>
					{#if isLive}
						<span class="inline-flex items-center gap-0.5 rounded-full bg-green-500/10 px-2 py-0.5 text-xs font-semibold text-green-600">
							<Circle size={10} fill="currentColor" /> {t('ama.live', 'Live')}
						</span>
					{:else}
						<span class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-semibold text-[var(--color-text-subtle)]">
							<CircleSlash size={11} /> {t('ama.closed', 'Closed')}
						</span>
					{/if}

					{#if canClose}
						<button
							type="button"
							onclick={closeAma}
							disabled={closing || !isLive}
							class="ml-auto inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-2.5 py-1 text-xs font-medium text-[var(--color-text-subtle)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
						>
							{#if closing}
								<Loader2 size={13} class="animate-spin" />
								{t('common.loading', 'Loading')}
							{:else}
								<Lock size={13} />
								{t('ama.close', 'Close AMA')}
							{/if}
						</button>
					{/if}
				</div>

				<h1 class="mt-2 text-balance text-2xl font-bold leading-tight tracking-tight text-[var(--color-text)] sm:text-3xl">
					{session.title}
				</h1>

				<!-- Host info -->
				<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-[var(--color-text-subtle)]">
					<div class="flex items-center gap-1.5">
						{#if session.author_avatar}
							<img src={session.author_avatar} alt="" class="h-6 w-6 rounded-full object-cover" />
						{/if}
						<span class="font-medium text-[var(--color-text)]">{hostName}</span>
					</div>
					{#if cfg?.host_profession}
						<span>{cfg.host_profession}</span>
					{/if}
					{#if cfg?.host_dest_country_id}
						<span>{t('ama.in_country', 'in')} {countryName}</span>
					{/if}
					{#if cfg?.host_years_abroad != null}
						<span>{cfg.host_years_abroad} {t('ama.years_abroad', 'yrs abroad')}</span>
					{/if}
					{#if time}<span>·</span><span>{time}</span>{/if}
					{#if cfg?.closes_at}
						<span class={isLive ? 'text-green-600' : ''}>
							{isLive ? t('ama.closes_in', 'closes') : t('ama.closed_at', 'closed')} {relativeTime(cfg.closes_at, new Date(), i18n.locale)}
						</span>
					{/if}
				</div>

				{#if closeError}
					<p class="mt-3 rounded-[var(--radius)] border border-[var(--color-primary)]/30 bg-[color-mix(in_srgb,var(--color-primary)_8%,transparent)] px-3 py-2 text-sm text-[var(--color-primary)]">
						{closeError}
					</p>
				{/if}
			</div>

			<!-- Body -->
			<div class="prose mt-5 max-w-none text-[var(--color-text)]" use:hydrateLinkPreviews use:hydrateMentions use:hydrateVideoEmbeds>
				{@html bodyHtml}
			</div>
		</article>

		<!-- Threaded replies (includes a top-level composer when authed) -->
		<ReplyThread
			replies={liveReplies}
			threadId={session.id}
			authed={data.authed}
			canSolve={false}
			acceptedReplyId={null}
			hostUserId={session.author_user_id}
			isHost={data.isHost}
		/>
	{/if}
</div>
