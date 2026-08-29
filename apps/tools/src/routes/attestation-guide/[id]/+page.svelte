<script lang="ts">
	import { untrack } from 'svelte';
	import { PUBLIC_JC_API_URL } from '$env/static/public';
	import { buildApiUrl } from '@japaconnect/api-url';
	import { buildSeoMeta } from '$lib/seo';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import type { AttestationPathwayWithAi } from '$lib/types';

	/**
	 * Attestation pathway detail page — §97, ADR-105.
	 * Renders a single attestation pathway: document type, issuing/destination,
	 * apostille requirement, pathway steps, and AI/HITL metadata.
	 */
	let { data } = $props();

	let pathway = $state<AttestationPathwayWithAi>(untrack(() => data.pathway));

	const seo = $derived(
		buildSeoMeta({
			title: `${pathway.document_type} — ${t('attestation_guide.seo_title', 'Attestation Guide — JapaConnect Tools')}`,
			description: `Attestation pathway for ${pathway.document_type} from ${pathway.issuing_space_name ?? ''} to ${pathway.destination_space_name ?? ''}.`,
			path: `/attestation-guide/${page.params.id}`,
			origin: page.url.origin
		})
	);

	/** Render pathway_steps as an ordered list. Each step is a JSON object that
	 *  may have title, description, url — rendered generically. */
	function pathwayStepEntries(step: unknown): [string, unknown][] {
		if (!step || typeof step !== 'object') return [];
		return Object.entries(step as Record<string, unknown>);
	}

	function isStepObject(step: unknown): step is Record<string, unknown> {
		return step !== null && typeof step === 'object' && !Array.isArray(step);
	}

	/** Format an ISO date string as a readable date. */
	function formatDate(iso?: string | null): string {
		if (!iso) return '';
		try {
			return new Date(iso).toLocaleDateString(undefined, {
				year: 'numeric',
				month: 'short',
				day: 'numeric'
			});
		} catch {
			return iso;
		}
	}

	/** ADR-105: community_last_verified older than 180 days (or NULL) is stale. */
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
	<title>{seo.title}</title>
	<meta name="description" content={seo.description} />
	<link rel="canonical" href={seo.canonical} />
	<meta property="og:title" content={seo.title} />
	<meta property="og:description" content={seo.description} />
	<meta property="og:url" content={seo.canonical} />
	<meta property="og:type" content="website" />
	<meta name="twitter:card" content="summary" />
	<meta name="twitter:title" content={seo.title} />
	<meta name="twitter:description" content={seo.description} />
</svelte:head>

