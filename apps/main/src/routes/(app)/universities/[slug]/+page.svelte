<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { GraduationCap, Users, MessageSquare, Info, ArrowRight, FileText, Star, GitCompare, BookOpen, UsersRound, ExternalLink } from 'lucide-svelte';
	import { EmptyState, UrlTabs, type UrlTab } from '$lib/components/ui';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const university = $derived(data.university);
	const posts = $derived(data.posts ?? []);
	const reviews = $derived(data.reviews ?? []);
	const members = $derived(data.members ?? []);
	const forums = $derived(data.forums ?? []);
	let activeTab = $state(untrack(() => page.url.searchParams.get('tab') ?? 'about'));

	// Wiki content: the API stores it as base64-encoded JSON. The
	// space:auto_populate task (Section 39 / Wikidata enrichment) stores one of
	// two shapes — rich: {title, extract, description, thumbnail, content_url},
	// or basic: {name, country, founded, website, description, wikipedia_url}.
	// Decode and surface the text/image for the Overview tab. Mirrors the
	// /spaces/[slug] page (the established convention) so the university page
	// never renders the raw base64 blob to the user.
	interface WikiData {
		title?: string;
		extract?: string;
		description?: string;
		thumbnail?: string;
		content_url?: string;
		name?: string;
		country?: string;
		founded?: string;
		website?: string;
		wikipedia_url?: string;
	}
	const wikiData = $derived.by(() => {
		const raw = university.wiki_content;
		if (!raw) return null;
		try {
			const json = typeof raw === 'string' ? JSON.parse(decodeBase64Text(raw)) : raw;
			return json as WikiData;
		} catch {
			return null;
		}
	});

	// Founded year — Wikidata dates arrive as "+1889-00-00T00:00:00Z"; strip
	// the leading + and the placeholder month/day to leave just the year.
	const foundedYear = $derived(
		wikiData?.founded ? wikiData.founded.replace(/^\+/, '').replace(/-00-00T.*$/, '') : null
	);

	// Display-ready custom field values from the backend (Section 39 sidebar:
	// "key custom field values (ranking, tuition, programs, etc.)"). The
	// handler joins custom_field_values with their definitions, filters to
	// show_in_frontend = true, and resolves select/multiselect option values
	// to labels — so each entry is ready to render as {label, value}.
	const displayFields = $derived(university.display_fields ?? []);

	// Whether the wiki payload contributes any key facts (Website/Founded) so
	// we only render that grid when it adds something beyond display_fields.
	// Avoids a duplicate "Website" row when the custom field already covers it.
	const wikiFacts = $derived.by(() => {
		const facts: { label: string; value: string; href?: string }[] = [];
		const keys = new Set(displayFields.map((f) => f.key));
		// Only surface wiki Website when no official_website_url field exists.
		if (wikiData?.website && !keys.has('official_website_url')) {
			facts.push({
				label: t('university.website', 'Website'),
				value: wikiData.website.replace(/^https?:\/\//, '').replace(/\/$/, ''),
				href: wikiData.website
			});
		}
		if (foundedYear && !keys.has('founded_year')) {
			facts.push({ label: t('university.founded', 'Founded'), value: foundedYear });
		}
		return facts;
	});

	const title = $derived(
		t('university.title', '{name} — JapaConnect').replace('{name}', university.name)
	);
	const description = $derived(
		university.description ??
			t('university.description', 'Explore {name} on JapaConnect.').replace(
				'{name}',
				university.name
			)
	);

	// JSON-LD: EducationalOrganization + AggregateRating (H3 — audit finding).
	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'EducationalOrganization',
			name: university.name,
			description: university.description ?? '',
			url: page.url.href,
			...(university.logo_url ? { logo: { '@type': 'ImageObject', url: university.logo_url } } : {}),
			...(reviews.length > 0
				? {
						aggregateRating: {
							'@type': 'AggregateRating',
							ratingValue: reviews.reduce((s, r) => s + r.rating, 0) / reviews.length,
							reviewCount: reviews.length
						}
					}
				: {})
		})
	);

	const stats = $derived([
		{
			icon: Users,
			label: t('university.members', 'Members'),
			value: university.member_count
		},
		{
			icon: MessageSquare,
			label: t('university.posts', 'Posts'),
			value: university.post_count
		},
		{
			icon: GraduationCap,
			label: t('university.followers', 'Followers'),
			value: university.follower_count
		}
	]);

	const tabs: UrlTab[] = [
		{ value: 'about', label: t('university.tab_about', 'Overview') },
		{ value: 'posts', label: t('university.tab_posts', 'Posts') },
		{ value: 'forums', label: t('university.tab_forums', 'Forums') },
		{ value: 'courses', label: t('university.tab_courses', 'Courses') },
		{ value: 'members', label: t('university.tab_members', 'Members') },
		{ value: 'reviews', label: t('university.tab_reviews', 'Reviews') },
		{ value: 'compare', label: t('university.tab_compare', 'Compare') }
	];
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{@html `<script type="application/ld+json">${jsonLd}</script>`}
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- University header -->
	<div class="border-b border-[var(--color-border)] px-4 py-4">
		<a
			href="/universities"
			class="mb-2 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			<GraduationCap size={16} /> {t('university.back', 'All universities')}
		</a>
		<div class="flex items-center gap-3">
			{#if university.logo_url}
				<img
					src={university.logo_url}
					alt={t('university.logo_alt', 'Logo of {name}').replace('{name}', university.name)}
					class="h-14 w-14 shrink-0 rounded-[var(--radius-card)] object-cover"
				/>
			{:else}
				<span
					class="grid h-14 w-14 shrink-0 place-items-center rounded-[var(--radius-card)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
				>
					<GraduationCap size={26} />
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<h1 class="truncate text-xl font-semibold text-[var(--color-text)]">{university.name}</h1>
				<p class="text-xs capitalize text-[var(--color-muted)]">
					{university.level} · {university.visibility}
				</p>
			</div>
		</div>
		{#if university.description}
			<p class="mt-3 text-sm text-[var(--color-text-subtle)]">{university.description}</p>
		{/if}

		<!-- Compare button (Section 39) — opens the university comparison picker
		     with this university pre-selected as Item 1 (Section 30 Comparison
		     System). Links to /compare/universities?item1_slug=... per the spec
		     rather than the generic /compare/new wizard. -->
		<div class="mt-3 flex flex-wrap gap-2">
			<a
				href="/compare/universities?item1_slug={university.slug}"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
			>
				<GitCompare size={15} />
				{t('university.compare', 'Compare')}
			</a>
			<a
				href="/spaces/{university.slug}"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
			>
				<MessageSquare size={15} />
				{t('university.discussions', 'View discussions')}
			</a>
		</div>
	</div>

	<!-- Tabs — Section 39 / ADR-039: Overview / Posts / Forums / Courses / Members / Reviews / Compare -->
	<UrlTabs {tabs} bind:value={activeTab} class="px-4" />

	<div class="p-3">
		<!-- Stats row -->
		<div class="grid grid-cols-3 gap-2">
			{#each stats as stat (stat.label)}
				<div
					class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-center"
				>
					<stat.icon size={18} class="mx-auto text-[var(--color-text-subtle)]" />
					<p class="mt-1.5 text-lg font-semibold text-[var(--color-text)]">{stat.value}</p>
					<p class="text-xs text-[var(--color-muted)]">{stat.label}</p>
				</div>
			{/each}
		</div>

		{#if activeTab === 'about'}
			<!-- Overview / wiki — Section 39: "rich wiki content (TipTap editor for
			     admins/moderators)". The space:auto_populate task stores the
			     Wikidata extract as base64-encoded JSON in wiki_content; decode
			     and render it (mirroring /spaces/[slug]) instead of dumping the
			     raw base64 blob. Falls back to the Space description, then an
			     empty state. -->
			<section
				class="mt-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
			>
				<h2 class="flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
					<Info size={16} /> {t('university.about', 'About')}
				</h2>
				{#if wikiData}
					{#if wikiData.thumbnail}
						<img
							src={wikiData.thumbnail}
							alt={wikiData.title ?? university.name}
							class="float-right mb-3 ml-3 h-24 w-24 rounded-[var(--radius)] object-cover"
						/>
					{/if}
					{#if wikiData.description}
						<p class="mb-2 mt-3 text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{wikiData.description}
						</p>
					{/if}
					{#if wikiData.extract}
						<p class="mt-3 text-sm leading-relaxed text-[var(--color-text-subtle)]">
							{wikiData.extract}
						</p>
					{:else if university.description}
						<p class="mt-3 text-sm leading-relaxed text-[var(--color-text-subtle)]">
							{university.description}
						</p>
					{/if}
					{#if wikiData.content_url ?? wikiData.wikipedia_url}
						<p class="mt-3 text-xs">
							<a
								href={wikiData.content_url ?? wikiData.wikipedia_url}
								target="_blank"
								rel="noopener noreferrer"
								class="text-[var(--color-primary)] hover:underline"
							>
								{t('university.wiki_source', 'Read more on Wikipedia →')}
							</a>
						</p>
					{/if}
					<!-- Key facts (Section 39 sidebar: "key custom field values
				     (ranking, tuition, programs, etc.)"). Renders the
				     display-ready custom field values from the backend first,
				     then falls back to any facts the Wikidata wiki payload
				     contributes that aren't already covered by a custom field
				     (deduped by field key). URL-typed fields render as links. -->
				{#if displayFields.length > 0 || wikiFacts.length > 0}
					<dl class="mt-4 grid grid-cols-2 gap-3 border-t border-[var(--color-border)] pt-4 text-sm">
						{#each displayFields as field (field.key)}
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{field.label}</dt>
								<dd class="truncate text-[var(--color-text)]">
									{#if field.field_type === 'url' || field.field_type === 'email'}
										<a
											href={field.field_type === 'email' ? `mailto:${field.value}` : field.value}
											{...field.field_type === 'email' ? {} : { target: '_blank', rel: 'noopener noreferrer' }}
											class="text-[var(--color-primary)] hover:underline"
										>
											{field.value.replace(/^https?:\/\//, '').replace(/\/$/, '')}
										</a>
									{:else}
										{field.value}
									{/if}
								</dd>
							</div>
						{/each}
						{#each wikiFacts as fact (fact.label)}
							<div class="min-w-0">
								<dt class="text-xs text-[var(--color-muted)]">{fact.label}</dt>
								<dd class="truncate text-[var(--color-text)]">
									{#if fact.href}
										<a
											href={fact.href}
											target="_blank"
											rel="noopener noreferrer"
											class="text-[var(--color-primary)] hover:underline"
										>
											{fact.value}
										</a>
									{:else}
										{fact.value}
									{/if}
								</dd>
							</div>
						{/each}
					</dl>
				{/if}
			{:else if university.description}
					<p class="mt-3 text-sm leading-relaxed text-[var(--color-text-subtle)]">
						{university.description}
					</p>
				{:else}
					<div class="mt-3">
						<EmptyState
							icon={Info}
							title={t('university.no_about', 'No description yet')}
						/>
					</div>
				{/if}
			</section>
		{:else if activeTab === 'posts'}
			<!-- Posts tab -->
			{#if posts.length === 0}
				<div class="mt-3">
					<EmptyState icon={FileText} title={t('university.no_posts', 'No posts yet')} />
				</div>
			{:else}
				<div class="mt-3 divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each posts as post (post.id)}
						<a href="/content/{post.slug}" class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
							<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
								<span class="rounded-full bg-[var(--color-surface-subtle)] px-2 py-0.5 font-medium text-[var(--color-text-subtle)]">{post.content_type}</span>
								<span>· {post.read_time_minutes} {t('feed.min_read')}</span>
							</div>
							<h3 class="mt-1.5 text-sm font-medium text-[var(--color-text)]">{post.title}</h3>
							<div class="mt-2 flex items-center gap-4 text-xs text-[var(--color-muted)]">
								<span>{post.view_count} {t('profile.views', 'views')}</span>
								<span>{post.comment_count} {t('profile.comments', 'comments')}</span>
							</div>
						</a>
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'forums'}
			<!-- Forums tab — threads in this university's space -->
			{#if forums.length === 0}
				<div class="mt-3">
					<EmptyState icon={MessageSquare} title={t('university.no_forums', 'No forum threads yet')} />
				</div>
			{:else}
				<div class="mt-3 divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each forums as thread (thread.id)}
						<a href="/forum/{thread.slug}" class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]">
							<h3 class="text-sm font-medium text-[var(--color-text)]">{thread.title}</h3>
							<div class="mt-2 flex items-center gap-4 text-xs text-[var(--color-muted)]">
								<span>{thread.comment_count ?? 0} {t('profile.comments', 'comments')}</span>
								<span>{thread.view_count ?? 0} {t('profile.views', 'views')}</span>
							</div>
						</a>
					{/each}
				</div>
			{/if}
		{:else if activeTab === 'courses'}
			<!-- Courses tab — links to the Learn app (ADR-060: courses live on learn.japaconnect.com) -->
			<div class="mt-3">
				<a
					href="https://learn.japaconnect.com/university/{university.slug}"
					class="flex items-center justify-between rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<span class="flex items-center gap-2.5">
						<BookOpen size={20} class="text-[var(--color-primary)]" />
						<span class="min-w-0">
							<span class="block text-sm font-medium text-[var(--color-text)]">
								{t('university.courses_title', 'Courses & Tracks')}
							</span>
							<span class="block text-xs text-[var(--color-muted)]">
								{t('university.courses_hint', 'Browse courses on the Learn platform')}
							</span>
						</span>
					</span>
					<ExternalLink size={18} class="shrink-0 text-[var(--color-text-subtle)]" />
				</a>
			</div>
		{:else if activeTab === 'members'}
			<!-- Members tab -->
			{#if members.length === 0}
				<div class="mt-3">
					<EmptyState icon={UsersRound} title={t('university.no_members', 'No members yet')} />
				</div>
			{:else}
				<ul class="mt-3 space-y-2">
					{#each members as member (member.user_id)}
						<li>
							<a
								href="/u/{member.username}"
								class="flex items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{#if member.avatar_url}
									<img src={member.avatar_url} alt={t('profile.avatar_alt', 'Avatar of {name}').replace('{name}', member.display_name ?? member.username)} class="h-10 w-10 rounded-full object-cover" />
								{:else}
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">
										{member.display_name?.charAt(0)?.toUpperCase() ?? '?'}
									</span>
								{/if}
								<span class="min-w-0 flex-1">
									<span class="block truncate text-sm font-medium text-[var(--color-text)]">{member.display_name ?? member.username}</span>
									<span class="block text-xs text-[var(--color-muted)]">@{member.username}</span>
								</span>
							</a>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if activeTab === 'reviews'}
			<!-- Reviews tab -->
			{#if reviews.length === 0}
				<div class="mt-3">
					<EmptyState icon={Star} title={t('university.no_reviews', 'No reviews yet')} />
				</div>
			{:else}
				<ul class="mt-3 space-y-2">
					{#each reviews as review (review.id)}
						<li class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							<div class="flex items-center gap-2">
								<div class="flex">
									{#each Array(5) as _, i}
										<Star
											size={14}
											class={i < review.rating ? 'fill-[var(--color-primary)] text-[var(--color-primary)]' : 'text-[var(--color-muted)]'}
										/>
									{/each}
								</div>
								{#if review.title}
									<span class="text-sm font-medium text-[var(--color-text)]">{review.title}</span>
								{/if}
							</div>
							{#if review.body}
								<p class="mt-2 text-sm text-[var(--color-text-subtle)]">{review.body}</p>
							{/if}
							<p class="mt-2 text-xs text-[var(--color-muted)]">
								{review.author_name ?? '—'} · {new Date(review.created_at).toLocaleDateString()}
							</p>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if activeTab === 'compare'}
			<!-- Compare tab — opens the university comparison picker with this
			     university pre-selected as Item 1 (Section 39 / Section 30). -->
			<div class="mt-3">
				<a
					href="/compare/universities?item1_slug={university.slug}"
					class="flex items-center justify-between rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					<span class="flex items-center gap-2.5">
						<GitCompare size={20} class="text-[var(--color-primary)]" />
						<span class="min-w-0">
							<span class="block text-sm font-medium text-[var(--color-text)]">
								{t('university.compare_title', 'Compare with other universities')}
							</span>
							<span class="block text-xs text-[var(--color-muted)]">
								{t('university.compare_hint', 'Add this university to the comparison tool')}
							</span>
						</span>
					</span>
					<ArrowRight size={18} class="shrink-0 text-[var(--color-text-subtle)]" />
				</a>
			</div>
		{/if}

		<!-- CTA to discussions -->
		<a
			href="/spaces/{university.slug}"
			class="mt-3 flex items-center justify-between rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<span class="flex items-center gap-2.5">
				<MessageSquare size={20} class="text-[var(--color-primary)]" />
				<span class="min-w-0">
					<span class="block text-sm font-medium text-[var(--color-text)]">
						{t('university.discussions', 'View discussions')}
					</span>
					<span class="block text-xs text-[var(--color-muted)]">
						{t('university.discussions_hint', 'Join the conversation in this space')}
					</span>
				</span>
			</span>
			<ArrowRight size={18} class="shrink-0 text-[var(--color-text-subtle)]" />
		</a>
	</div>
</div>
