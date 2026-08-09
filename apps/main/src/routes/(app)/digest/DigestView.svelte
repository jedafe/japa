// Ref: japaconnect-engineering/sections/84-community-digest-widget.md
<script lang="ts">
	import { t } from '$lib/i18n';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import {
		Newspaper,
		MessageSquare,
		UserPlus,
		CalendarDays,
		BookOpen,
		Vote,
		ArrowLeft
	} from 'lucide-svelte';
	import type { IconComponent } from '$lib/components/ui/icons';
	import type {
		DigestEdition,
		DigestContent,
		DigestThread,
		DigestMember,
		DigestEvent,
		DigestStory,
		DigestPoll
	} from '$lib/types';

	interface Props {
		digest: DigestEdition | null;
		loadError: boolean;
		/** Show the back link to /digest (single edition pages). */
		showBack?: boolean;
		/** Show a status badge when the edition is not published. */
		showStatus?: boolean;
	}

	let { digest, loadError, showBack = false, showStatus = false }: Props = $props();

	const title = $derived(t('digest.title', 'Community Digest'));
	const description = $derived(
		t('digest.description', 'A round-up of the best of the community, delivered on a cadence.')
	);

	/** Defensively parse the content JSONB into a known shape. The backend stores
	 * content as a JSONB object; it may arrive as an object, a string, or null. */
	const content = $derived<DigestContent | null>(parseContent(digest?.content));

	function parseContent(raw: unknown): DigestContent | null {
		if (raw === null || raw === undefined) return null;
		if (typeof raw === 'string') {
			try {
				const parsed = JSON.parse(raw);
				if (parsed && typeof parsed === 'object') return parsed as DigestContent;
			} catch {
				return null;
			}
			return null;
		}
		if (typeof raw === 'object' && !Array.isArray(raw)) return raw as DigestContent;
		return null;
	}

	const cadenceLabel = $derived(
		digest?.cadence === 'monthly'
			? t('digest.cadence_monthly', 'Monthly')
			: t('digest.cadence_weekly', 'Weekly')
	);

	const publishedDate = $derived(
		digest?.published_at
			? new Date(digest.published_at).toLocaleDateString(undefined, {
					year: 'numeric',
					month: 'long',
					day: 'numeric'
				})
			: null
	);

	const statusLabel = $derived(
		digest && digest.status !== 'published'
			? t(`digest.status_${digest.status}`, digest.status)
			: null
	);

	const topThreads = $derived(content?.top_threads ?? []);
	const newMembers = $derived(content?.new_members ?? []);
	const events = $derived(content?.events ?? []);
	const story = $derived(content?.story ?? null);
	const poll = $derived(content?.poll ?? null);

	function threadHref(t: DigestThread): string | null {
		if (t.url) return t.url;
		if (t.slug) return `/forum/${t.slug}`;
		return null;
	}

	function storyHref(s: DigestStory): string | null {
		if (s.url) return s.url;
		if (s.slug) return `/stories/${s.slug}`;
		return null;
	}

	function eventHref(e: DigestEvent): string | null {
		return e.url ?? null;
	}

	function memberName(m: DigestMember): string {
		return m.name ?? m.username ?? t('digest.member', 'Member');
	}

	const sectionIcons: Record<string, IconComponent> = {
		top_threads: MessageSquare,
		new_members: UserPlus,
		events: CalendarDays,
		story: BookOpen,
		poll: Vote
	};

	const sectionTitles: Record<string, string> = {
		top_threads: t('digest.section_top_threads', 'Top threads'),
		new_members: t('digest.section_new_members', 'New members'),
		events: t('digest.section_events', 'Upcoming events'),
		story: t('digest.section_story', 'Featured story'),
		poll: t('digest.section_poll', 'Community poll')
	};

	/** Known section keys rendered in a stable order. */
	const knownSections = ['top_threads', 'new_members', 'events', 'story', 'poll'] as const;

	/** Extra unknown keys present in content (defensive — render generically). */
	const extraSections = $derived(
		content
			? Object.keys(content).filter((k) => !knownSections.includes(k as (typeof knownSections)[number]))
			: []
	);
</script>

