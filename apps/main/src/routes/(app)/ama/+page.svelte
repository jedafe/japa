<script lang="ts">
	// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
	import { untrack } from 'svelte';
	import { page } from '$app/state';
	import { MessageSquare, Plus, Circle, CircleSlash, Loader2 } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { i18n } from '$lib/i18n';
	import { relativeTime } from '$lib/feed/time';
	import { tiptapExcerpt } from '$lib/tiptap/serialize';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import type { AMAThread } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let sessions = $state<AMAThread[]>(untrack(() => data.sessions));
	let atEnd = $state(false);
	let loading = $state(false);
	let sentinel = $state<HTMLElement | null>(null);

	$effect(() => {
		const seed = data.sessions;
		untrack(() => {
			sessions = seed;
			atEnd = seed.length < data.pageSize;
			loading = false;
		});
	});

	const title = $derived(t('ama.list_title', 'Ask an Expat — Live AMAs — JapaConnect'));
	const description = $derived(
		t('ama.list_description', 'Live Ask Me Anything sessions with experienced members of the diaspora.')
	);

	const userGroups = $derived(page.data.user?.groups ?? []);
	const canStart = $derived(
		!!page.data.user &&
			userGroups.some(
				(g) =>
					g === 'professional' ||
					g === 'Professional' ||
					g === 'agent' ||
					g === 'Agent' ||
					g === 'admin' ||
					g === 'Admin' ||
					g === 'super_admin' ||
					g === 'Super Admin'
			)
	);

	function cursor(): string | null {
		const last = sessions[sessions.length - 1];
		return last ? last.created_at : null;
	}

	$effect(() => {
		const el = sentinel;
		if (!el || atEnd) return;
		const obs = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) loadMore();
			},
			{ rootMargin: '600px' }
		);
		obs.observe(el);
		return () => obs.disconnect();
	});

	async function loadMore() {
		if (loading || atEnd) return;
		const c = cursor();
		if (!c) return;
		loading = true;
		const params = new URLSearchParams({
			page_size: String(data.pageSize),
			cursor_created_at: c
		});
		const res = await api.get<AMAThread[]>(`/forum/ama?${params.toString()}`);
		if (res.error) {
			loading = false;
			return;
		}
		const batch = res.data ?? [];
		sessions = [...sessions, ...batch];
		atEnd = batch.length < data.pageSize;
		loading = false;
	}

	function amaConfig(thread: AMAThread): {
		closes_at?: string;
		host_profession?: string;
		host_dest_country_id?: string;
		host_years_abroad?: number | null;
	} | null {
		return (thread.ama_config as {
			closes_at?: string;
			host_profession?: string;
			host_dest_country_id?: string;
			host_years_abroad?: number | null;
		}) ?? null;
	}

	function isLive(thread: AMAThread): boolean {
		const cfg = amaConfig(thread);
		if (!cfg?.closes_at) return true;
		return new Date(cfg.closes_at).getTime() > Date.now();
	}

	function faqJsonLd(): string {
		const completed = sessions.filter((s) => !isLive(s));
		if (completed.length === 0) return '';
		const faqs = completed.slice(0, 10).map((s) => ({
			'@type': 'Question',
			name: s.title,
			acceptedAnswer: {
				'@type': 'Answer',
				text: tiptapExcerpt(s.body, 200)
			}
		}));
		return JSON.stringify({ '@context': 'https://schema.org', '@type': 'FAQPage', mainEntity: faqs });
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	{#if faqJsonLd()}
		{@html `<script type="application/ld+json">${faqJsonLd()}</script>`}
	{/if}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('ama.heading', 'Ask an Expat — Live AMAs')}
		</h1>
		{#if canStart}
			<a
				href="/ama/new"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<Plus size={14} /> {t('ama.start', 'Start an AMA')}
			</a>
		{/if}
	</div>

	<!-- List -->
	<div class="p-3">
		{#if data.loadError}
			<EmptyState
				icon={MessageSquare}
				title={t('ama.load_error_title', 'Failed to load AMAs')}
				body={t('ama.load_error_body', 'Please try again later.')}
			/>
		{:else if sessions.length === 0}
			<EmptyState
				icon={MessageSquare}
				title={t('ama.empty_title', 'No AMAs yet')}
				body={t('ama.empty_body', 'Check back soon for live Ask Me Anything sessions with experienced expats.')}
				action={canStart ? { label: t('ama.start', 'Start an AMA'), href: '/ama/new' } : undefined}
			/>
		{:else}
			<div class="space-y-3">
				{#each sessions as session (session.id)}
					{@const cfg = amaConfig(session)}
					{@const live = isLive(session)}
					<a
						href="/ama/{session.id}"
						class={`block rounded-[var(--radius-card)] border p-4 transition-colors hover:border-[var(--color-primary)]/40 ${live ? 'border-[var(--color-primary)]/30 bg-[var(--color-primary)]/[0.03]' : 'border-[var(--color-border)] bg-[var(--color-surface)]'}`}
					>
						<div class="flex flex-wrap items-center gap-2">
							<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-semibold text-[var(--color-primary)]">
								{t('ama.badge', 'AMA')}
							</span>
							{#if live}
								<span class="inline-flex items-center gap-0.5 rounded-full bg-green-500/10 px-2 py-0.5 text-xs font-semibold text-green-600">
									<Circle size={10} fill="currentColor" /> {t('ama.live', 'Live')}
								</span>
							{:else}
								<span class="inline-flex items-center gap-0.5 rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 text-xs font-semibold text-[var(--color-text-subtle)]">
									<CircleSlash size={11} /> {t('ama.closed', 'Closed')}
								</span>
							{/if}

						</div>

						<h2 class="mt-2 text-base font-semibold text-[var(--color-text)]">{session.title}</h2>

						<div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-text-subtle)]">
							<div class="flex items-center gap-1.5">
								{#if session.author_avatar}
									<img src={session.author_avatar} alt="" class="h-5 w-5 rounded-full object-cover" />
								{/if}
								<span class="font-medium text-[var(--color-text)]">
									{session.author_name || t('ama.host', 'Host')}
								</span>
							</div>
							{#if cfg?.host_profession}
								<span>{cfg.host_profession}</span>
							{/if}
							{#if cfg?.host_dest_country_id}
								<span>{t('ama.in_country', 'in')} {cfg.host_dest_country_id}</span>
							{/if}
							{#if cfg?.host_years_abroad != null}
								<span>{cfg.host_years_abroad} {t('ama.years_abroad', 'yrs abroad')}</span>
							{/if}
							{#if cfg?.closes_at}
								<span class={live ? 'text-green-600' : ''}>
									{live ? t('ama.closes_in', 'closes') : t('ama.closed_at', 'closed')} {relativeTime(cfg.closes_at, new Date(), i18n.locale)}
								</span>
							{/if}
						</div>
					</a>
				{/each}
			</div>

			{#if !atEnd}
				<div bind:this={sentinel} class="h-8" aria-hidden="true"></div>
				{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
			{:else}
				<p class="py-6 text-center text-xs text-[var(--color-muted)]">
					{t('feed.end', "You're all caught up")}
				</p>
			{/if}
		{/if}
	</div>
</div>
