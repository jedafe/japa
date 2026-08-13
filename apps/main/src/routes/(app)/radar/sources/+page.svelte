<script lang="ts">
	// Public source directory at /radar/sources (INTEL-024 fix).
	// Lists the intelligence crawl sources as a transparency surface.
	// All data from SSR load() — no client fetch, no hardcoded data (Rule 16).
	import { t } from '$lib/i18n';
	import { ArrowLeft, Database, ShieldCheck, ShieldAlert, Globe } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';

	let { data } = $props();

	interface SourceSummary {
		name: string;
		domain: string;
		trust_level: number;
		category: string;
		last_verified_at?: string | null;
	}

	const sources = $derived<SourceSummary[]>(data.sources);
	const intelUnavailable = $derived(data.intelUnavailable);

	const trustLabel = (level: number) => {
		const labels: Record<number, string> = {
			1: t('radar.sources.tier1', 'Official'),
			2: t('radar.sources.tier2', 'Trusted'),
			3: t('radar.sources.tier3', 'Aggregator'),
			4: t('radar.sources.tier4', 'Community')
		};
		return labels[level] ?? t('radar.sources.tier_unknown', 'Unknown');
	};

	function formatDate(val?: string | null): string {
		if (!val) return '—';
		try {
			return new Date(val).toLocaleDateString();
		} catch {
			return val;
		}
	}
</script>

<svelte:head>
	<title>{t('radar.sources.title', 'Data Sources')} · {t('radar.title', 'Japa Radar')}</title>
	<meta name="description" content={t('radar.sources.desc', 'The crawl sources powering the Japa Radar intelligence feed.')} />
</svelte:head>

<div class="mx-auto max-w-4xl px-4 py-6">
	<a
		href="/radar"
		class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] transition-colors hover:text-[var(--color-text)]"
	>
		<ArrowLeft size={15} />
		{t('radar.back')}
	</a>

	<h1 class="mt-5 text-xl font-medium text-[var(--color-text)] sm:text-2xl">
		{t('radar.sources.title', 'Data Sources')}
	</h1>
	<p class="mt-1 text-sm text-[var(--color-muted)]">
		{t('radar.sources.desc', 'The crawl sources powering the Japa Radar intelligence feed.')}
	</p>

	{#if intelUnavailable}
		<EmptyState
			icon={Database}
			title={t('radar.unavailable_title', 'Intelligence system unavailable')}
			body={t('radar.unavailable_desc', 'The intelligence system is not currently configured.')}
		/>
	{:else if sources.length === 0}
		<EmptyState
			icon={Database}
			title={t('radar.sources.empty', 'No sources registered')}
			body={t('radar.sources.empty_desc', 'There are no crawl sources in the registry yet.')}
		/>
	{:else}
		<div class="mt-6 overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]">
			<table class="w-full text-sm">
				<thead class="border-b border-[var(--color-border)] bg-[var(--color-surface-subtle)]">
					<tr>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('radar.sources.col_name', 'Source')}
						</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('radar.sources.col_category', 'Category')}
						</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('radar.sources.col_trust', 'Trust Tier')}
						</th>
						<th class="px-4 py-3 text-left text-xs font-medium uppercase tracking-wide text-[var(--color-muted)]">
							{t('radar.sources.col_verified', 'Last Verified')}
						</th>
					</tr>
				</thead>
				<tbody class="divide-y divide-[var(--color-border)]">
					{#each sources as src}
						<tr class="transition-colors hover:bg-[var(--color-surface-subtle)]">
							<td class="px-4 py-3">
								<div class="flex items-center gap-2">
									<Globe size={14} class="shrink-0 text-[var(--color-muted)]" />
									<div class="min-w-0">
										<p class="truncate font-medium text-[var(--color-text)]">{src.name}</p>
										<p class="truncate text-xs text-[var(--color-muted)]">{src.domain}</p>
									</div>
								</div>
							</td>
							<td class="px-4 py-3 capitalize text-[var(--color-text)]">{src.category}</td>
							<td class="px-4 py-3">
								<span class="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--color-text)]">
									{#if src.trust_level <= 2}
										<ShieldCheck size={12} class="text-[var(--color-success)]" />
									{:else}
										<ShieldAlert size={12} class="text-[var(--color-warning)]" />
									{/if}
									{trustLabel(src.trust_level)}
								</span>
							</td>
							<td class="px-4 py-3 text-xs text-[var(--color-muted)]">
								{formatDate(src.last_verified_at)}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
