<script lang="ts">
	import { enhance } from '$app/forms';
	import { beforeNavigate } from '$app/navigation';
	import { onMount, untrack } from 'svelte';
	import DocEditor from '$lib/components/docs/DocEditor.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import type { ManagedCategory } from '$lib/docs/catalog';
	import { t } from '$lib/i18n';
	import type { DocPage } from '$lib/types';
	import type { ExistingPage } from '$lib/server/docs-author';

	interface FormValues {
		error?: string;
		message?: string;
		saved?: boolean;
	}

	interface Props {
		page: DocPage;
		/** PATCH-allowed statuses (current + draft/in_review) — §44 amendment 2. */
		statuses: string[];
		/** Sibling pages for the parent select (all statuses, minus self). */
		existingPages: ExistingPage[];
		/** TipTap doc or rendered HTML seed for the editor. */
		editSeed: unknown;
		categories: string[];
		subcategories: string[];
		/** Managed catalog — drives category→subcategory suggestion filtering. */
		managed?: ManagedCategory[];
		form?: FormValues | null;
		/** Cancel link target (view URL without ?edit=1). */
		viewPath: string;
	}

	let {
		page,
		statuses,
		existingPages,
		editSeed,
		categories,
		subcategories,
		managed = [],
		form = null,
		viewPath
	}: Props = $props();

	let editTitle = $state(untrack(() => page.title));
	let editSlug = $state(untrack(() => page.slug));
	let editCategory = $state(untrack(() => page.category ?? ''));
	let editSubcategory = $state(untrack(() => page.subcategory ?? ''));
	let editStatus = $state(untrack(() => page.review_status));
	let editParentId = $state(untrack(() => page.parent_id ?? ''));
	let editPosition = $state(untrack(() => String(page.position)));
	let contentJson = $state<unknown>(null);
	let contentEdited = $state(false);
	let saving = $state(false);
	let reviewing = $state(false);

	// Reseed when the page identity changes (client-side nav between pages).
	let seededFor = $state('');
	$effect(() => {
		if (page.id !== seededFor) {
			seededFor = page.id;
			editTitle = page.title;
			editSlug = page.slug;
			editCategory = page.category ?? '';
			editSubcategory = page.subcategory ?? '';
			editStatus = page.review_status;
			editParentId = page.parent_id ?? '';
			editPosition = String(page.position);
			contentJson = null;
			contentEdited = false;
		}
	});

	// Subcategory suggestions follow the selected category: its managed subs
	// plus subs already used by pages in that category. No category selected
	// → the union of managed + page-derived subs (the `subcategories` prop).
	const filteredSubcategories = $derived.by(() => {
		const cat = editCategory.trim().toLowerCase();
		if (!cat) return subcategories;
		const seen = new Map<string, string>();
		const managedSubs =
			managed.find((c) => c.name.toLowerCase() === cat)?.subcategories ?? [];
		for (const s of managedSubs) if (s.trim()) seen.set(s.toLowerCase(), s);
		for (const p of existingPages) {
			if ((p.category ?? '').trim().toLowerCase() !== cat) continue;
			if (p.subcategory?.trim()) seen.set(p.subcategory.toLowerCase(), p.subcategory.trim());
		}
		return [...seen.values()];
	});

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
		existingPages.map((p) => ({
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
		existingPages.find((p) => p.id === editParentId)?.title ?? null
	);

	const dirty = $derived(
		contentEdited ||
			editTitle !== page.title ||
			editSlug !== page.slug ||
			editCategory !== (page.category ?? '') ||
			editSubcategory !== (page.subcategory ?? '') ||
			editStatus !== page.review_status ||
			editParentId !== (page.parent_id ?? '') ||
			editPosition !== String(page.position)
	);

	onMount(() => {
		const handler = (e: BeforeUnloadEvent) => {
			if (dirty && !saving) e.preventDefault();
		};
		window.addEventListener('beforeunload', handler);
		return () => window.removeEventListener('beforeunload', handler);
	});

	beforeNavigate((nav) => {
		// Guard every in-app navigation (link, goto, back/forward) — 'form'
		// is our own submit (saving/reviewing) and 'leave' is handled by the
		// beforeunload listener.
		if (dirty && !saving && !reviewing && nav.type !== 'form' && nav.type !== 'leave') {
			if (!confirm(t('edit.unsaved_confirm', 'You have unsaved changes. Leave this page?'))) {
				nav.cancel();
			}
		}
	});

	// §44 amendment 2: the dropdown can only move between draft/in_review —
	// approving and publishing are dedicated actions (they record the
	// reviewer), surfaced as one-click buttons in the review bar.
	const canApprove = $derived(page.review_status === 'draft' || page.review_status === 'in_review');
	const canPublish = $derived(page.review_status === 'approved');
	// §44 amendment 3: content edits to an approved page void the approval —
	// warn the editor before they save (status demotes to in_review).
	const demoteWarning = $derived(page.review_status === 'approved' && dirty);
	// Review actions must never run over unsaved edits: approving while dirty
	// silently discards the form's edits and stamps the *server* version —
	// not what the reviewer is looking at.
	const reviewBlocked = $derived(dirty || saving || reviewing);
</script>

{#if form?.error}
	<p class="form-error" role="alert">
		{form.message || t(`new_page.error.${form.error}`, 'Could not save the page.')}
	</p>
{/if}

<form
	method="POST"
	action="?/save"
	class="edit-form"
	use:enhance={({ formData }) => {
		formData.set('content', JSON.stringify(contentJson ?? page.content ?? {}));
		// Autocomplete fields carry no name attribute — push tracked values.
		formData.set('category', editCategory);
		formData.set('subcategory', editSubcategory);
		formData.set('parent_id', editParentId);
		saving = true;
		return async ({ update, result }) => {
			// Keep saving=true through update(): a redirect result runs goto()
			// → beforeNavigate while awaiting, and the dirty guard must let
			// the post-submit redirect through.
			try {
				await update();
			} finally {
				saving = false;
			}
			// Saved content is now canonical — a successful save (in-place or
			// via slug-rename redirect on the same page id) must clear the flag
			// or the dirty guard keeps prompting after the save landed.
			if (result.type === 'success' || result.type === 'redirect') contentEdited = false;
		};
	}}
>
	<input type="hidden" name="page_id" value={page.id} />
	<input type="hidden" name="expected_updated_at" value={page.updated_at} />

	<label class="ef-field">
		<span class="ef-label">{t('new_page.field_title', 'Title')}</span>
		<input type="text" name="title" class="ef-input ef-title" required maxlength="200" bind:value={editTitle} />
	</label>

	<div class="ef-grid">
		<div class="ef-field">
			<label class="ef-label" for="doc-edit-category">{t('new_page.field_category', 'Category')}</label>
			<Autocomplete
				id="doc-edit-category"
				placeholder={t('new_page.optional', 'Optional')}
				emptyMessage={t('new_page.no_matches', 'No matches — type to use a new value')}
				fetcher={suggestItems(categories)}
				initialFetcher={suggestInitial(categories)}
				selectedId={editCategory || null}
				selectedLabel={editCategory || null}
				freeText
				allowFreeText
				onSelect={(item) => (editCategory = item.id)}
				onInput={(v) => (editCategory = v)}
				onFreeTextBlur={(v) => (editCategory = v.trim())}
				onClear={() => (editCategory = '')}
			/>
		</div>

		<div class="ef-field">
			<label class="ef-label" for="doc-edit-subcategory">{t('new_page.field_subcategory', 'Subcategory')}</label>
			<Autocomplete
				id="doc-edit-subcategory"
				disabled={!editCategory.trim()}
				placeholder={editCategory.trim() ? t('new_page.optional', 'Optional') : t('new_page.subcategory_pick_first', 'Pick a category first')}
				emptyMessage={t('new_page.no_matches', 'No matches — type to use a new value')}
				fetcher={suggestItems(filteredSubcategories)}
				initialFetcher={suggestInitial(filteredSubcategories)}
				selectedId={editSubcategory || null}
				selectedLabel={editSubcategory || null}
				freeText
				allowFreeText
				onSelect={(item) => (editSubcategory = item.id)}
				onInput={(v) => (editSubcategory = v)}
				onFreeTextBlur={(v) => (editSubcategory = v.trim())}
				onClear={() => (editSubcategory = '')}
			/>
		</div>

		<label class="ef-field">
			<span class="ef-label">{t('new_page.field_status', 'Review status')}</span>
			<select name="review_status" class="ef-input" bind:value={editStatus}>
				{#each statuses as s (s)}
					<option value={s}>{t(`new_page.status.${s}`, s)}</option>
				{/each}
			</select>
		</label>
	</div>

	<details class="ef-advanced">
		<summary>{t('edit.advanced', 'Organization & URL')}</summary>
		<div class="ef-grid ef-advanced-grid">
			<label class="ef-field">
				<span class="ef-label">{t('new_page.field_slug', 'Slug')}</span>
				<input
					type="text"
					name="slug"
					class="ef-input ef-mono"
					pattern="[a-z0-9]+(-[a-z0-9]+)*"
					bind:value={editSlug}
				/>
				<span class="ef-hint">{t('edit.slug_hint', 'Renaming the slug changes the page URL.')}</span>
			</label>
			<div class="ef-field">
				<label class="ef-label" for="doc-edit-parent">{t('new_page.field_parent', 'Parent page')}</label>
				<Autocomplete
					id="doc-edit-parent"
					placeholder={t('new_page.no_parent', '— Top level —')}
					emptyMessage={t('new_page.no_parent_matches', 'No matching pages')}
					fetcher={parentFetcher}
					initialFetcher={() => Promise.resolve(parentItems)}
					selectedId={editParentId || null}
					selectedLabel={parentLabel}
					onSelect={(item) => (editParentId = item.id)}
					onClear={() => (editParentId = '')}
				/>
			</div>
			<label class="ef-field">
				<span class="ef-label">{t('new_page.field_position', 'Position')}</span>
				<input type="number" name="position" class="ef-input" min="0" step="1" bind:value={editPosition} />
			</label>
		</div>
	</details>

	<DocEditor
		initialContent={editSeed}
		bind:value={contentJson}
		onUpdate={() => (contentEdited = true)}
		placeholder={t('new_page.content_placeholder', 'Start writing the documentation…')}
	/>

	<div class="ef-actions">
		<button type="submit" class="ef-save" disabled={saving || reviewing}>
			{saving ? t('edit.saving', 'Saving…') : t('edit.save', 'Save changes')}
		</button>
		<a href={viewPath} class="ef-cancel">{t('editor.cancel', 'Cancel')}</a>
		{#if demoteWarning}
			<span class="ef-demote-hint">
				{t('edit.approved_demote_hint', 'Saving edits will return this page to review — its approval will be cleared.')}
			</span>
		{/if}
	</div>
</form>

<!-- Review workflow actions — the dedicated endpoints own approved/published
     so reviewer metadata stays truthful (§44 amendment 2). -->
{#if canApprove || canPublish}
	<div class="review-bar">
		{#if canApprove}
			<form
				method="POST"
				action="?/approve"
				use:enhance={() => {
					reviewing = true;
					return async ({ update }) => {
						try {
							await update();
						} finally {
							reviewing = false;
						}
					};
				}}
			>
				<input type="hidden" name="page_id" value={page.id} />
				<button type="submit" class="review-btn approve" disabled={reviewBlocked}>
					{t('edit.approve', 'Approve')}
				</button>
			</form>
		{/if}
		{#if canPublish}
			<form
				method="POST"
				action="?/publish"
				use:enhance={() => {
					reviewing = true;
					return async ({ update }) => {
						try {
							await update();
						} finally {
							reviewing = false;
						}
					};
				}}
			>
				<input type="hidden" name="page_id" value={page.id} />
				<button type="submit" class="review-btn publish" disabled={reviewBlocked}>
					{t('edit.publish', 'Publish')}
				</button>
			</form>
		{/if}
		<span class="review-hint">
			{dirty
				? t('edit.review_dirty_hint', 'Save your edits before approving or publishing — review applies to the saved version.')
				: t('edit.review_hint', 'Approve marks the page reviewed; publish makes it visible to readers.')}
		</span>
	</div>
{/if}

<style>
	.edit-form { display: flex; flex-direction: column; gap: 1rem; }
	.ef-field { display: flex; flex-direction: column; gap: 0.375rem; min-width: 0; }
	.ef-label {
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.05em;
		color: var(--color-text-subtle);
	}
	.ef-input {
		padding: 0.5rem 0.75rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius);
		background: var(--color-surface);
		color: var(--color-text);
		font-size: 0.875rem;
		font-family: var(--font-sans);
	}
	.ef-input:focus { outline: none; border-color: var(--color-primary); }
	.ef-title { font-size: 1.25rem; font-weight: 600; }
	.ef-mono { font-family: var(--font-mono); font-size: 0.8125rem; }
	.ef-hint { font-size: 0.75rem; color: var(--color-muted); }
	.ef-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 0.875rem;
	}
	.ef-advanced {
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface);
	}
	.ef-advanced > summary {
		padding: 0.625rem 1rem;
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--color-text-subtle);
		cursor: pointer;
		list-style: none;
	}
	.ef-advanced > summary::-webkit-details-marker { display: none; }
	.ef-advanced[open] > summary { border-bottom: 1px solid var(--color-border); }
	.ef-advanced-grid { padding: 1rem; }
	.ef-actions {
		display: flex;
		align-items: center;
		gap: 1rem;
		margin-top: 0.25rem;
	}
	.ef-save {
		padding: 0.625rem 1.5rem;
		border: none;
		border-radius: var(--radius);
		background: var(--color-primary);
		color: var(--color-primary-contrast);
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
	}
	.ef-save:hover { filter: brightness(1.05); }
	.ef-save:disabled { opacity: 0.6; cursor: default; }
	.ef-cancel {
		font-size: 0.875rem;
		color: var(--color-muted);
		text-decoration: none;
	}
	.ef-cancel:hover { color: var(--color-text); }
	.ef-demote-hint {
		font-size: 0.75rem;
		color: var(--method-patch);
		font-weight: 500;
	}
	.form-error {
		margin: 0 0 0.25rem;
		padding: 0.75rem 1rem;
		border-radius: var(--radius);
		border: 1px solid color-mix(in srgb, var(--color-danger, #dc2626) 40%, var(--color-border));
		background: color-mix(in srgb, var(--color-danger, #dc2626) 8%, transparent);
		color: var(--color-danger, #dc2626);
		font-size: 0.875rem;
	}
	.review-bar {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
		padding: 0.75rem 1rem;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-card);
		background: var(--color-surface-subtle);
	}
	.review-btn {
		padding: 0.4375rem 1rem;
		border: none;
		border-radius: var(--radius);
		font-size: 0.8125rem;
		font-weight: 600;
		cursor: pointer;
	}
	.review-btn.approve { background: var(--method-patch); color: #fff; }
	.review-btn.publish { background: var(--method-get, #16a34a); color: #fff; }
	.review-btn:disabled { opacity: 0.6; cursor: default; }
	.review-hint { font-size: 0.75rem; color: var(--color-muted); }
	@media (max-width: 640px) {
		.ef-grid { grid-template-columns: 1fr; }
		.review-bar { flex-wrap: wrap; }
	}
</style>
