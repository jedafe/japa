<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { Stamp, ArrowLeft, Download, Globe, AlertTriangle } from 'lucide-svelte';
	import { PUBLIC_JC_API_URL } from '$env/static/public';
	import { buildApiUrl } from '@japaconnect/api-url';
	import type { PageData } from './$types';

	/**
	 * Attestation Guide detail page — Section 97, ADR-105.
	 *
	 * Ref: japaconnect-engineering/sections/97-attestation-guide.md
	 */
	let { data }: { data: PageData } = $props();

	const pathway = $derived(data.pathway!);

	const title = $derived(`${pathway.document_type} — ${t('attestation_guide.title', 'Attestation Guide')}`);
	const description = $derived(t('attestation_guide.subtitle', 'Step-by-step document attestation pathways.'));

	function pathwayStepEntries(step: unknown): [string, unknown][] {
		if (!step || typeof step !== 'object') return [];
		return Object.entries(step as Record<string, unknown>);
	}

	function isStepObject(step: unknown): step is Record<string, unknown> {
		return step !== null && typeof step === 'object' && !Array.isArray(step);
	}

	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
		} catch {
			return iso;
		}
	}

	const STALENESS_THRESHOLD_DAYS = 180;

	function isStale(iso?: string | null): boolean {
		if (!iso) return true;
		try {
			const verified = new Date(iso);
			const daysOld = (Date.now() - verified.getTime()) / (1000 * 60 * 60 * 24);
			return daysOld > STALENESS_THRESHOLD_DAYS;
		} catch {
			return true;
		}
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={description} />
	<link rel="canonical" href={page.url.href} />
	<meta property="og:title" content={title} />
	<meta property="og:description" content={description} />
	<meta property="og:url" content={page.url.href} />
	<meta property="og:type" content="website" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl">
	<div class="mx-4 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href={pathway.destination_space_id
				? `/tools/attestation-guide?dest=${pathway.destination_space_id}`
				: '/tools/attestation-guide'}
			class="inline-flex items-center gap-1.5 text-sm text-[var(--color-muted)] hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={16} />
			{t('attestation_guide.back', 'Back to attestation guide')}
		</a>
	</div>

	<div class="p-4 space-y-4">
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5">
			<div class="flex items-start gap-3">
				<div class="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
					<Stamp size={20} />
				</div>
				<div class="min-w-0 flex-1">
					<h1 class="text-lg font-semibold text-[var(--color-text)] capitalize">{pathway.document_type}</h1>
					<p class="text-sm text-[var(--color-text-subtle)]">
						{pathway.issuing_space_name ?? t('attestation_guide.unknown_issuing', 'Unknown')}
						→
						{pathway.destination_space_name ?? t('attestation_guide.unknown_dest', 'Unknown')}
					</p>
					{#if pathway.apostille_required}
						<span
							class="mt-2 inline-flex items-center gap-1 rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-primary)]"
						>
							{t('attestation_guide.apostille_required', 'Apostille required')}
						</span>
					{:else}
						<span
							class="mt-2 inline-flex items-center gap-1 rounded-full bg-[var(--color-muted)]/10 px-2 py-0.5 text-xs font-medium text-[var(--color-muted)]"
						>
							{t('attestation_guide.no_apostille', 'No apostille')}
						</span>
					{/if}
				</div>
			</div>
		</div>

		{#if pathway.community_last_verified}
			<p class="text-xs text-[var(--color-muted)]">
				{t('attestation_guide.community_last_verified', 'Community last verified:')}
				{formatDate(pathway.community_last_verified)}
			</p>
		{/if}
		{#if isStale(pathway.community_last_verified)}
			<p
				class="rounded-[var(--radius)] px-3 py-2 text-xs"
				style="background: color-mix(in srgb, var(--color-warning, #f59e0b) 10%, transparent); color: var(--color-warning, #f59e0b)"
			>
				<AlertTriangle size={14} class="inline align-text-bottom" />
				{t(
					'attestation_guide.stale_warning',
					'This pathway has not been community-verified recently. Details may be outdated — please confirm with official sources.'
				)}
			</p>
		{/if}

		{#if Array.isArray(pathway.pathway_steps) && pathway.pathway_steps.length > 0}
			<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
				<h2 class="mb-3 text-sm font-semibold text-[var(--color-text)]">{t('attestation_guide.pathway_steps', 'Pathway steps')}</h2>
				<ol class="space-y-3">
					{#each pathway.pathway_steps as step, i (i)}
						<li class="flex items-start gap-3">
							<span
								class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-xs font-semibold text-[var(--color-primary)]"
							>
								{i + 1}
							</span>
							<div class="min-w-0 flex-1 text-sm text-[var(--color-text)]">
								{#if isStepObject(step)}
									{#each pathwayStepEntries(step) as [key, val] (key)}
										<div class="flex items-start gap-1.5">
											<span class="font-medium">{key}:</span>
											{#if key === 'url' && typeof val === 'string'}
												<a
													href={val}
													target="_blank"
													rel="noopener noreferrer"
													class="break-all text-[var(--color-primary)] hover:underline"
												>
													{val}
												</a>
											{:else}
												<span class="text-[var(--color-text-subtle)]">{String(val)}</span>
											{/if}
										</div>
									{/each}
								{:else}
									<span class="text-[var(--color-text-subtle)]">{String(step)}</span>
								{/if}
							</div>
						</li>
					{/each}
				</ol>
			</div>
		{/if}

		<div class="flex flex-wrap items-center gap-3">
			<a
				href={buildApiUrl(`/attestation/${pathway.id}/export/pdf`, PUBLIC_JC_API_URL)}
				target="_blank"
				rel="noopener noreferrer"
				class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] hover:border-[var(--color-primary)] hover:text-[var(--color-primary)]"
			>
				<Download size={16} />
				{t('attestation_guide.download_pdf', 'Export pathway')}
			</a>
		</div>
	</div>
</div>
