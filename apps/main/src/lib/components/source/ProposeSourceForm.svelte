<script lang="ts">
	// Ref: docs/superpowers/plans/2026-09-11-sources-directory-v2-plan.md
	// Public source proposal form body. Submits to POST /api/v1/sources/propose
	// and renders category/source-type selects from the backend metadata
	// (Rule 16 — no hardcoded option lists). Rendered inline by the
	// /sources/submit page and inside ProposeSourceModal.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { Loader2 } from 'lucide-svelte';
	import type { SourceMeta } from '$lib/types/sources';

	let {
		meta,
		onSubmitted
	}: {
		meta: SourceMeta;
		onSubmitted?: () => void;
	} = $props();

	let name = $state('');
	let domain = $state('');
	let sourceUrl = $state('');
	let category = $state('');
	let sourceType = $state('rss');
	let description = $state('');
	let loading = $state(false);
	let submitted = $state(false);
	let error = $state('');

	function validate(): boolean {
		error = '';
		if (!name.trim()) error = t('sources.submit.error.name_required', 'Name is required');
		else if (!sourceUrl.trim()) error = t('sources.submit.error.url_required', 'Source URL is required');
		else if (!category.trim()) error = t('sources.submit.error.category_required', 'Category is required');
		return !error;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!validate()) return;
		loading = true;
		error = '';
		try {
			const res = await api.post<{ id: string; slug: string; status: string; admin_review_url: string }>(
				'/sources/propose',
				{
					name: name.trim(),
					domain: domain.trim(),
					source_url: sourceUrl.trim(),
					category: category.trim(),
					source_type: sourceType,
					description: description.trim() || null
				}
			);
			if (res.data) {
				submitted = true;
				onSubmitted?.();
			} else {
				error = res.error?.message ?? t('sources.submit.error.failed', 'Proposal failed. Please try again.');
			}
		} catch (err) {
			error =
				err instanceof Error ? err.message : t('sources.submit.error.failed', 'Proposal failed. Please try again.');
		} finally {
			loading = false;
		}
	}

	function inputClass(): string {
		return 'w-full rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
	}
</script>

{#if submitted}
	<div class="rounded-md bg-green-100 p-4 text-sm text-green-800 dark:bg-green-900/20 dark:text-green-200">
		{t('sources.submit.success', 'Source proposed successfully. It will appear in the directory once approved.')}
	</div>
{:else}
	<form onsubmit={submit} class="flex flex-col gap-3">
		<div>
			<label class="mb-1 block text-sm font-medium" for="psf-name">
				{t('sources.submit.name', 'Source name')}
			</label>
			<input id="psf-name" name="name" bind:value={name} class={inputClass()} placeholder="Node.js Blog" disabled={loading} />
		</div>

		<div>
			<label class="mb-1 block text-sm font-medium" for="psf-source-url">
				{t('sources.submit.source_url', 'Source URL')}
			</label>
			<input
				id="psf-source-url"
				name="source_url"
				type="url"
				bind:value={sourceUrl}
				class={inputClass()}
				placeholder="https://nodejs.org/en/blog/"
				disabled={loading}
			/>
		</div>

		<div>
			<label class="mb-1 block text-sm font-medium" for="psf-domain">
				{t('sources.submit.domain', 'Domain (optional)')}
			</label>
			<input
				id="psf-domain"
				name="domain"
				bind:value={domain}
				class={inputClass()}
				placeholder="nodejs.org"
				disabled={loading}
			/>
		</div>

		<div>
			<label class="mb-1 block text-sm font-medium" for="psf-category">
				{t('sources.submit.category', 'Category')}
			</label>
			{#if meta.categories.length > 0}
				<select id="psf-category" name="category" bind:value={category} class={inputClass()} disabled={loading}>
					<option value="">{t('sources.submit.select_category', 'Select a category')}</option>
					{#each meta.categories as c}
						<option value={c}>{c}</option>
					{/each}
				</select>
			{:else}
				<input
					id="psf-category"
					name="category"
					bind:value={category}
					class={inputClass()}
					placeholder="news"
					disabled={loading}
				/>
			{/if}
		</div>

		<div>
			<label class="mb-1 block text-sm font-medium" for="psf-source-type">
				{t('sources.submit.source_type', 'Source type')}
			</label>
			<select id="psf-source-type" name="source_type" bind:value={sourceType} class={inputClass()} disabled={loading}>
				{#each meta.source_types as st}
					<option value={st}>{st}</option>
				{/each}
			</select>
		</div>

		<div>
			<label class="mb-1 block text-sm font-medium" for="psf-description">
				{t('sources.submit.description_label', 'Description')}
			</label>
			<textarea
				id="psf-description"
				name="description"
				bind:value={description}
				class={inputClass()}
				rows={3}
				placeholder="Official Node.js releases and announcements"
				disabled={loading}
			></textarea>
		</div>

		{#if error}
			<p class="text-sm text-red-600" role="alert">{error}</p>
		{/if}

		<button
			type="submit"
			disabled={loading}
			class="flex items-center justify-center gap-1.5 rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
		>
			{#if loading}
				<Loader2 size={14} class="animate-spin" />
				{t('sources.submit.submitting', 'Submitting…')}
			{:else}
				{t('sources.submit.submit', 'Propose source')}
			{/if}
		</button>
	</form>
{/if}
