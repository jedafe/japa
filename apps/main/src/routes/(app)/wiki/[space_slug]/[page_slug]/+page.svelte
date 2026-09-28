<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import PostEditor from '$lib/components/create/editors/PostEditor.svelte';
	import EntityFieldsPanel from '$lib/components/wiki/EntityFieldsPanel.svelte';
	import SetSidebar from '$lib/components/shell/SetSidebar.svelte';
	import WikiTabs from '$lib/components/wiki/WikiTabs.svelte';
	import WikiSidebar from '$lib/components/wiki/WikiSidebar.svelte';
	import { searchEntities } from '$lib/api/jce';
	import type { ResolvedItem } from '$lib/types/jce';
	import { BookOpen, ChevronRight, Lock, Pencil, History, Send } from 'lucide-svelte';
	import type { WikiCategory, WikiPage } from '$lib/types';
	import type { PageData } from './$types';
	import { sanitizeContentHtml } from '$lib/utils/safeHtml';
	import { tiptapToHtml, tiptapToText, type TipTapDoc } from '$lib/tiptap/serialize';

	let { data }: { data: PageData } = $props();

	const spaceName = $derived(
		data.page?.space_name ?? data.spaceDetail?.space?.name ?? data.spaceSlug
	);
	const spaceId = $derived(data.page?.space_id ?? data.spaceDetail?.space?.id ?? '');
	const spaceLevel = $derived(data.spaceDetail?.space?.level ?? '');
	const crumbKind = $derived(
		data.wikiScope === 'university'
			? t('wiki.breadcrumb_university_guides', 'University Guides')
			: spaceLevel === 'city'
				? t('wiki.breadcrumb_city_guides', 'City Guides')
				: spaceLevel === 'state'
					? t('wiki.breadcrumb_state_guides', 'State Guides')
					: t('wiki.breadcrumb_country_guides', 'Country Guides')
	);

	function categoryLabel(cat: WikiCategory): string {
		// §80 Wiki Types: prefer the admin-set template label when the API
		// supplies it; fall back to the i18n key for unmanaged categories.
		return data.page?.category_label || t(`wiki.category_${cat}`, cat.replace(/_/g, ' '));
	}

	// Render the wiki content to safe HTML. TipTap JSON is serialized to HTML;
	// legacy HTML strings are sanitized directly (Rule 16 / §5.1).
	const htmlContent = $derived.by(() => {
		const c = data.page?.current_content;
		if (c == null) return '';
		if (typeof c === 'string') return sanitizeContentHtml(c);
		if (typeof c === 'object') {
			return sanitizeContentHtml(tiptapToHtml(c as unknown as TipTapDoc));
		}
		return '';
	});

	// Edit form state.
	let editing = $state(false);
	let editBodyHtml = $state('');
	let editBodyJson = $state<Record<string, unknown>>({ type: 'doc', content: [] });
	let summary = $state('');
	let loading = $state(false);
	let submitted = $state(false);
	let errorKey = $state('');

	async function startEdit() {
		summary = '';
		submitted = false;
		errorKey = '';
		editing = true;
		// Fetch the raw (unresolved) revision so authored {{...}} literals
		// survive a re-save — the display payload has them resolved.
		const res = await api.get<WikiPage>(
			`/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(data.pageSlug)}?raw=1`
		);
		const c = res.data?.current_content ?? data.page?.current_content;
		if (!editing) return;
		if (typeof c === 'string') {
			editBodyHtml = c;
			editBodyJson = { type: 'doc', content: [] };
		} else if (c && typeof c === 'object') {
			editBodyHtml = tiptapToHtml(c as unknown as TipTapDoc);
			editBodyJson = c as Record<string, unknown>;
		} else {
			editBodyHtml = '';
			editBodyJson = { type: 'doc', content: [] };
		}
	}

	function cancelEdit() {
		editing = false;
	}

	async function submitRevision(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!data.page) return;
		const plainText = tiptapToText(editBodyJson as unknown as TipTapDoc).trim();
		if (!plainText) {
			errorKey = 'wiki.content_required';
			return;
		}
		loading = true;
		try {
			const res = await api.post<{ id: string; status: string }>(
				`/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(data.pageSlug)}/revisions`,
				{ content: editBodyJson, summary: summary.trim() || null }
			);
			if (res.data) {
				submitted = true;
				editing = false;
			} else if (res.error) {
				// Surface the backend's structured error code (e.g. wiki.insufficient_reputation).
				errorKey = `wiki.${res.error.code}`;
			} else {
				errorKey = 'wiki.submit_failed';
			}
		} catch {
			errorKey = 'wiki.submit_failed';
		} finally {
			loading = false;
		}
	}

	const title = $derived(
		data.page ? `${data.page.title} — JapaConnect` : t('wiki.page_title_suffix')
	);
	const description = $derived.by(() => {
		if (!data.page?.current_content) return t('wiki.page_description');
		const text = tiptapToText(data.page.current_content as unknown as TipTapDoc).trim();
		if (!text) return t('wiki.page_description');
		return text.length > 160 ? text.slice(0, 157).trimEnd() + '…' : text;
	});
	const ogImage = $derived.by(() => {
		const u = data.spaceDetail?.header_url || data.spaceDetail?.logo_url || '';
		return u && !u.startsWith('http') ? page.url.origin + u : u;
	});
	const canonicalUrl = $derived(page.url.origin + page.url.pathname);
	const canEdit = $derived(
		!!data.page && !data.page.is_locked && data.authed && data.userPermissions?.includes('wiki.edit')
	);
	const wikiEnabled = $derived(data.features?.['wiki.enabled'] !== false);
	const minReputation = $derived(Number(data.siteSettings?.['wiki.min_reputation_to_edit'] ?? 100));

	// §80 community field ops — the data panel + per-entity "suggest an edit"
	// affordances only render when the structured_edits flag is on.
	const structuredEditsOn = $derived(
		wikiEnabled && data.features?.['wiki.structured_edits'] === true
	);
	// The page's structured-data target is its host space (entity_type 'space').
	const panelSpaceId = $derived(data.page?.space_id ?? data.spaceDetail?.space?.id ?? '');
	// When the page is bound to a child entity class (data_entity_type, e.g.
	// 'visa_route'), list that space's entities with per-row suggest buttons.
	const dataEntityType = $derived(data.page?.data_entity_type ?? '');

	let childEntities = $state<ResolvedItem[]>([]);
	let childEntitiesLoading = $state(false);

	$effect(() => {
		const type = dataEntityType;
		const spaceId = panelSpaceId;
		if (!structuredEditsOn || !type || !spaceId) {
			childEntities = [];
			return;
		}
		childEntitiesLoading = true;
		searchEntities(type, '', { geo_space_id: spaceId }, { pageSize: 50 })
			.then((items) => (childEntities = items))
			.catch(() => (childEntities = []))
			.finally(() => (childEntitiesLoading = false));
	});
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={canonicalUrl} />
	{#if ogImage}
		<meta property="og:image" content={ogImage} />
		<meta name="twitter:image" content={ogImage} />
		<meta name="twitter:card" content="summary_large_image" />
	{/if}
	<link rel="canonical" href={canonicalUrl} />
</svelte:head>

<SetSidebar>
	<WikiSidebar
		spaceSlug={data.spaceSlug}
		{spaceName}
		{spaceId}
		{spaceLevel}
		pages={data.index ?? []}
		currentPageSlug={data.pageSlug}
	/>
</SetSidebar>

<div class="mx-auto w-full max-w-full">
	<!-- Breadcrumb: Wiki / {kind} Guides / {space} / {page} -->
	<div class="border-b border-[var(--color-border)] px-4 py-2.5 sm:px-6">
		<nav aria-label={t('a11y.breadcrumb', 'Breadcrumb')} class="min-w-0 text-xs text-[var(--color-muted)]">
			<ol class="flex items-center gap-1.5">
				<li>
					<a href="/wiki" class="transition-colors hover:text-[var(--color-text)]">
						{t('wiki.heading', 'Wiki')}
					</a>
				</li>
				<li aria-hidden="true"><ChevronRight size={12} /></li>
				<li>{crumbKind}</li>
				<li aria-hidden="true"><ChevronRight size={12} /></li>
				<li>
					<a
						href="/wiki/{encodeURIComponent(data.spaceSlug)}"
						class="truncate transition-colors hover:text-[var(--color-text)]"
					>
						{spaceName}
					</a>
				</li>
				{#if data.page}
					<li aria-hidden="true"><ChevronRight size={12} /></li>
					<li class="truncate font-medium text-[var(--color-text)]">{data.page.title}</li>
				{/if}
			</ol>
		</nav>
	</div>

	<!-- Guide tab strip — active tab = this page's category -->
	<div class="px-1 sm:px-3">
		<WikiTabs
			spaceSlug={data.spaceSlug}
			categories={data.apiCategories ?? []}
			pages={data.index ?? []}
			active={data.page?.category ?? ''}
			currentPageSlug={data.pageSlug}
		/>
	</div>

	<div class="p-4">
		{#if !wikiEnabled && (data.loadError || !data.page)}
			<EmptyState
				icon={BookOpen}
				title={t('wiki.coming_soon', 'Wiki is not enabled for this destination')}
				body={t('wiki.coming_soon_body', 'Wiki is currently disabled. Please check back later.')}
				action={{ label: t('wiki.back_to_index', 'Back to wiki'), href: `/wiki/${data.spaceSlug}` }}
			/>
		{:else if data.loadError || !data.page}
			<EmptyState
				icon={BookOpen}
				title={t('wiki.page_not_found', 'Wiki page not found')}
				body={t('wiki.page_not_found_body', "The wiki page you're looking for doesn't exist or has been removed.")}
				action={{ label: t('wiki.back_to_index', 'Back to wiki'), href: `/wiki/${data.spaceSlug}` }}
			/>
		{:else if submitted}
			<div class="rounded-[12px] border border-green-500/30 bg-green-500/10 p-6 text-center">
				<p class="text-lg font-medium text-green-600">
					{t('wiki.revision_submitted', 'Your revision has been submitted for review.')}
				</p>
				<div class="mt-4 flex justify-center gap-3">
					<a
						href="/wiki/{data.spaceSlug}/{data.pageSlug}"
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('wiki.back_to_index', 'Back to page')}
					</a>
					<button
						type="button"
						onclick={() => (submitted = false)}
						class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
					>
						{t('wiki.edit', 'Edit again')}
					</button>
				</div>
			</div>
		{:else}
			<article class="mx-auto max-w-3xl">
				<header class="mb-4 flex flex-wrap items-center gap-2">
					<h1 class="text-2xl font-semibold text-[var(--color-text)]">{data.page.title}</h1>
					<span class="rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
						{categoryLabel(data.page.category as WikiCategory)}
					</span>
					{#if data.page.is_locked}
						<span class="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2.5 py-0.5 text-xs font-medium text-amber-600">
							<Lock size={12} /> {t('wiki.locked', 'Locked')}
						</span>
					{/if}
				</header>

				<div class="mb-6 flex flex-wrap items-center gap-3 text-sm">
					{#if canEdit}
						<button
							type="button"
							onclick={startEdit}
							class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						>
							<Pencil size={14} /> {t('wiki.edit', 'Edit')}
						</button>
					{:else if data.page.is_locked}
						<span class="text-xs text-[var(--color-muted)]">{t('wiki.locked_body', 'This page is locked and cannot be edited.')}</span>
					{/if}
					<a
						href="/wiki/{data.spaceSlug}/{data.pageSlug}/history"
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					>
						<History size={14} /> {t('wiki.history', 'History')}
					</a>
				</div>

				{#if editing}
					<form onsubmit={submitRevision} class="space-y-4 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-5" novalidate>
						<div class="flex flex-col gap-1.5">
							<span class="text-sm font-medium text-[var(--color-text)]">
								{t('wiki.content_label', 'Content')}
							</span>
							<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] focus-within:border-[var(--color-primary)]">
								<PostEditor
									html={editBodyHtml}
									onedit={() => {}}
									onhtml={(v) => (editBodyHtml = v)}
									onjson={(v) => (editBodyJson = v)}
								/>
							</div>
						</div>
						<div class="flex flex-col gap-1.5">
							<label for="wiki-summary" class="text-sm font-medium text-[var(--color-text)]">
								{t('wiki.edit_summary_label', 'Edit summary')}
							</label>
							<input
								id="wiki-summary"
								type="text"
								value={summary}
								placeholder={t('wiki.edit_summary_placeholder', 'Briefly describe your changes')}
								oninput={(e) => (summary = e.currentTarget.value)}
								class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
							/>
						</div>

						<p class="text-xs text-[var(--color-muted)]">
							{t('wiki.reputation_required', 'Minimum reputation required to edit: {min}')
								.replace('{min}', String(minReputation))}
						</p>

						{#if errorKey}
							<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
								{t(errorKey, t('wiki.submit_failed', 'Failed to submit revision. Please try again.'))}
							</p>
						{/if}

						<div class="flex justify-end gap-2 pt-1">
							<button
								type="button"
								onclick={cancelEdit}
								class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('wiki.cancel', 'Cancel')}
							</button>
							<button
								type="submit"
								disabled={loading}
								class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
							>
								<Send size={14} />
								{loading ? '...' : t('wiki.submit_revision', 'Submit revision')}
							</button>
						</div>
					</form>
				{:else}
					{#if htmlContent}
						<div class="prose max-w-none text-sm leading-relaxed text-[var(--color-text)]">
							{@html htmlContent}
						</div>
					{:else}
						<div class="rounded-[12px] border border-dashed border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-6 text-center">
							<p class="text-sm text-[var(--color-muted)]">
								{t('wiki.empty_content', 'This page has no content yet.')}
							</p>
							{#if canEdit}
								<button
									type="button"
									onclick={startEdit}
									class="mt-3 inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90"
								>
									<Pencil size={14} /> {t('wiki.contribute_cta', 'Be the first to contribute')}
								</button>
							{/if}
						</div>
					{/if}

					{#if structuredEditsOn && panelSpaceId}
						<EntityFieldsPanel
							entityType="space"
							entityId={panelSpaceId}
							groupId={data.page.field_group_id ?? undefined}
							groupSlug={data.page.field_group_slug ?? undefined}
						/>
					{/if}

					{#if structuredEditsOn && dataEntityType}
						<section
							class="mt-6 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
						>
							<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">
								{t('wiki.data_entities_title', 'Listed entries')}
							</h2>
							{#if childEntitiesLoading}
								<p class="text-sm text-[var(--color-muted)]">
									{t('common.loading', 'Loading')}…
								</p>
							{:else if childEntities.length === 0}
								<p class="text-sm text-[var(--color-muted)]">
									{t('wiki.data_entities_empty', 'No entries yet.')}
								</p>
							{:else}
								<ul class="divide-y divide-[var(--color-border)]">
									{#each childEntities as item (item.id)}
										<li class="py-2 text-sm text-[var(--color-text)]">
											{item.title}
										</li>
									{/each}
								</ul>
							{/if}
						</section>
					{/if}
				{/if}
			</article>
		{/if}
	</div>
</div>
