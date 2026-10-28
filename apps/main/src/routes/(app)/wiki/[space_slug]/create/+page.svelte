<script lang="ts">
	// Ref: japaconnect-engineering/sections/80-destination-wiki.md
	import { beforeNavigate, goto } from '$app/navigation';
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import PostEditor from '$lib/components/create/editors/PostEditor.svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import { tiptapToText, type TipTapDoc } from '$lib/tiptap/serialize';
	import { BookOpen, Plus, Send } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import type { PageData } from './$types';
	import type { WikiCategory } from '$lib/types';

	let { data }: { data: PageData } = $props();

	const wikiEnabled = $derived(data.features?.['wiki.enabled'] !== false);
	// §80 Super-Admin Direct Publish — `*` wildcard counts as every node.
	const canCreate = $derived(
		data.authed &&
			(data.userPermissions?.includes('wiki.edit') === true ||
				data.userPermissions?.includes('*') === true)
	);
	// wiki.manage holders (super_admin + assigned groups) may author new
	// page templates inline — same permission as the admin panel route.
	const canManageTemplates = $derived(
		data.userPermissions?.includes('wiki.manage') === true ||
			data.userPermissions?.includes('*') === true
	);

	interface CatItem {
		id?: string;
		slug: WikiCategory;
		label: string;
		/** i18n key from the API — resolved through t() like the wiki tabs. */
		label_key?: string;
		/** §80 page templates — the category's admin-defined lazy pages
		 *  ({name} substituted server-side). Picking one binds the created
		 *  page to the template via matching (category, slug). */
		page_templates?: { slug: string; label: string; position: number }[];
		/** Directory categories ("States", "Universities", …) are child-space
		 *  listings, not page buckets — excluded from the create options. */
		directory?: boolean;
	}

	// §80: only page categories are selectable — directory categories
	// back tabs/listings, so a new page can never be filed under them.
	function pageCategories(items: CatItem[]): CatItem[] {
		return items.filter((c) => !c.directory);
	}

	const CUSTOM_PAGE = '__custom';

	let title = $state('');
	let slug = $state('');
	let category = $state<WikiCategory | ''>('');
	let pageChoice = $state<string>(CUSTOM_PAGE);
	let summary = $state('');
	let bodyHtml = $state('');
	let bodyJson = $state<unknown>(null);
	let categories = $state<CatItem[]>([]);
	const categoryOptions = $derived(
		categories.map((c) => ({
			value: c.slug,
			label: t(c.label_key ?? `wiki.category_${c.slug}`, c.label)
		}))
	);
	let catLoading = $state(false);
	let loading = $state(false);
	let errorKey = $state<string | null>(null);
	let fieldErrors = $state<Record<string, string>>({});
	// Dirty guard — set by real user input (title/slug/summary/editor),
	// never by programmatic template picks; cleared after a successful
	// create so the redirect doesn't trip the confirm.
	let dirty = $state(false);
	let created = $state(false);
	beforeNavigate((nav) => {
		if (
			!created &&
			dirty &&
			!confirm(t('settings.unsaved_warning', 'You have unsaved changes. Leave anyway?'))
		) {
			nav.cancel();
		}
	});

	const catTemplates = $derived(
		(categories.find((c) => c.slug === category)?.page_templates ?? [])
			.slice()
			.sort((a, b) => a.position - b.position)
	);
	// A category with declared templates offers them first; the custom
	// free-form page stays as an explicit last option.
	const pageOptions = $derived([
		...catTemplates.map((tm) => ({ value: tm.slug, label: tm.label })),
		{ value: CUSTOM_PAGE, label: t('wiki.create_custom_page', 'Custom page…') }
	]);
	const templateLocked = $derived(catTemplates.length > 0 && pageChoice !== CUSTOM_PAGE);

	// Inline template authoring (wiki.manage only) — the Page row renders
	// for templateless categories too so the "New template" affordance is
	// always reachable. Ref: §80 Inline template authoring.
	const showPageRow = $derived(catTemplates.length > 0 || !!canManageTemplates);
	let tplOpen = $state(false);
	let tplLabel = $state('');
	let tplSlug = $state('');
	let tplSlugEdited = $state(false);
	let tplSaving = $state(false);
	let tplError = $state<string | null>(null);

	// Slug auto-derives from the label with the {name} token and any
	// dangling preposition dropped — "Prices in {name}" → "prices". Still
	// editable before saving.
	function onTplLabelInput() {
		if (!tplSlugEdited) {
			tplSlug = slugify(tplLabel.replace(/\{name\}/gi, '')).replace(/-(in|of|for|at|the|a)$/, '');
		}
	}

	async function createTemplate() {
		// Capture the category — if the user switches category mid-flight the
		// template must land under the category it was created for, and the
		// follow-up selection only applies while that category is current.
		const catSlug = category;
		const catId = categories.find((c) => c.slug === catSlug)?.id;
		const label = tplLabel.trim();
		const tplSlugVal = tplSlug.trim();
		// Same charset as the backend's wikiSlugPattern — rejects leading/
		// trailing/double hyphens before the round-trip.
		if (!catId || !label || !/^[a-z0-9]+(-[a-z0-9]+)*$/.test(tplSlugVal)) {
			tplError = t('wiki.new_template_invalid', 'A label and a lowercase-hyphen slug are required.');
			return;
		}
		// Positions are 1-based in the seeded data; append after the max so
		// gaps or duplicates can't collide on an existing position.
		const tplPosition = Math.max(0, ...catTemplates.map((tm) => tm.position)) + 1;
		tplSaving = true;
		tplError = null;
		try {
			const res = await api.post<{ slug: string }>(
				`/admin/wiki/categories/${catId}/page-templates`,
				{ slug: tplSlugVal, label, position: tplPosition }
			);
			if (res.error) {
				tplError = res.error.message;
				return;
			}
			// Re-fetch so the new template arrives with {name} substituted
			// server-side — the raw label would render the token literally.
			const catRes = await api.get<CatItem[]>(`/wiki/categories?space=${encodeURIComponent(data.spaceSlug)}`);
			if (catRes.data) {
				categories = pageCategories(catRes.data);
			} else {
				// Fallback: the lazy page stub's title carries the resolved
				// {name} — push it so the locked title never stores the token.
				let resolvedLabel = label;
				const pg = await api.get<{ title?: string }>(
					`/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(tplSlugVal)}`
				);
				if (pg.data?.title) resolvedLabel = pg.data.title;
				categories = categories.map((c) =>
					c.slug === catSlug
						? { ...c, page_templates: [...(c.page_templates ?? []), { slug: tplSlugVal, label: resolvedLabel, position: tplPosition }] }
						: c
				);
			}
			tplLabel = '';
			tplSlug = '';
			tplSlugEdited = false;
			// Only auto-select the new template while its category is still
			// current — a mid-flight category switch keeps the user's form.
			if (category === catSlug) {
				tplOpen = false;
				onPageChoice(res.data?.slug ?? tplSlugVal);
			}
		} catch {
			tplError = t('wiki.new_template_failed', 'Could not create the template.');
		} finally {
			tplSaving = false;
		}
	}

	function onCategoryChange(v: WikiCategory) {
		category = v;
		tplOpen = false;
		tplError = null;
		const tpls = categories
			.find((c) => c.slug === v)
			?.page_templates?.slice()
			.sort((a, b) => a.position - b.position) ?? [];
		onPageChoice(tpls.length ? tpls[0].slug : CUSTOM_PAGE);
	}

	// Template pick locks title+slug to the template's label/slug — the
	// member fills in a predefined page, so both stay aligned with the
	// admin-defined identity ({name} already substituted in the label).
	function onPageChoice(v: string) {
		pageChoice = v;
		const tpl = catTemplates.find((tm) => tm.slug === v);
		if (tpl) {
			title = tpl.label;
			slug = tpl.slug;
		} else {
			title = '';
			slug = '';
		}
	}

	$effect(() => {
		catLoading = true;
		(async () => {
			try {
				const catRes = await api.get<CatItem[]>(`/wiki/categories?space=${encodeURIComponent(data.spaceSlug)}`);
				categories = pageCategories(catRes.data ?? []);
				if (categories.length > 0 && category === '') {
					onCategoryChange(categories[0].slug);
				}
			} catch {
				/* keep empty */
			} finally {
				catLoading = false;
			}
		})();
	});

	function slugify(value: string): string {
		return value
			.toLowerCase()
			.trim()
			.replace(/[^\w\s-]/g, '')
			.replace(/\s+/g, '-')
			.replace(/-+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function onTitleInput() {
		dirty = true;
		if (slug === '' || slug === slugify(title.slice(0, -1))) {
			slug = slugify(title);
		}
	}

	function validate(): boolean {
		fieldErrors = {};
		if (title.trim().length < 2) {
			fieldErrors.title = t('wiki.create_title_short', 'Title must be at least 2 characters.');
		}
		if (slug.length < 1 || !/^[a-z0-9-]+$/.test(slug)) {
			fieldErrors.slug = t('wiki.create_slug_invalid', 'Slug must be lowercase letters, numbers, and hyphens.');
		}
		if (category === '') {
			fieldErrors.category = t('wiki.create_category_required', 'Choose a category.');
		}
		if (tiptapToText(bodyJson as unknown as TipTapDoc | null).trim().length === 0) {
			fieldErrors.content = t('wiki.content_required', 'Content is required.');
		}
		return Object.keys(fieldErrors).length === 0;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!canCreate) return;
		if (!validate()) return;

		loading = true;
		errorKey = null;
		try {
			const res = await api.post<{
				page_id: string; revision_id: string; slug: string; status: string;
			}>('/wiki/' + encodeURIComponent(data.spaceSlug), {
				title: title.trim(),
				slug,
				category,
				content: bodyJson,
				summary: summary.trim() || undefined
			});
			if (res.error) {
				if (res.status === 403 && res.error.code === 'insufficient_reputation') {
					errorKey = 'wiki.insufficient_reputation';
				} else if (res.error.code === 'page_exists') {
					errorKey = 'wiki.create_page_exists';
				} else if (res.error.code === 'duplicate_content') {
					errorKey = 'wiki.duplicate_content';
				} else if (res.error.code === 'community_edits_disabled') {
					errorKey = 'wiki.community_edits_disabled';
				} else {
					errorKey = 'wiki.create_failed';
				}
				return;
			}
			const pageSlug = res.data?.slug ?? slug;
			created = true;
			await goto(`/wiki/${encodeURIComponent(data.spaceSlug)}/${encodeURIComponent(pageSlug)}`);
		} catch {
			errorKey = 'wiki.create_failed';
		} finally {
			loading = false;
		}
	}

	const titleText = $derived(`${t('wiki.create_title', 'Create page')} — ${data.spaceSlug}`);
</script>

<svelte:head>
	<title>{titleText}</title>
	<meta name="description" content={t('wiki.create_description', 'Create a new wiki page')} />
</svelte:head>

<div class="mx-auto w-full max-w-3xl p-4">
	<div class="mb-6 flex flex-col items-start gap-1">
		<a
			href="/wiki/{data.spaceSlug}"
			class="inline-flex items-center gap-1 text-xs text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
		>
			← {t('wiki.back_to_index', 'Back')}
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('wiki.create_title', 'Create page')} — {data.spaceSlug}
		</h1>
	</div>

	{#if !wikiEnabled}
		<EmptyState
			icon={BookOpen}
			title={t('wiki.coming_soon', 'Wiki is not enabled for this destination')}
			body={t('wiki.coming_soon_body', 'Wiki is currently disabled. Please check back later.')}
		/>
	{:else if !canCreate}
		<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-4 py-3 text-sm text-[var(--color-text-subtle)]">
			{t('wiki.create_permission', 'You must be signed in with permission to create wiki pages.')}
		</div>
	{:else}
		<form onsubmit={submit} class="space-y-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<div>
				<label for="wiki-create-category" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.create_category_label', 'Category')}
				</label>
				<AutocompleteSelect
					id="wiki-create-category"
					value={category}
					options={categoryOptions}
					disabled={catLoading}
					onValueChange={(v) => onCategoryChange(v as WikiCategory)}
				/>
				{#if fieldErrors.category}
					<p class="mt-1 text-xs text-red-500">{fieldErrors.category}</p>
				{/if}
			</div>

			{#if showPageRow}
				<div>
					<div class="mb-1 flex items-center justify-between gap-2">
						<label for="wiki-create-page" class="block text-sm font-medium text-[var(--color-text)]">
							{t('wiki.create_page_label', 'Page')}
						</label>
						{#if canManageTemplates}
							<button
								type="button"
								onclick={() => {
									tplOpen = !tplOpen;
									if (tplOpen) tplError = null;
								}}
								aria-expanded={tplOpen}
								class="inline-flex items-center gap-1 text-xs text-[var(--color-primary)] transition hover:opacity-80"
							>
								<Plus size={12} />
								{t('wiki.new_template', 'New template')}
							</button>
						{/if}
					</div>
					<AutocompleteSelect
						id="wiki-create-page"
						value={pageChoice}
						options={pageOptions}
						onValueChange={(v) => onPageChoice(v)}
					/>
					<p class="mt-1 text-xs text-[var(--color-muted)]">
						{templateLocked
							? t('wiki.create_template_hint', 'This is a predefined wiki page — its title and URL are fixed; you fill in the content for this space.')
							: t('wiki.create_custom_hint', 'A one-off page with your own title and URL slug.')}
					</p>
					{#if tplOpen && canManageTemplates}
						<div class="mt-2 space-y-2 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3">
							<p class="text-xs text-[var(--color-muted)]">
								{t('wiki.new_template_hint', 'Adds a predefined page under this category for every wiki of this type. Use {name} for the space name — e.g. "Prices in {name}".')}
							</p>
							<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
								<input
									type="text"
									bind:value={tplLabel}
									oninput={onTplLabelInput}
									placeholder={t('wiki.new_template_label', 'Template label')}
									aria-label={t('wiki.new_template_label', 'Template label')}
									class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
								/>
								<input
									type="text"
									bind:value={tplSlug}
									oninput={() => (tplSlugEdited = true)}
									placeholder={t('wiki.new_template_slug', 'template-slug')}
									aria-label={t('wiki.create_slug_label', 'URL slug')}
									class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
								/>
							</div>
							{#if tplError}
								<p class="text-xs text-red-500" role="alert">{tplError}</p>
							{/if}
							<div class="flex justify-end gap-2">
								<button
									type="button"
									onclick={() => (tplOpen = false)}
									class="rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface)]"
								>
									{t('wiki.cancel', 'Cancel')}
								</button>
								<button
									type="button"
									onclick={createTemplate}
									disabled={tplSaving}
									class="rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
								>
									{tplSaving ? '...' : t('wiki.new_template_add', 'Add template')}
								</button>
							</div>
						</div>
					{/if}
				</div>
			{/if}

			<div>
				<label for="wiki-create-title" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.create_title_label', 'Page title')}
				</label>
				<input
					id="wiki-create-title"
					type="text"
					bind:value={title}
					oninput={onTitleInput}
					disabled={templateLocked}
					placeholder={t('wiki.create_title_placeholder', 'e.g. Cost of Living')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20 disabled:cursor-not-allowed disabled:opacity-70"
				/>
				{#if fieldErrors.title}
					<p class="mt-1 text-xs text-red-500">{fieldErrors.title}</p>
				{/if}
			</div>

			<div>
				<label for="wiki-create-slug" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.create_slug_label', 'URL slug')}
				</label>
				<input
					id="wiki-create-slug"
					type="text"
					bind:value={slug}
					oninput={() => (dirty = true)}
					disabled={templateLocked}
					placeholder={t('wiki.create_slug_placeholder', 'cost-of-living')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20 disabled:cursor-not-allowed disabled:opacity-70"
				/>
				{#if fieldErrors.slug}
					<p class="mt-1 text-xs text-red-500">{fieldErrors.slug}</p>
				{/if}
			</div>

			<div>
				<label for="wiki-create-summary" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.edit_summary_label', 'Edit summary')}
				</label>
				<input
					id="wiki-create-summary"
					type="text"
					bind:value={summary}
					oninput={() => (dirty = true)}
					placeholder={t('wiki.edit_summary_placeholder', 'Briefly describe your changes')}
					class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
				/>
			</div>

			<div>
				<span class="mb-1 block text-sm font-medium text-[var(--color-text)]">
					{t('wiki.content_label', 'Content')}
				</span>
				<PostEditor
					html={bodyHtml}
					onedit={() => (dirty = true)}
					onhtml={(v) => (bodyHtml = v)}
					onjson={(v) => (bodyJson = v)}
				/>
				{#if fieldErrors.content}
					<p class="mt-1 text-xs text-red-500">{fieldErrors.content}</p>
				{/if}
			</div>

			{#if errorKey}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{t(errorKey, t('wiki.create_failed', 'Failed to create page. Please try again.'))}
				</p>
			{/if}

			<div class="flex justify-end gap-2 pt-1">
				<a
					href="/wiki/{data.spaceSlug}"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
				>
					{t('wiki.cancel', 'Cancel')}
				</a>
				<button
					type="submit"
					disabled={loading}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition hover:opacity-90 disabled:opacity-50"
				>
					<Send class="rtl:-scale-x-100" size={14} />
					{loading ? '...' : t('wiki.create_submit', 'Create page')}
				</button>
			</div>
		</form>
	{/if}
</div>
