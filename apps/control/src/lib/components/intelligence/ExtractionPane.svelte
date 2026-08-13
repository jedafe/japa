<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { Sparkles, AlertTriangle, CheckCircle2 } from 'lucide-svelte';

	// ExtractionPane renders the extraction results for an intel record.
	// The `extracted` blob is the structured output from the extractor; the
	// `confidence` blob holds per-field confidence scores; `evidence` holds
	// the citation snippets. All three are JSON blobs (or already-parsed
	// objects) coming from the API.

	let {
		extracted = null as unknown,
		confidence = null as unknown,
		evidence = null as unknown,
		confidenceOverall = null as number | null,
		loading = false
	}: {
		extracted?: unknown;
		confidence?: unknown;
		evidence?: unknown;
		confidenceOverall?: number | null;
		loading?: boolean;
	} = $props();

	function stringify(val: unknown): string {
		if (val == null) return '';
		if (typeof val === 'string') {
			try {
				return JSON.stringify(JSON.parse(val), null, 2);
			} catch {
				return val;
			}
		}
		try {
			return JSON.stringify(val, null, 2);
		} catch {
			return String(val);
		}
	}

	const extractedJson = $derived(stringify(extracted));
	const confidenceJson = $derived(stringify(confidence));
	const evidenceJson = $derived(stringify(evidence));

	const hasData = $derived(!!extractedJson || !!confidenceJson || !!evidenceJson);

	// Confidence bucket for the overall score badge.
	const confidenceTier = $derived.by(() => {
		const v = confidenceOverall;
		if (v == null) return 'unknown';
		if (v >= 0.85) return 'high';
		if (v >= 0.6) return 'medium';
		return 'low';
	});

	const tierColor = $derived(
		confidenceTier === 'high'
			? 'text-emerald-500 bg-emerald-500/10'
			: confidenceTier === 'medium'
				? 'text-amber-500 bg-amber-500/10'
				: confidenceTier === 'low'
					? 'text-destructive bg-destructive/10'
					: 'text-muted-foreground bg-muted'
	);

	const tierLabel = $derived(
		confidenceTier === 'high'
			? t('intel.confidence_high', 'High')
			: confidenceTier === 'medium'
				? t('intel.confidence_medium', 'Medium')
				: confidenceTier === 'low'
					? t('intel.confidence_low', 'Low')
					: t('intel.confidence_unknown', 'Unknown')
	);
</script>

<div class="border-border-card bg-background-alt shadow-card flex h-full flex-col rounded-xl">
	<div class="border-border-card flex items-center justify-between border-b px-4 py-3">
		<div class="flex items-center gap-2">
			<Sparkles class="text-muted-foreground size-4" />
			<h3 class="text-foreground text-sm font-semibold">
				{t('intel.extraction_results', 'Extraction Results')}
			</h3>
		</div>
		{#if confidenceOverall != null}
			<span class="inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium {tierColor}">
				{#if confidenceTier === 'high'}
					<CheckCircle2 class="size-3" />
				{:else if confidenceTier === 'low'}
					<AlertTriangle class="size-3" />
				{/if}
				{tierLabel} · {(confidenceOverall * 100).toFixed(0)}%
			</span>
		{/if}
	</div>

	<div class="flex-1 overflow-y-auto p-4">
		{#if loading}
			<div class="space-y-2">
				{#each Array(6) as _}
					<div class="bg-muted h-4 w-2/3 animate-pulse rounded"></div>
				{/each}
			</div>
		{:else if !hasData}
			<div class="text-muted-foreground py-8 text-center text-sm">
				<Sparkles class="mx-auto size-8 opacity-30" />
				<p class="mt-2">{t('intel.no_extraction', 'No extraction data available')}</p>
			</div>
		{:else}
			{#if extractedJson}
				<div class="mb-4">
					<h4 class="text-muted-foreground mb-1.5 text-xs font-semibold uppercase tracking-wider">
						{t('intel.extracted_fields', 'Extracted Fields')}
					</h4>
					<div class="border-border-card bg-background overflow-auto rounded-lg border p-3">
						<pre class="text-foreground whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">{extractedJson}</pre>
					</div>
				</div>
			{/if}

			{#if confidenceJson}
				<div class="mb-4">
					<h4 class="text-muted-foreground mb-1.5 text-xs font-semibold uppercase tracking-wider">
						{t('intel.confidence_scores', 'Confidence Scores')}
					</h4>
					<div class="border-border-card bg-background overflow-auto rounded-lg border p-3">
						<pre class="text-foreground whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">{confidenceJson}</pre>
					</div>
				</div>
			{/if}

			{#if evidenceJson}
				<div>
					<h4 class="text-muted-foreground mb-1.5 text-xs font-semibold uppercase tracking-wider">
						{t('intel.evidence', 'Evidence')}
					</h4>
					<div class="border-border-card bg-background overflow-auto rounded-lg border p-3">
						<pre class="text-foreground whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">{evidenceJson}</pre>
					</div>
				</div>
			{/if}
		{/if}
	</div>
</div>
