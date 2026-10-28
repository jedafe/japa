<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { enhance } from '$app/forms';
	import { beforeNavigate } from '$app/navigation';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import DocEditor from '$lib/components/docs/DocEditor.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import { parseManagedCategories } from '$lib/docs/catalog';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { AUDIENCE_ICON } from '$lib/audience-meta';
	import { ChevronRight } from 'lucide-svelte';
	import type { DocAudience, DocNavTree, Menu, SessionUser } from '$lib/types';
	import type { ExistingPage } from '$lib/server/docs-author';
	import type { SeoMeta } from '$lib/seo/meta';

	interface FormValues {
		title?: string;
		slug?: string;
		category?: string;
		subcategory?: string;
		parentId?: string;
		reviewStatus?: string;
		slugTouched?: boolean;
		error?: string;
		message?: string;
	}

	interface Props {
		data: {
			audience: DocAudience;
			labelKey: string;
			audienceLabel: string;
			statuses: string[];
			existingPages: ExistingPage[];
			nextPosition: number;
			meta: SeoMeta;
			nav: DocNavTree[];
			navCollapsed: boolean;
			accessibleAudiences: DocAudience[];
			user: SessionUser | null;
			siteSettings?: Record<string, unknown>;
			footerMenu?: Menu | null;
			rightSidebarWidgets?: unknown[];
			leftSidebarWidgets?: unknown[];
		};
		form: FormValues | null;
	}

	let { data, form }: Props = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	const AudienceIcon = $derived(AUDIENCE_ICON[data.audience]);

	// Form state — seeded from `form` when the action fails so input survives.
	let title = $state(untrack(() => form?.title ?? ''));
	let slug = $state(untrack(() => form?.slug ?? ''));
	// slug_touched round-trips through a hidden input so a failed submit keeps
	// a hand-edited slug instead of silently auto-regenerating it (§44 fix).
	let slugTouched = $state(untrack(() => form?.slugTouched ?? false));
	let category = $state(untrack(() => form?.category ?? ''));
	let subcategory = $state(untrack(() => form?.subcategory ?? ''));
	let parentId = $state(untrack(() => form?.parentId ?? ''));
	let reviewStatus = $state(untrack(() => form?.reviewStatus ?? 'draft'));
	let position = $state(untrack(() => String(data.nextPosition)));
	let contentJson = $state<unknown>(null);
	let submitting = $state(false);
	let contentEdited = $state(false);

	// §44 amendment 2: create writes only draft/in_review directly — the
	// server action chains approve/publish via the dedicated endpoints when
	// an author picks them here, so reviewer metadata stays truthful.
	const statusChoices = $derived(
		data.statuses.filter((s) =>
			['draft', 'in_review', 'approved', 'published'].includes(s)
		)
	);

	function slugify(s: string): string {
		return s
			.normalize('NFD')
			.replace(/[̀-ͯ]/g, '') // fold diacritics — "Guides d'étude" → "guides-detude"
			.toLowerCase()
			.trim()
			.replace(/[^a-z0-9\s-]/g, '')
			.replace(/[\s_]+/g, '-')
			.replace(/-+/g, '-')
			.replace(/^-|-$/g, '');
	}

	function onTitleInput() {
		if (!slugTouched) slug = slugify(title);
	}

	// Category/subcategory suggestions — managed catalog (docs.categories)
	// merged with the taxonomy already in use on this audience's pages.
	// Fields stay free-text; these are suggestions only. Compared
	// case-insensitively so "Guides"/"guides" offer one suggestion.
	const managedCategories = $derived(
		parseManagedCategories(data.siteSettings?.['docs.categories'])
	);
	const categorySuggestions = $derived(
		[
			...new Map(
				[
					...managedCategories.map((c) => c.name),
					...data.existingPages.map((p) => p.category?.trim())
				]
					.filter((c): c is string => !!c)
					.map((c) => [c.toLowerCase(), c])
			).values()
		]
	);
	const subcategorySuggestions = $derived(
		[
			...new Map(
				[
					...(category.trim()
						? (managedCategories.find(
								(c) => c.name.toLowerCase() === category.trim().toLowerCase()
							)?.subcategories ?? [])
						: managedCategories.flatMap((c) => c.subcategories)),
					...data.existingPages
						.filter(
							(p) =>
								!category.trim() ||
								(p.category ?? '').trim().toLowerCase() ===
									category.trim().toLowerCase()
						)
						.map((p) => p.subcategory?.trim())
				]
					.filter((c): c is string => !!c)
					.map((c) => [c.toLowerCase(), c])
			).values()
		]
	);

	const toItems = (list: string[]): AutocompleteItem[] =>
		list.map((s) => ({ id: s, label: s }));
	const suggestItems =
		(list: string[]) =>
		(q: string): Promise<AutocompleteItem[]> =>
			Promise.resolve(
				toItems(list.filter((s) => s.toLowerCase().includes(q.toLowerCase())))
			);
	const suggestInitial =
		(list: string[]) =>
		(): Promise<AutocompleteItem[]> =>
			Promise.resolve(toItems(list));

	const parentItems = $derived(
		data.existingPages.map((p) => ({
			id: p.id,
			label: p.title,
			sublabel:
				p.slug +
				(p.review_status !== 'published'
					? ` · ${t(`new_page.status.${p.review_status}`, p.review_status)}`
					: '')
		}))
	);
	const parentFetcher = (q: string): Promise<AutocompleteItem[]> =>
		Promise.resolve(
			parentItems.filter((p) => p.label.toLowerCase().includes(q.toLowerCase()))
		);
	const parentLabel = $derived(
		data.existingPages.find((p) => p.id === parentId)?.title ?? null
	);

	// Unsaved-changes guard: dirty once any field diverges from its seed.
	const dirty = $derived(
		contentEdited ||
			title !== '' ||
			slugTouched ||
			category !== '' ||
			subcategory !== '' ||
			parentId !== '' ||
			reviewStatus !== 'draft' ||
			position !== String(data.nextPosition) ||
			Boolean(form?.error)
	);

	onMount(() => {
		const handler = (e: BeforeUnloadEvent) => {
			if (dirty && !submitting) e.preventDefault();
		};
		window.addEventListener('beforeunload', handler);
		return () => window.removeEventListener('beforeunload', handler);
	});

	beforeNavigate((nav) => {
		// Cover back/forward (popstate) and goto too — 'form' is our own
		// submit (submitting) and 'leave' is handled by beforeunload.
		if (dirty && !submitting && nav.type !== 'form' && nav.type !== 'leave') {
			if (!confirm(t('edit.unsaved_confirm', 'You have unsaved changes. Leave this page?'))) {
				nav.cancel();
			}
		}
	});