<div class="mx-auto max-w-3xl px-4 py-8">
	<a
		href="/attestation-guide"
		class="inline-flex items-center gap-1.5 text-sm text-muted hover:text-text transition-colors mb-6"
	>
		<Icon name="arrow-left" size={16} />
		{t('attestation_guide.back_to_list', 'Back to attestation guide')}
	</a>

	<div class="p-6 bg-surface border border-border rounded-lg space-y-4">
		<div>
			<h1 class="text-2xl font-bold text-text">{pathway.document_type}</h1>
			<p class="text-sm text-text-subtle mt-1">
				{pathway.issuing_space_name ?? t('attestation_guide.unknown_issuing', 'Unknown')}
				→
				{pathway.destination_space_name ?? t('attestation_guide.unknown_dest', 'Unknown')}
			</p>
		</div>

		<div class="flex flex-wrap gap-2">
			{#if pathway.apostille_required}
				<span
					class="text-xs px-2 py-0.5 rounded-full font-medium"
					style="background: color-mix(in srgb, var(--color-primary) 15%, transparent); color: var(--color-primary);"
				>
					{t('attestation_guide.apostille_required', 'Apostille required')}
				</span>
			{:else}
				<span
					class="text-xs px-2 py-0.5 rounded-full font-medium"
					style="background: color-mix(in srgb, var(--color-muted) 15%, transparent); color: var(--color-muted);"
				>
					{t('attestation_guide.no_apostille', 'No apostille')}
				</span>
			{/if}
		</div>
	</div>

	{#if Array.isArray(pathway.pathway_steps) && pathway.pathway_steps.length > 0}
		<div class="mt-6 p-6 bg-surface border border-border rounded-lg">
			<h2 class="text-sm font-semibold text-muted uppercase tracking-wide mb-3">
				{t('attestation_guide.pathway_steps', 'Pathway steps')}
			</h2>
			<ol class="space-y-3">
				{#each pathway.pathway_steps as step, i (i)}
					<li class="flex items-start gap-3">
						<span
							class="flex-shrink-0 w-6 h-6 rounded-full bg-[var(--tool-accent)]/10 text-[var(--tool-accent)] text-xs font-semibold flex items-center justify-center"
						>
							{i + 1}
						</span>
						<div class="text-sm text-text min-w-0">
							{#if isStepObject(step)}
								{#each pathwayStepEntries(step) as [key, val] (key)}
									<div class="flex items-start gap-1.5">
										<span class="font-medium text-text">{key}:</span>
										{#if key === 'url' && typeof val === 'string'}
											<a
												href={val}
												target="_blank"
												rel="noopener noreferrer"
												class="text-primary hover:underline break-all"
											>
												{val}
											</a>
										{:else}
											<span class="text-text-subtle">{String(val)}</span>
										{/if}
									</div>
								{/each}
							{:else}
								<span class="text-text-subtle">{String(step)}</span>
							{/if}
						</div>
					</li>
				{/each}
			</ol>
		</div>
	{/if}

	<div class="mt-6 p-6 bg-surface border border-border rounded-lg space-y-4">
		<div class="flex flex-wrap items-center gap-3">
			<a
				href={buildApiUrl(`/attestation/${pathway.id}/export/pdf`, PUBLIC_JC_API_URL)}
				target="_blank"
				rel="noopener noreferrer"
				class="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-text bg-surface border border-border rounded-lg hover:border-primary hover:text-primary transition-colors"
			>
				<Icon name="download" size={16} />
				{t('attestation_guide.download_pdf', 'Export pathway')}
			</a>
		</div>

		{#if pathway.community_last_verified}
			<p class="text-xs text-muted">
				{t('attestation_guide.community_last_verified', 'Community last verified:')}
				{formatDate(pathway.community_last_verified)}
			</p>
		{/if}

		{#if isStale(pathway.community_last_verified)}
			<p
				class="text-xs px-3 py-2 rounded-lg"
				style="background: color-mix(in srgb, var(--color-warning, #f59e0b) 10%, transparent); color: var(--color-warning, #f59e0b);"
			>
				{t(
					'attestation_guide.stale_warning',
					'This pathway has not been community-verified recently. Details may be outdated — please confirm with official sources.'
				)}
			</p>
		{/if}

		{#if pathway.ai_generated}
			<div class="p-3 bg-amber-500/5 border border-amber-500/20 rounded-lg flex items-start gap-2">
				<Icon name="alert-triangle" size={14} class="text-amber-600 flex-shrink-0 mt-0.5" />
				<div class="text-xs text-text leading-relaxed">
					<p>
						{t('common.ai_generated', 'AI-generated content')}
						{#if pathway.ai_model}({pathway.ai_model}){/if}
					</p>
					{#if pathway.hitl_reviewed_by}
						<p class="mt-1 text-muted">
							{t('common.reviewed_by', 'Reviewed by')} {pathway.hitl_reviewed_by}
							{#if pathway.hitl_reviewed_at}
								· {formatDate(pathway.hitl_reviewed_at)}
							{/if}
						</p>
					{:else}
						<p class="mt-1 text-amber-600">
							{t(
								'common.hitl_pending',
								'This content has not been human-reviewed yet. Verify with official sources.'
							)}
						</p>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</div>
