<script lang="ts">
	import { t } from '$lib/i18n';
	import UserHoverCard from '$lib/components/ui/UserHoverCard.svelte';
	import { page } from '$app/state';
	import { untrack } from 'svelte';
	import { api } from '$lib/api/client';
	import { GraduationCap, Users, MessageSquare, Info, ArrowRight, FileText, Star, GitCompare, BookOpen, UsersRound, ExternalLink, Pencil, Trash2, Heart } from 'lucide-svelte';
	import { EmptyState, UrlTabs, type UrlTab, SpaceJoinButton } from '$lib/components/ui';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import { decodeBase64Text } from '$lib/tiptap/serialize';
	import ReviewForm from '$lib/components/reviews/ReviewForm.svelte';
	import type { PageData } from './$types';
	import type { Review, CourseOffering, WikiPageIndex } from '$lib/types';
	import { safeJsonLdScript } from '$lib/utils/safeHtml';
	import { formatMoney } from '$lib/utils/currency';
	import { imgLightbox } from '$lib/lightbox.svelte';

	let { data }: { data: PageData } = $props();

	const university = $derived(data.university);
	const compareHref = $derived(data.compareHref);
	const posts = $derived(data.posts ?? []);
	// Use untrack() so the $state() initializer intentionally reads the load
	// data once without a reactivity warning.
	let reviews = $state<Review[]>(untrack(() => data.reviews ?? []));
	const reviewAggregate = $derived(data.reviewAggregate);
	const members = $derived(data.members ?? []);
	const forums = $derived(data.forums ?? []);
	// §39 — institutional degree offerings (Programs tab).
	const programs = $derived((data.programs ?? []) as CourseOffering[]);
	// §80 amendment — university spaces may carry community wiki pages,
	// gated by the wiki.enabled feature flag.
	const wikiEnabled = $derived(!!data.wikiEnabled);
	const wikiPages = $derived<WikiPageIndex[]>(data.wikiPages ?? []);
	const currentUser = $derived(data.user);
	let activeTab = $state(untrack(() => page.url.searchParams.get('tab') ?? 'about'));

	// Review edit state — PATCH /reviews/:id and DELETE /reviews/:id.
	let editingReview = $state<Review | null>(null);
	let editRating = $state(0);
	let editTitle = $state('');
	let editBody = $state('');
	let editSaving = $state(false);
	let editError = $state<string | null>(null);

	// Map reviewer_user_id -> display name so the review list can show names
	// without depending on the /spaces/:id/reviews convenience route. Members
	// are loaded with the page; the current user is added if available.
	const reviewerName = $derived.by(() => {
		const map: Record<string, string> = {};
		if (currentUser?.id) {
			map[currentUser.id] = currentUser.display_name ?? currentUser.username ?? t('reviews.you', 'You');
		}
		for (const m of members) {
			map[m.user_id] = m.display_name ?? m.username ?? t('reviews.anonymous', 'Anonymous');
		}
		return (id: string) => map[id] ?? t('reviews.anonymous', 'Anonymous');
	});

	// Derived stats for reviews; fall back to computing from the list when the
	// aggregate endpoint is unavailable (e.g. no reviews yet).
	const reviewCount = $derived(reviewAggregate?.review_count ?? reviews.length);
	const averageRating = $derived(
		reviewAggregate?.average_rating ?? (reviews.length > 0 ? reviews.reduce((s, r) => s + r.rating, 0) / reviews.length : 0)
	);

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
	// Uses the server-fetched aggregate when available so the rating reflects
	// the full review set, not just the first page.
	const jsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'EducationalOrganization',
			name: university.name,
			...(university.initials ? { alternateName: university.initials } : {}),
			description: university.description ?? '',
			url: page.url.href,
			...(university.logo_url ? { logo: { '@type': 'ImageObject', url: university.logo_url } } : {}),
			...(reviewCount > 0
				? {
						aggregateRating: {
							'@type': 'AggregateRating',
							ratingValue: averageRating,
							reviewCount
						}
					}
				: {})
		})
	);

	// When a new review is created, prepend it to the local list. The server
	// returns the canonical Review shape (Section 29), which includes
	// reviewer_user_id, so owner actions work immediately.
	function onReviewCreated(review: Review) {
		reviews = [review, ...reviews];
	}

	async function deleteReview(id: string) {
		const res = await api.delete<Review>(`/reviews/${id}`);
		if (res.error) return;
		reviews = reviews.filter((r) => r.id !== id);
		editingReview = null;
	}

	function startEdit(review: Review) {
		editingReview = review;
		editRating = review.rating;
		editTitle = review.title ?? '';
		editBody = review.body ?? '';
		editError = null;
	}

	function cancelEdit() {
		editingReview = null;
		editRating = 0;
		editTitle = '';
		editBody = '';
		editError = null;
	}

	async function saveEdit() {
		if (!editingReview) return;
		editSaving = true;
		editError = null;
		const res = await api.patch<Review>(`/reviews/${editingReview.id}`, {
			rating: editRating,
			title: editTitle.trim() || undefined,
			body: editBody.trim() || undefined
		});
		editSaving = false;
		if (res.error) {
			editError = res.error.message;
			return;
		}
		if (res.data) {
			reviews = reviews.map((r) => (r.id === res.data!.id ? res.data! : r));
		}
		cancelEdit();
	}

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
			icon: Heart,
			label: t('university.followers', 'Followers'),
			value: university.follower_count
		}
	]);

	const tabs: UrlTab[] = [
		{ value: 'about', label: t('university.tab_about', 'Overview') },
		{ value: 'posts', label: t('university.tab_posts', 'Posts') },
		{ value: 'forums', label: t('university.tab_forums', 'Forums') },
		{ value: 'programs', label: t('university.tab_programs', 'Programs') },
		{ value: 'members', label: t('university.tab_members', 'Members') },
		{ value: 'reviews', label: t('university.tab_reviews', 'Reviews') },
		{ value: 'compare', label: t('university.tab_compare', 'Compare') }
	];

	function formatWikiCategory(category: string): string {
		return category.replace(/_/g, ' ');
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
	{@html safeJsonLdScript(jsonLd)}
</svelte:head>

<SetSidebar>
	<div class="flex flex-col gap-4 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<div class="flex items-center gap-3">
			{#if university.logo_url}
				<img
					src={university.logo_url}
					alt={university.name}
					class="h-12 w-12 shrink-0 rounded-[var(--radius)] object-cover"
				/>
			{:else}
				<span
					class="grid h-12 w-12 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
				>
					<GraduationCap size={24} />
				</span>
			{/if}
			<div class="min-w-0 flex-1">
				<h2 class="truncate text-sm font-semibold text-[var(--color-text)]">{university.name}</h2>
				<p class="text-xs capitalize text-[var(--color-muted)]">{t('university.type', 'University')} · {university.visibility}</p>
			</div>
		</div>

		<SpaceJoinButton
			entityId={university.id}
			entitySlug={university.slug}
			initiallyJoined={university.is_member ?? false}
			initiallyPending={university.is_pending ?? false}
			initiallyFollowing={university.is_following ?? false}
			compact
		/>

		{#if wikiFacts.length > 0}
			<dl class="space-y-2 text-sm">
				{#each wikiFacts as fact (fact.label)}
					<div class="flex justify-between gap-2">
						<dt class="text-[var(--color-muted)]">{fact.label}</dt>
						<dd class="text-right text-[var(--color-text)]">
							{#if fact.href}
								<a href={fact.href} class="break-all text-[var(--color-primary)] hover:underline" target="_blank" rel="noopener">
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

		<a
			href={compareHref}
			class="inline-flex items-center justify-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
		>
			<GitCompare size={15} />
			{t('university.compare', 'Compare')}
		</a>
	</div>
</SetSidebar>

<div class="mx-auto w-full max-w-full">
	<!-- University header -->
	<div class="mx-3 border-b border-[var(--color-border)] px-4 py-4">
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
				<div class="flex items-center gap-2">
					{#if university.initials}
						<span class="shrink-0 rounded bg-[var(--color-primary)]/10 px-2 py-0.5 text-sm font-semibold text-[var(--color-primary)]"
							>{university.initials}</span>
					{/if}
					<h1 class="truncate text-xl font-semibold text-[var(--color-text)]">{university.name}</h1>
				</div>
				<p class="text-xs capitalize text-[var(--color-muted)]">
					{t('university.type', 'University')} · {university.visibility}
				</p>
			</div>
		</div>
		{#if university.description}
			<p class="mt-3 text-sm text-[var(--color-text-subtle)]">{university.description}</p>
		{/if}

		<!-- Compare button (Section 39) — opens the university comparison picker
		     with this university pre-selected as Item 1 (Section 30 Comparison
		     System). The href is resolved by the backend redirect endpoint
		     GET /universities/{slug}/compare so the frontend never hardcodes
		     the comparison picker path. -->
		<div class="mt-3 flex flex-wrap items-center gap-2">
			<a
				href={compareHref}
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
			{#if wikiEnabled}
				<!-- Wiki button (§80 amendment — University Spaces), wired like the
				     destination-space wiki button on /spaces/[slug]. -->
				<a
					href="/wiki/{university.slug}"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					data-sveltekit-preload-data="hover"
				>
					<BookOpen size={15} />
					{t('space.wiki', 'Wiki')}
				</a>
			{/if}
			<SpaceJoinButton
				entityId={university.id}
				entitySlug={university.slug}
				initiallyJoined={university.is_member ?? false}
				initiallyPending={university.is_pending ?? false}
				initiallyFollowing={university.is_following ?? false}
			/>
		</div>
	</div>

	<!-- Tabs — Section 39 / ADR-039: Overview / Posts / Forums / Courses / Members / Reviews / Compare -->
	<UrlTabs {tabs} bind:value={activeTab} class="mx-3 px-4" />

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
					<div use:imgLightbox>
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
				</div>
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

			{#if wikiEnabled}
				<!-- Community Wiki (§80 amendment — University Spaces) — same
				     wiring as the destination-space wiki section on /spaces/[slug]. -->
				<div class="mt-4 border-t border-[var(--color-border)] pt-4">
					<h2 class="mb-2 text-sm font-medium text-[var(--color-text)]">{t('space.community_wiki', 'Community Wiki')}</h2>
					{#if wikiPages.length > 0}
						<ul class="mb-3 space-y-2">
							{#each wikiPages as wikiPage (wikiPage.id)}
								<li>
									<a
										href="/wiki/{encodeURIComponent(university.slug)}/{encodeURIComponent(wikiPage.slug)}"
										class="flex items-center justify-between rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm transition-colors hover:border-[var(--color-primary)] hover:bg-[var(--color-surface)]"
										data-sveltekit-preload-data="hover"
									>
										<span class="font-medium text-[var(--color-text)]">{wikiPage.title}</span>
										<span class="text-xs uppercase tracking-wide text-[var(--color-muted)]">{formatWikiCategory(wikiPage.category)}</span>
									</a>
								</li>
							{/each}
						</ul>
					{:else}
						<p class="mb-3 text-sm text-[var(--color-text-subtle)]">{t('space.no_wiki_pages', 'No community wiki pages yet.')}</p>
					{/if}
					<a
						href="/wiki/{encodeURIComponent(university.slug)}"
						class="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline"
						data-sveltekit-preload-data="hover"
					>
						{t('space.view_wiki', 'View wiki')}
						<span aria-hidden="true">→</span>
					</a>
				</div>
			{/if}
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
		{:else if activeTab === 'programs'}
			<!-- Programs tab — institutional degree offerings (Section 39
			     university_course_offerings). Distinct from the Courses tab,
			     which lists Japa University creator tracks. -->
			{#if programs.length === 0}
				<div class="mt-3">
					<EmptyState icon={GraduationCap} title={t('university.no_programs', 'No programs listed yet')} body={t('university.no_programs_hint', 'Degree programs offered by this university will appear here.')} />
				</div>
			{:else}
				<div class="mt-3 divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
					{#each programs as offering (offering.id)}
						<a
							href="/universities/{university.slug}/courses/{offering.slug}"
							class="block p-4 transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							<div class="flex items-start justify-between gap-3">
								<p class="text-sm font-medium text-[var(--color-text)]">{offering.course?.title}</p>
								{#if offering.course?.degree_type}
									<span class="shrink-0 rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] font-semibold uppercase text-[var(--color-muted)]">{offering.course.degree_type}</span>
								{/if}
							</div>
							<div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-[var(--color-muted)]">
								{#if offering.tuition != null}
									<span class="font-medium text-[var(--color-text)]">{formatMoney(offering.tuition, offering.currency, data.currencies)}</span>
								{/if}
								{#if offering.duration_years}
									<span>{offering.duration_years} {t('university.years', 'yrs')}</span>
								{/if}
								{#if offering.study_mode}
									<span>{offering.study_mode}</span>
								{/if}
								{#if offering.intake_months?.length}
									<span>{t('university.intakes', 'Intakes')}: {offering.intake_months.join(', ')}</span>
								{/if}
							</div>
						</a>
					{/each}
				</div>
			{/if}
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
							<UserHoverCard
								id={member.user_id}
								slug={member.username}
								name={member.display_name ?? member.username}
								avatar={member.avatar_url}
								class="inline-flex w-full min-w-0 max-w-full items-center gap-3 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-left transition-colors hover:bg-[var(--color-surface-subtle)]"
							>
								{#snippet trigger()}
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
								{/snippet}
							</UserHoverCard>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if activeTab === 'reviews'}
			<!-- Reviews tab (Section 29 / Phase 17). -->
			{#if reviewCount > 0}
				<div class="mt-3 flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
					<div class="flex">
						{#each Array(5) as _, i}
							<Star
								size={16}
								class={i < Math.round(averageRating) ? 'fill-[var(--color-primary)] text-[var(--color-primary)]' : 'text-[var(--color-muted)]'}
							/>
						{/each}
					</div>
					<span class="text-sm text-[var(--color-text)]">
						{averageRating.toFixed(1)} / 5 · {reviewCount} {t('university.review_count', 'reviews')}
					</span>
				</div>
			{/if}

			{#if currentUser}
				<div class="mt-3">
					<ReviewForm entityType="university" entityId={university.id} onCreated={onReviewCreated} />
				</div>
			{/if}

			{#if reviews.length === 0}
				<div class="mt-3">
					<EmptyState icon={Star} title={t('university.no_reviews', 'No reviews yet')} />
				</div>
			{:else}
				<ul class="mt-3 space-y-2">
					{#each reviews as review (review.id)}
						<li class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
							{#if editingReview?.id === review.id}
								<div class="space-y-3">
									<div class="flex items-center gap-1">
										{#each [1, 2, 3, 4, 5] as n (n)}
											<button
												type="button"
												onclick={() => (editRating = n)}
												class="p-0.5"
												aria-label={t('reviews.set_rating', '{n} stars').replace('{n}', String(n))}
											>
												<Star
													size={18}
													class={n <= editRating ? 'fill-[var(--color-warning)] text-[var(--color-warning)]' : 'text-[var(--color-muted)]'}
												/>
											</button>
										{/each}
									</div>
									<input
										type="text"
										class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
										placeholder={t('reviews.title_placeholder', 'Title (optional)')}
										value={editTitle}
										oninput={(e) => (editTitle = e.currentTarget.value)}
									/>
									<textarea
										class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)]"
										rows="3"
										placeholder={t('reviews.body_placeholder', 'Share your experience…')}
										value={editBody}
										oninput={(e) => (editBody = e.currentTarget.value)}
									></textarea>
									{#if editError}<p class="text-xs text-[var(--color-danger,#dc2626)]">{editError}</p>{/if}
									<div class="flex justify-end gap-2">
										<button
											type="button"
											onclick={cancelEdit}
											class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
										>
											{t('common.cancel')}
										</button>
										<button
											type="button"
											disabled={editSaving || !editRating}
											onclick={saveEdit}
											class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50"
										>
											{editSaving ? t('common.saving') : t('common.save')}
										</button>
									</div>
								</div>
							{:else}
								<div class="flex items-start justify-between gap-2">
									<div class="min-w-0 flex-1">
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
											{reviewerName(review.reviewer_user_id)} · {new Date(review.created_at).toLocaleDateString()}
										</p>
									</div>
									{#if currentUser?.id && review.reviewer_user_id === currentUser.id}
										<div class="flex shrink-0 gap-1">
											<button
												type="button"
												onclick={() => startEdit(review)}
												class="rounded p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-text)]"
												aria-label={t('reviews.edit')}
												title={t('reviews.edit')}
											>
												<Pencil size={14} />
											</button>
											<button
												type="button"
												onclick={() => deleteReview(review.id)}
												class="rounded p-1.5 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)] hover:text-[var(--color-danger)]"
												aria-label={t('reviews.delete')}
												title={t('reviews.delete')}
											>
												<Trash2 size={14} />
											</button>
										</div>
									{/if}
								</div>
							{/if}
						</li>
					{/each}
				</ul>
			{/if}
		{:else if activeTab === 'compare'}
			<!-- Compare tab — opens the university comparison picker with this
			     university pre-selected as Item 1 (Section 39 / Section 30). -->
			<div class="mt-3">
				<a
					href={compareHref}
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
