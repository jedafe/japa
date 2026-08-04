<script lang="ts">
	import { t } from '$lib/i18n';
	import { Download, FileUp, CheckCircle2, AlertCircle, Loader2 } from 'lucide-svelte';
	import { EmptyState, RelativeTime } from '$lib/components/ui';
	import { enhance } from '$app/forms';
	import { invalidate } from '$app/navigation';
	import type { ContentImport } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const imports = $state<ContentImport[]>(data.imports);

	let sourceType = $state('medium');
	let sourceUrl = $state('');
	let result = $state<{ ok?: boolean; error?: string; import?: ContentImport } | null>(null);
	let submitting = $state(false);

	const statusColor = (status: string): string => {
		switch (status) {
			case 'completed': return 'text-green-600';
			case 'failed': return 'text-red-500';
			case 'processing': return 'text-blue-500';
			default: return 'text-[var(--color-muted)]';
		}
	};
</script>

<svelte:head>
	<title>{t('import.title', 'Import Content — JapaConnect')}</title>
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="border-b border-[var(--color-border)] px-4 py-3">
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
				<div class="mb-3 flex items-center gap-2 text-sm text-green-600">
					<CheckCircle2 size={16} /> {t('import.started', 'Import started. We will process it in the background.')}
				</div>
			{:else if result?.error}
				<div class="mb-3 flex items-center gap-2 text-sm text-red-500">
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
					</label>
					<select
						id="source_type"
						name="source_type"
						bind:value={sourceType}
						class="mt-1 w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:outline-none focus:ring-2 focus:ring-[var(--color-primary)]"
					>
						<option value="medium">Medium</option>
						<option value="substack">Substack</option>
						<option value="rss">RSS Feed</option>
						<option value="zip">ZIP File</option>
					</select>
				</div>

				{#if sourceType !== 'zip'}
					<div>
						<label for="source_url" class="block text-xs font-medium text-[var(--color-muted)]">
							{t('import.source_url', 'Source URL')}
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
					disabled={submitting || (sourceType !== 'zip' && !sourceUrl)}
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