<svelte:head>
	<title>{title} — JapaConnect</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
	{#if showBack}
		<a
			href="/digest"
			class="mb-5 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} /> {t('digest.back', 'Back to digest')}
		</a>
	{/if}

	{#if loadError}
		<EmptyState
			icon={Newspaper}
			title={t('digest.load_error_title', 'Could not load digest')}
			body={t('digest.load_error_body', 'Please try again later.')}
		/>
	{:else if !digest}
		<EmptyState
			icon={Newspaper}
			title={t('digest.empty_title', 'No digest yet')}
			body={t('digest.empty_body', 'Check back soon for the latest community digest.')}
		/>
	{:else}
		<header class="mb-6 flex flex-wrap items-center gap-3 border-b border-[var(--color-border)] pb-4">
			<div class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
				<Newspaper class="h-5 w-5" />
			</div>
			<div class="min-w-0 flex-1">
				<h1 class="text-2xl font-semibold text-[var(--color-text)]">{title}</h1>
				<div class="mt-1 flex flex-wrap items-center gap-2 text-sm text-[var(--color-text-subtle)]">
					<span class="inline-flex items-center gap-1 rounded-full bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text)]">
						{cadenceLabel}
					</span>
					{#if publishedDate}
						<span>· {publishedDate}</span>
					{/if}
					{#if showStatus && statusLabel}
						<span class="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2.5 py-0.5 text-xs font-medium text-amber-600">
							{statusLabel}
						</span>
					{/if}
				</div>
			</div>
		</header>

		{#if !content}
			<EmptyState
				icon={Newspaper}
				title={t('digest.empty_content_title', 'This edition has no content')}
				body={t('digest.empty_content_body', 'The digest content has not been published yet.')}
			/>
		{:else}
			<div class="space-y-6">
				{#if topThreads.length > 0}
					<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-5">
						<h2 class="mb-3 flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
							<MessageSquare size={16} class="text-[var(--color-primary)]" />
							{sectionTitles.top_threads}
						</h2>
						<ul class="space-y-2">
							{#each topThreads as thread (thread.id ?? thread.title)}
								{@const href = threadHref(thread)}
								<li>
									{#if href}
										<a href={href} class="text-sm text-[var(--color-primary)] hover:underline">
											{thread.title ?? t('digest.untitled_thread', 'Untitled thread')}
										</a>
									{:else}
										<span class="text-sm text-[var(--color-text)]">
											{thread.title ?? t('digest.untitled_thread', 'Untitled thread')}
										</span>
									{/if}
								</li>
							{/each}
						</ul>
					</section>
				{/if}

				{#if newMembers.length > 0}
					<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-5">
						<h2 class="mb-3 flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
							<UserPlus size={16} class="text-[var(--color-primary)]" />
							{sectionTitles.new_members}
						</h2>
						<ul class="flex flex-wrap gap-2">
							{#each newMembers as member (member.id ?? member.username ?? member.name)}
								<li class="inline-flex items-center gap-1.5 rounded-full bg-[var(--color-surface-subtle)] px-3 py-1 text-sm text-[var(--color-text)]">
									{#if member.avatar_url}
										<img src={member.avatar_url} alt="" class="h-5 w-5 rounded-full object-cover" />
									{/if}
									{memberName(member)}
								</li>
							{/each}
						</ul>
					</section>
				{/if}

				{#if events.length > 0}
					<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-5">
						<h2 class="mb-3 flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
							<CalendarDays size={16} class="text-[var(--color-primary)]" />
							{sectionTitles.events}
						</h2>
						<ul class="space-y-2">
							{#each events as ev (ev.id ?? ev.title)}
								{@const href = eventHref(ev)}
								<li class="flex items-center gap-2 text-sm">
									{#if ev.starts_at}
										<span class="text-[var(--color-text-subtle)]">
											{new Date(ev.starts_at).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}
										</span>
									{/if}
									{#if href}
										<a href={href} class="text-[var(--color-primary)] hover:underline">
											{ev.title ?? t('digest.untitled_event', 'Untitled event')}
										</a>
									{:else}
										<span class="text-[var(--color-text)]">
											{ev.title ?? t('digest.untitled_event', 'Untitled event')}
										</span>
									{/if}
								</li>
							{/each}
						</ul>
					</section>
				{/if}

				{#if story}
					<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-5">
						<h2 class="mb-3 flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
							<BookOpen size={16} class="text-[var(--color-primary)]" />
							{sectionTitles.story}
						</h2>
						{#if storyHref(story)}
							<a href={storyHref(story)} class="text-sm text-[var(--color-primary)] hover:underline">
								{story.title ?? t('digest.untitled_story', 'Untitled story')}
							</a>
						{:else}
							<p class="text-sm text-[var(--color-text)]">
								{story.title ?? t('digest.untitled_story', 'Untitled story')}
							</p>
						{/if}
					</section>
				{/if}

				{#if poll}
					<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-5">
						<h2 class="mb-3 flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
							<Vote size={16} class="text-[var(--color-primary)]" />
							{sectionTitles.poll}
						</h2>
						<p class="text-sm text-[var(--color-text)]">
							{poll.question ?? t('digest.no_poll_question', 'Community poll')}
						</p>
						{#if poll.url}
							<a href={poll.url} class="mt-2 inline-block text-sm text-[var(--color-primary)] hover:underline">
								{t('digest.vote_now', 'Vote now')} →
							</a>
						{/if}
					</section>
				{/if}

				{#each extraSections as key (key)}
					{@const value = content[key]}
					<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 sm:p-5">
						<h2 class="mb-3 flex items-center gap-2 text-base font-semibold text-[var(--color-text)]">
							{sectionTitles[key] ?? key}
						</h2>
						<pre class="overflow-x-auto text-xs text-[var(--color-text-subtle)]">{JSON.stringify(value, null, 2)}</pre>
					</section>
				{/each}
			</div>
		{/if}
	{/if}
</div>