</script>

<svelte:head>
	<title>{data.meta.title}</title>
	<meta name="description" content={data.meta.description} />
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	logoUrl={(data.siteSettings?.["appearance.logo_url"] as string | undefined) ?? ""}
	footerMenu={data.footerMenu}
	rightSidebarWidgets={data.rightSidebarWidgets}
	leftSidebarWidgets={data.leftSidebarWidgets}
	activeAudience={data.audience}
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	<section class="new-page">
		<nav class="breadcrumb" aria-label={t('a11y.breadcrumb', 'Breadcrumb')}>
			<a href="/docs" class="crumb">{t('audience.overview')}</a>
			<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
			<a href="/docs/{data.audience}" class="crumb">{t(data.labelKey, data.audience)}</a>
			<ChevronRight size={12} class="crumb-sep rtl:-scale-x-100" />
			<span class="crumb-current">{t('new_page.title', 'New page')}</span>
		</nav>

		<header class="page-header">
			<span class="audience-icon"><AudienceIcon size={22} /></span>
			<div>
				<h1>{t('new_page.heading', 'New {audience} page', { audience: data.audienceLabel })}</h1>
				<p class="page-desc">{t('new_page.description', 'Create a new documentation page.')}</p>
			</div>
		</header>

		{#if form?.error}
			<p class="form-error" role="alert">
				{form.message || t(`new_page.error.${form.error}`, 'Could not create the page.')}
			</p>
		{/if}

		<form
			method="POST"
			action="?/create"
			class="new-form"
			use:enhance={({ formData }) => {
				formData.set('content', JSON.stringify(contentJson ?? {}));
				formData.set('slug_touched', slugTouched ? '1' : '');
				// Autocomplete fields carry no name attribute — push the tracked
				// values explicitly.
				formData.set('category', category);
				formData.set('subcategory', subcategory);
				formData.set('parent_id', parentId);
				submitting = true;
				return async ({ update }) => {
					// Keep submitting=true through update(): a redirect result runs
					// goto() → beforeNavigate while awaiting, and the dirty guard
					// must let the post-submit redirect through.
					try {
						await update();
					} finally {
						submitting = false;
					}
				};
			}}
		>
			<div class="field-grid">
				<label class="field field-wide">
					<span class="field-label">{t('new_page.field_title', 'Title')}</span>
					<input
						type="text"
						name="title"
						class="field-input"
						required
						maxlength="200"
						bind:value={title}
						oninput={onTitleInput}
						placeholder={t('new_page.title_placeholder', 'e.g. Getting started')}
					/>
				</label>

				<label class="field field-wide">
					<span class="field-label">{t('new_page.field_slug', 'Slug')}</span>
					<input
						type="text"
						name="slug"
						class="field-input field-mono"
						required
						pattern="[a-z0-9]+(-[a-z0-9]+)*"
						bind:value={slug}
						oninput={() => (slugTouched = true)}
						placeholder="getting-started"
					/>
					<span class="field-hint">/docs/{data.audience}/<strong>{slug || '…'}</strong></span>
				</label>

				<div class="field">
					<label class="field-label" for="doc-category">{t('new_page.field_category', 'Category')}</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('new_page.category_hint', 'The top-level section this page lives in.')}</p>
					<Autocomplete
						id="doc-category"
						placeholder={t('new_page.category_placeholder', 'e.g. Guides')}
						emptyMessage={t('new_page.no_matches', 'No matches — type to use a new value')}
						fetcher={suggestItems(categorySuggestions)}
						initialFetcher={suggestInitial(categorySuggestions)}
						selectedId={category || null}
						selectedLabel={category || null}
						freeText
						allowFreeText
						onSelect={(item) => (category = item.id)}
						onInput={(v) => (category = v)}
						onFreeTextBlur={(v) => (category = v.trim())}
						onClear={() => (category = '')}
					/>
				</div>

				<div class="field">
					<label class="field-label" for="doc-subcategory">{t('new_page.field_subcategory', 'Subcategory')}</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('new_page.subcategory_hint', 'Optional — a nested grouping under the category.')}</p>
					<Autocomplete
						id="doc-subcategory"
						disabled={!category.trim()}
						placeholder={category.trim() ? t('new_page.optional', 'Optional') : t('new_page.subcategory_pick_first', 'Pick a category first')}
						emptyMessage={t('new_page.no_matches', 'No matches — type to use a new value')}
						fetcher={suggestItems(subcategorySuggestions)}
						initialFetcher={suggestInitial(subcategorySuggestions)}
						selectedId={subcategory || null}
						selectedLabel={subcategory || null}
						freeText
						allowFreeText
						onSelect={(item) => (subcategory = item.id)}
						onInput={(v) => (subcategory = v)}
						onFreeTextBlur={(v) => (subcategory = v.trim())}
						onClear={() => (subcategory = '')}
					/>
				</div>

				<div class="field">
					<label class="field-label" for="doc-parent">{t('new_page.field_parent', 'Parent page')}</label>
					<p class="mt-1 text-xs text-[var(--color-muted)]">{t('new_page.parent_hint', 'Optional — nest this page under another.')}</p>
					<Autocomplete
						id="doc-parent"
						placeholder={t('new_page.no_parent', '— Top level —')}
						emptyMessage={t('new_page.no_parent_matches', 'No matching pages')}
						fetcher={parentFetcher}
						initialFetcher={() => Promise.resolve(parentItems)}
						selectedId={parentId || null}
						selectedLabel={parentLabel}
						onSelect={(item) => (parentId = item.id)}
						onClear={() => (parentId = '')}
					/>
					<span class="field-hint">
						{t('new_page.parent_hint', 'A non-published parent hides this page from the nav until it publishes.')}
					</span>
				</div>

				<label class="field">
					<span class="field-label">{t('new_page.field_position', 'Position')}</span>
					<input
						type="number"
						name="position"
						class="field-input"
						min="0"
						step="1"
						bind:value={position}
					/>
				</label>

				<label class="field">
					<span class="field-label">{t('new_page.field_status', 'Review status')}</span>
					<select name="review_status" class="field-input" bind:value={reviewStatus}>
						{#each statusChoices as s (s)}
							<option value={s}>{t(`new_page.status.${s}`, s)}</option>
						{/each}
					</select>
					<span class="field-hint">
						{t('new_page.status_hint', 'Draft stays private. In review queues it for a reviewer. Approve/publish run the review steps for you.')}
					</span>
				</label>
			</div>

			<div class="field field-wide">
				<span class="field-label">{t('new_page.field_content', 'Content')}</span>
				<DocEditor
					bind:value={contentJson}
					onUpdate={() => (contentEdited = true)}
					placeholder={t('new_page.content_placeholder', 'Start writing the documentation…')}
				/>
			</div>

			<div class="form-actions">
				<button type="submit" class="submit-btn" disabled={submitting}>
					{submitting
						? t('new_page.saving', 'Creating…')
						: reviewStatus === 'in_review'
							? t('new_page.submit_review', 'Create & submit for review')
							: reviewStatus === 'approved'
								? t('new_page.submit_approve', 'Create & approve')
								: reviewStatus === 'published'
									? t('new_page.submit_publish', 'Create & publish')
									: t('new_page.submit', 'Create page')}
				</button>
				<a href="/docs/{data.audience}" class="cancel-link">{t('editor.cancel', 'Cancel')}</a>
			</div>
		</form>
	</section>
</AppShell>

<style>
	.new-page {
		max-width: 860px;
		margin: 0 auto;
		padding: 2.5rem 1.5rem 4rem;
	}
	.breadcrumb {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.8125rem;
		margin-bottom: 1.5rem;
	}
	.crumb { color: var(--color-muted); text-decoration: none; }
	.crumb:hover { color: var(--color-primary); }
	.breadcrumb :global(.crumb-sep) { color: var(--color-muted); flex-shrink: 0; }
	.crumb-current { color: var(--color-text-subtle); font-weight: 500; }

	.page-header {
		display: flex;
		align-items: flex-start;
		gap: 0.875rem;
		margin-bottom: 1.75rem;
	}
	.audience-icon {
		display: grid;
		place-items: center;
		height: 2.75rem;
		width: 2.75rem;
		border-radius: var(--radius-card);
		background: color-mix(in srgb, var(--color-primary) 10%, transparent);
		color: var(--color-primary);
		flex-shrink: 0;
	}
	.page-header h1 {
		font-family: var(--font-heading, var(--font-sans));
		font-size: 1.5rem;
		font-weight: 700;
		color: var(--color-text);
		margin: 0;
		letter-spacing: -0.02em;
	}
	.page-desc {
		font-size: 0.875rem;
		color: var(--color-text-subtle);
		margin: 0.25rem 0 0;
	}

	.form-error {
		margin: 0 0 1.25rem;
		padding: 0.75rem 1rem;
		border-radius: var(--radius);
		border: 1px solid color-mix(in srgb, var(--color-danger, #dc2626) 40%, var(--color-border));
		background: color-mix(in srgb, var(--color-danger, #dc2626) 8%, transparent);
		color: var(--color-danger, #dc2626);
		font-size: 0.875rem;
	}

	.field-grid {
		display: grid;
		grid-template-columns: repeat(2, 1fr);
		gap: 1rem;
		margin-bottom: 1.25rem;
	}
	.field { display: flex; flex-direction: column; gap: 0.375rem; min-width: 0; }
	.field-wide { grid-column: 1 / -1; }
	.field-label {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-subtle);
	}
	.field-input {
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		font-family: var(--font-sans);
	}
	.field-input:focus { outline: none; border-color: var(--color-primary); }
	.field-mono { font-family: var(--font-mono); font-size: 0.8125rem; }
	.field-hint {
		font-size: 0.75rem;
		color: var(--color-muted);
	}
	.field-hint strong { font-weight: 600; color: var(--color-text-subtle); }

	.new-form .field-wide { margin-bottom: 1.25rem; }

	.form-actions {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-top: 1.5rem;
	}
	.submit-btn {
		padding: 0.625rem 1.5rem;
		border: none;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
	}
	.submit-btn:hover { filter: brightness(1.05); }
	.submit-btn:disabled { opacity: 0.6; cursor: default; }
	.cancel-link {
		font-size: 0.875rem;
		color: var(--color-muted);
		text-decoration: none;
	}
	.cancel-link:hover { color: var(--color-text); }

	@media (max-width: 640px) {
		.field-grid { grid-template-columns: 1fr; }
	}
</style>
