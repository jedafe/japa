<script lang="ts">
	// Ref: docs/superpowers/plans/2026-09-11-sources-directory-v2-plan.md
	// Public source proposal form. Submits to POST /api/v1/sources/propose and
	// renders category/source-type selects from the backend metadata endpoint
	// (Rule 16 — no hardcoded option lists).
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let name = $state('');
	let domain = $state('');
	let sourceUrl = $state('');
	let category = $state('');
	let sourceType = $state('rss');
	let description = $state('');
	let loading = $state(false);
	let submitted = $state(false);
	let error = $state('');

	const meta = $derived(data.meta);
	const title = $derived(t('sources.submit.title', 'Propose a source'));

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
			} else {
				error = t('sources.submit.error.failed', 'Proposal failed. Please try again.');
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

<svelte:head>
	<title>{title}</title>
</svelte:head>

<div class="mx-auto max-w-xl p-4">
	<h1 class="text-xl font-bold">{title}</h1>
	<p class="text-sm text-[var(--color-text-muted)]">
		{t('sources.submit.description', 'Suggest a new source for the directory. Proposals are reviewed by admins before going live.')}
	</p>

	{#if submitted}
		<div class="mt-4 rounded-md bg-green-100 p-4 text-sm text-green-800 dark:bg-green-900/20 dark:text-green-200">
			{t('sources.submit.success', 'Source proposed successfully. It will appear in the directory once approved.')}
		</div>
	{:else}
		<form onsubmit={submit} class="mt-4 flex flex-col gap-3">
			<div>
				<label class="mb-1 block text-sm font-medium" for="name">
					{t('sources.submit.name', 'Source name')}
				</label>
				<input id="name" name="name" bind:value={name} class={inputClass()} placeholder="Node.js Blog" />
			</div>

			<div>
				<label class="mb-1 block text-sm font-medium" for="source_url">
					{t('sources.submit.source_url', 'Source URL')}
				</label>
				<input
					id="source_url"
					name="source_url"
					type="url"
					bind:value={sourceUrl}
					class={inputClass()}
					placeholder="https://nodejs.org/en/blog/"
				/>
			</div>

			<div>
				<label class="mb-1 block text-sm font-medium" for="domain">
					{t('sources.submit.domain', 'Domain (optional)')}
				</label>
				<input
					id="domain"
					name="domain"
					bind:value={domain}
					class={inputClass()}
					placeholder="nodejs.org"
				/>
			</div>

			<div>
				<label class="mb-1 block text-sm font-medium" for="category">
					{t('sources.submit.category', 'Category')}
				</label>
				{#if meta.categories.length > 0}
					<select id="category" name="category" bind:value={category} class={inputClass()}>
						<option value="">{t('sources.submit.select_category', 'Select a category')}</option>
						{#each meta.categories as c}
							<option value={c}>{c}</option>
						{/each}
					</select>
				{:else}
					<input
						id="category"
						name="category"
						bind:value={category}
						class={inputClass()}
						placeholder="news"
					/>
				{/if}
			</div>

			<div>
				<label class="mb-1 block text-sm font-medium" for="source_type">
					{t('sources.submit.source_type', 'Source type')}
				</label>
				<select id="source_type" name="source_type" bind:value={sourceType} class={inputClass()}>
					{#each meta.source_types as st}
						<option value={st}>{st}</option>
					{/each}
				</select>
			</div>

			<div>
				<label class="mb-1 block text-sm font-medium" for="description">
					{t('sources.submit.description', 'Description')}
				</label>
				<textarea
					id="description"
					name="description"
					bind:value={description}
					class={inputClass()}
					rows={4}
					placeholder="Official Node.js releases and announcements"
				></textarea>
			</div>

			{#if error}
				<p class="text-sm text-red-600">{error}</p>
			{/if}

			<button
				type="submit"
				disabled={loading}
				class="rounded-md bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
			>
				{loading ? t('sources.submit.submitting', 'Submitting…') : t('sources.submit.submit', 'Propose source')}
			</button>
		</form>
	{/if}
</div>
