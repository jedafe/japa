<script lang="ts">
	import { untrack } from 'svelte';
	import { enhance } from '$app/forms';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import DocEditor from '$lib/components/docs/DocEditor.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import { AUDIENCE_ICON } from '$lib/audience-meta';
	import { ChevronRight } from 'lucide-svelte';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	let collapsed = $state(untrack(() => data.navCollapsed));

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	const AudienceIcon = $derived(AUDIENCE_ICON[data.audience]);

	// Form state — seeded from `form` when the action fails so input survives.
	let title = $state(untrack(() => form?.title ?? ''));
	let slug = $state(untrack(() => form?.slug ?? ''));
	let slugTouched = $state(false);
	let category = $state(untrack(() => form?.category ?? ''));
	let subcategory = $state(untrack(() => form?.subcategory ?? ''));
	let parentId = $state(untrack(() => form?.parentId ?? ''));
	let reviewStatus = $state(untrack(() => form?.reviewStatus ?? 'draft'));
	let position = $state(untrack(() => String(data.nextPosition)));
	let contentJson = $state<unknown>(null);
	let submitting = $state(false);

	function slugify(s: string): string {
		return s
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

	// Datalist suggestions from existing pages in this audience (API-sourced).
	const existingCategories = $derived(
		[...new Set(data.existingPages.map((p) => p.category).filter((c): c is string => !!c))]
	);
	const existingSubcategories = $derived(
		[
			...new Set(
				data.existingPages
					.filter((p) => !category || p.category === category)
					.map((p) => p.subcategory)
					.filter((c): c is string => !!c)
			)
		]
	);
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
		<nav class="breadcrumb" aria-label="Breadcrumb">
			<a href="/docs" class="crumb">{t('audience.overview')}</a>
			<ChevronRight size={12} class="crumb-sep" />
			<a href="/docs/{data.audience}" class="crumb">{t(data.labelKey, data.audience)}</a>
			<ChevronRight size={12} class="crumb-sep" />
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
				submitting = true;
				return async ({ update }) => {
					submitting = false;
					await update();
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

				<label class="field">
					<span class="field-label">{t('new_page.field_category', 'Category')}</span>
					<input
						type="text"
						name="category"
						class="field-input"
						list="doc-categories"
						maxlength="120"
						bind:value={category}
						placeholder={t('new_page.category_placeholder', 'e.g. Guides')}
					/>
					<datalist id="doc-categories">
						{#each existingCategories as c (c)}
							<option value={c}></option>
						{/each}
					</datalist>
				</label>

				<label class="field">
					<span class="field-label">{t('new_page.field_subcategory', 'Subcategory')}</span>
					<input
						type="text"
						name="subcategory"
						class="field-input"
						list="doc-subcategories"
						maxlength="120"
						bind:value={subcategory}
						placeholder={t('new_page.optional', 'Optional')}
					/>
					<datalist id="doc-subcategories">
						{#each existingSubcategories as s (s)}
							<option value={s}></option>
						{/each}
					</datalist>
				</label>

				<label class="field">
					<span class="field-label">{t('new_page.field_parent', 'Parent page')}</span>
					<select name="parent_id" class="field-input" bind:value={parentId}>
						<option value="">{t('new_page.no_parent', '— Top level —')}</option>
						{#each data.existingPages as p (p.id)}
							<option value={p.id}>{p.title}</option>
						{/each}
					</select>
				</label>

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
						{#each data.statuses as s (s)}
							<option value={s}>{t(`new_page.status.${s}`, s)}</option>
						{/each}
					</select>
					<span class="field-hint">{t('new_page.status_hint', 'Only published pages are visible to readers.')}</span>
				</label>
			</div>

			<div class="field field-wide">
				<span class="field-label">{t('new_page.field_content', 'Content')}</span>
				<DocEditor
					bind:value={contentJson}
					placeholder={t('new_page.content_placeholder', 'Start writing the documentation…')}
				/>
			</div>

			<div class="form-actions">
				<button type="submit" class="submit-btn" disabled={submitting}>
					{submitting ? t('new_page.saving', 'Creating…') : t('new_page.submit', 'Create page')}
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
		font-family: var(--font-sans);
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
