<script lang="ts">
	import HintIcon from '$lib/components/ui/HintIcon.svelte';
	import { t } from '$lib/i18n';
	import { untrack } from 'svelte';
	import { Download, FileUp, CheckCircle2, AlertCircle, Loader2 } from 'lucide-svelte';
	import { EmptyState, RelativeTime } from '$lib/components/ui';
	import { enhance } from '$app/forms';
	import { invalidate } from '$app/navigation';
	import type { ContentImport, ImportSource } from '$lib/types';
	import type { PageData } from './$types';
	import Autocomplete from '$lib/components/create/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';

	let { data }: { data: PageData } = $props();

	const imports = $derived<ContentImport[]>(data.imports);
	const sources = $derived<ImportSource[]>(data.sources);

	let sourceType = $state(untrack(() => data.sources[0]?.value ?? 'medium'));
	let sourceUrl = $state('');
	let result = $state<{ ok?: boolean; error?: string; import?: ContentImport } | null>(null);
	let submitting = $state(false);

	// Whether the currently selected source type requires a URL. Drives the
	// conditional URL input. Falls back to true when sources haven't loaded.
	const currentSource = $derived(sources.find((s) => s.value === sourceType));
	const requiresUrl = $derived(currentSource?.requires_url ?? true);

	const statusColor = (status: string): string => {
		switch (status) {
			case 'completed': return 'text-[var(--color-success)]';
			case 'failed': return 'text-[var(--color-danger)]';
			case 'processing': return 'text-[var(--color-info)]';
			default: return 'text-[var(--color-muted)]';
		}
	};

	// Autocomplete fetchers — filter the already-loaded import sources client-side.
	function fetchSources(q: string): Promise<AutocompleteItem[]> {
		const query = q.toLowerCase();
		const filtered = query === ''
			? sources
			: sources.filter((s) => s.label.toLowerCase().includes(query) || s.value.toLowerCase().includes(query));
		return Promise.resolve(filtered.map((s) => ({ id: s.value, label: s.label })));
	}
	function fetchSourcesInitial(): Promise<AutocompleteItem[]> {
		return fetchSources('');
	}
	const selectedSourceLabel = $derived(
		sourceType ? sources.find((s) => s.value === sourceType)?.label ?? null : null
	);
</script>

<svelte:head>
	<title>{t('import.title', 'Import Content — {site}')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<Download size={20} /> {t('import.heading', 'Import Content')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-muted)]">
			{t('import.description', 'Import your existing content from Medium, Substack, RSS, or a ZIP file.')}
		</p>
	</div>

	<div class="p-4">
		<!-- Import form -->
		<div class="mb-6 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
			<h2 class="mb-3 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
				<FileUp size={16} /> {t('import.start', 'Start a new import')}
			</h2>

			{#if result?.ok}
				<div class="mb-3 flex items-center gap-2 text-sm text-[var(--color-success)]">
					<CheckCircle2 size={16} /> {t('import.started', 'Import started. We will process it in the background.')}
				</div>
			{:else if result?.error}
				<div class="mb-3 flex items-center gap-2 text-sm text-[var(--color-danger)]">
					<AlertCircle size={16} /> {result.error}
				</div>
			{/if}

			<form
				method="POST"
				action="?/start"
				use:enhance={() => {
					submitting = true;
					return async ({ result: r }) => {
						submitting = false;
						if (r.type === 'success') {
							result = r.data as { ok?: boolean; error?: string; import?: ContentImport };
							if (result?.ok) {
								await invalidate('/me/imports');
							}
						}
					};
				}}
				class="space-y-3"
			>
				<div>
					<label for="source_type" class="block text-xs font-medium text-[var(--color-muted)]">
						{t('import.source_type', 'Source type')}
					
					<HintIcon text={t('import.source_type_hint', 'Where this data comes from — CSV, API, manual.')}  />
</label>
					<!-- Hidden field keeps the SvelteKit form action payload intact. -->
					<input type="hidden" name="source_type" value={sourceType} />
					<Autocomplete
						placeholder={t('import.source_type', 'Source type')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchSources}
						initialFetcher={fetchSourcesInitial}
						selectedId={sourceType || null}
						selectedLabel={selectedSourceLabel}
						onSelect={(item) => (sourceType = item.id)}
						onClear={() => (sourceType = sources[0]?.value ?? '')}
					/>
				</div>

				{#if requiresUrl}
					<div>
						<label for="source_url" class="block text-xs font-medium text-[var(--color-muted)]">
							{t('import.source_url', 'Source URL')}
						
						<HintIcon text={t('import.source_url_hint', 'The URL to import from — full URL including https://.')}  />
</label>
						<input
							id="source_url"
							name="source_url"
							type="url"
							bind:value={sourceUrl}
							placeholder="https://medium.com/@yourname"
							class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
						/>
					</div>
				{/if}

				<button
					type="submit"
					disabled={submitting || (requiresUrl && !sourceUrl)}
					class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
				>
					{submitting ? t('common.submitting', 'Submitting…') : t('import.start_btn', 'Start import')}
				</button>
			</form>
		</div>

		<!-- Import history -->
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">
			{t('import.history', 'Import history')}
		</h2>

		{#if imports.length === 0}
			<EmptyState icon={Download} title={t('import.no_imports', 'No imports yet')} />
		{:else}
			<div class="space-y-2">
				{#each imports as imp (imp.id)}
					<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0 flex-1">
								<span class="block text-sm font-medium capitalize text-[var(--color-text)]">
									{imp.source_type}
								</span>
								{#if imp.source_url}
									<span class="mt-0.5 block truncate text-xs text-[var(--color-muted)]">{imp.source_url}</span>
								{/if}
								<span class="mt-1 block text-xs text-[var(--color-muted)]">
									<RelativeTime iso={imp.created_at} />
									{#if imp.completed_at}
										· {t('import.completed', 'Completed')} <RelativeTime iso={imp.completed_at} />
									{/if}
								</span>
								{#if imp.total_items > 0}
									<span class="mt-1 block text-xs text-[var(--color-muted)]">
										{imp.imported_count}/{imp.total_items} {t('import.imported', 'imported')}
										{#if imp.failed_count > 0}
											· {imp.failed_count} {t('import.failed', 'failed')}
										{/if}
									</span>
								{/if}
							</div>
							<span class="flex shrink-0 items-center gap-1 text-xs font-medium capitalize {statusColor(imp.status)}">
								{#if imp.status === 'completed'}
									<CheckCircle2 size={12} />
								{:else if imp.status === 'failed'}
									<AlertCircle size={12} />
								{:else}
									<Loader2 size={12} class={imp.status === 'processing' ? 'animate-spin' : ''} />
								{/if}
								{imp.status}
							</span>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
