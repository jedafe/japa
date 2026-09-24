<script lang="ts">
	// Ref: docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md §5
	// Public source proposal form body. Submits to POST /api/v1/sources/propose
	// and renders category/source-kind options from the backend metadata
	// (Rule 16 — no hardcoded option lists). A debounced blur (or the
	// "Check URL" button) runs POST /api/v1/sources/probe and renders the
	// detected-method chips + proposed channels preview; the last successful
	// report is attached to the proposal as probe_snapshot. Rendered inline
	// by the /sources/submit page and inside ProposeSourceModal.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { requireAuthOrRedirect } from '$lib/auth/guest';
	import { Loader2, ScanSearch, AlertTriangle, X } from 'lucide-svelte';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/create/Autocomplete.svelte';
	import { staticAutocompleteFetcher } from '$lib/autocomplete';
	import { sourceCategoryOptions, sourceCategoryLabel } from '$lib/sources';
	import type { SourceMeta, ProbeReport } from '$lib/types/sources';

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
	let categories = $state<string[]>([]);
	let sourceKind = $state('publisher');
	let description = $state('');
	let loading = $state(false);
	let submitted = $state(false);
	let error = $state('');

	// Live probe preview state (spec §2 preview stage). probeUrl records the
	// URL the current report belongs to — editing the URL invalidates it so a
	// stale snapshot is never submitted.
	let probeReport = $state<ProbeReport | null>(null);
	let probeLoading = $state(false);
	let probeError = $state('');
	let probeUrl = $state('');
	let probeTimer: ReturnType<typeof setTimeout> | null = null;
	let probeSeq = 0;

	const kindOptions = $derived(meta.submittable_source_kinds ?? meta.source_kinds ?? []);
	// Categories are the shared canonical list (curated catalog ∪ full
	// taxonomy) — the same set the /sources directory filter offers.
	const categoryOptions = $derived(sourceCategoryOptions(meta));
	// Options already picked are removed from the autocomplete list so a
	// category cannot be selected twice.
	const categoryItems = $derived<AutocompleteItem[]>(
		categoryOptions
			.filter((c) => !categories.includes(c))
			.map((c) => ({ id: c, label: sourceCategoryLabel(c) }))
	);
	const categoryFetcher = $derived(staticAutocompleteFetcher(categoryItems));
	const probeMethods = $derived(probeReport?.methods ?? []);
	const probeChannels = $derived(probeReport?.channels ?? []);
	const probeWarnings = $derived(probeReport?.warnings ?? []);
	const probeJsonldTypes = $derived(probeReport?.jsonld_types ?? []);
	const robotsBlocked = $derived(!!probeReport?.robots?.fetched && !probeReport.robots.allowed);

	function removeCategory(c: string) {
		categories = categories.filter((v) => v !== c);
	}

	function methodLabel(m: string): string {
		return t(`sources.submit.method.${m}`, m.replace(/_/g, ' '));
	}

	// Loose client-side gate — the backend normalizes/validates; we only avoid
	// firing probes for obviously incomplete input.
	function looksLikeUrl(v: string): boolean {
		const s = v.trim();
		return s.length > 3 && (/^https?:\/\//i.test(s) || s.includes('.'));
	}

	function onUrlInput() {
		if (sourceUrl.trim() !== probeUrl) {
			probeReport = null;
			probeError = '';
		}
		if (probeTimer) {
			clearTimeout(probeTimer);
			probeTimer = null;
		}
	}

	// Debounced probe on blur (spec §5: "URL blur → POST /sources/probe").
	function scheduleProbe() {
		if (probeTimer) clearTimeout(probeTimer);
		const url = sourceUrl.trim();
		if (!looksLikeUrl(url) || url === probeUrl) return;
		probeTimer = setTimeout(() => void runProbe(), 600);
	}

	async function runProbe() {
		if (probeTimer) {
			clearTimeout(probeTimer);
			probeTimer = null;
		}
		const url = sourceUrl.trim();
		if (!looksLikeUrl(url) || loading) return;
		// Probe requires auth — send guests to login instead of surfacing a
		// raw 401 under the URL field.
		if (!(await requireAuthOrRedirect())) return;

		const seq = ++probeSeq;
		probeLoading = true;
		probeError = '';
		try {
			const res = await api.post<ProbeReport>('/sources/probe', {
				url,
				source_kind: sourceKind,
				categories
			});
			if (seq !== probeSeq) return; // a newer probe superseded this one
			if (res.data) {
				probeReport = res.data;
				probeUrl = url;
				probeError = '';
			} else {
				// Degrade gracefully (401/422/429/network) — the form still
				// submits without a snapshot.
				probeReport = null;
				probeUrl = '';
				probeError =
					res.error?.message ?? t('sources.submit.probe.failed', 'Could not inspect this URL — you can still submit.');
			}
		} catch {
			if (seq === probeSeq) {
				probeReport = null;
				probeError = t('sources.submit.probe.failed', 'Could not inspect this URL — you can still submit.');
			}
		} finally {
			if (seq === probeSeq) probeLoading = false;
		}
	}

	function validate(): boolean {
		error = '';
		if (!name.trim()) error = t('sources.submit.error.name_required', 'Name is required');
		else if (!sourceUrl.trim()) error = t('sources.submit.error.url_required', 'Source URL is required');
		else if (categories.length === 0)
			error = t('sources.submit.error.categories_required', 'Select at least one category');
		return !error;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (!validate()) return;
		// Proposals require auth — send guests to login instead of surfacing
		// a raw 401 from /sources/propose.
		if (!(await requireAuthOrRedirect())) return;
		loading = true;
		error = '';
		try {
			const payload: Record<string, unknown> = {
				name: name.trim(),
				domain: domain.trim(),
				source_url: sourceUrl.trim(),
				categories,
				source_kind: sourceKind,
				description: description.trim() || null
			};
			// Attach the last successful probe report for this URL (spec §5) —
			// omitted entirely when probing failed or was never run.
			if (probeReport && probeUrl === sourceUrl.trim()) {
				payload.probe_snapshot = probeReport;
			}
			const res = await api.post<{ id: string; slug: string; status: string; admin_review_url: string }>(
				'/sources/propose',
				payload
			);
			if (res.data) {
				submitted = true;
				onSubmitted?.();
			} else if (res.error?.code === 'duplicate_source') {
				error = t('sources.submit.error.duplicate', 'A source with this URL already exists.');
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
			<input id="psf-name" name="name" bind:value={name} class={inputClass()} placeholder="DAAD Scholarships" disabled={loading} />
		</div>

		<div>
			<label class="mb-1 block text-sm font-medium" for="psf-source-url">
				{t('sources.submit.source_url', 'Source URL')}
			</label>
			<div class="flex items-start gap-2">
				<input
					id="psf-source-url"
					name="source_url"
					type="url"
					bind:value={sourceUrl}
					oninput={onUrlInput}
					onblur={scheduleProbe}
					class={inputClass()}
					placeholder="https://www.daad.de/en/scholarships/"
					disabled={loading}
				/>
				<button
					type="button"
					onclick={() => void runProbe()}
					disabled={loading || probeLoading || !looksLikeUrl(sourceUrl)}
					class="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] transition hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
				>
					{#if probeLoading}
						<Loader2 size={14} class="animate-spin" />
						{t('sources.submit.probe.checking', 'Checking…')}
					{:else}
						<ScanSearch size={14} />
						{t('sources.submit.probe.check', 'Check URL')}
					{/if}
				</button>
			</div>

			{#if probeError}
				<p class="mt-1.5 flex items-center gap-1 text-xs text-[var(--color-muted)]" role="status">
					<AlertTriangle size={12} class="shrink-0" />
					{probeError}
				</p>
			{/if}

			{#if probeReport}
				<div class="mt-2 flex flex-col gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3">
					{#if robotsBlocked}
						<p class="flex items-center gap-1.5 text-xs font-medium text-amber-600 dark:text-amber-400" role="alert">
							<AlertTriangle size={12} class="shrink-0" />
							{t('sources.submit.probe.robots_blocked', 'robots.txt disallows automated access to this site.')}
						</p>
					{/if}

					{#if probeMethods.length > 0 || probeJsonldTypes.length > 0 || probeReport.has_websub || probeReport.requires_js}
						<div>
							<p class="mb-1 text-xs font-medium text-[var(--color-muted)]">
								{t('sources.submit.probe.detected', 'Detected')}
							</p>
							<div class="flex flex-wrap gap-1.5">
								{#each probeMethods as m (m.method)}
									<span
										class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs text-[var(--color-primary)]"
										title="{m.evidence} ({Math.round(m.confidence * 100)}%)"
									>
										{methodLabel(m.method)}
									</span>
								{/each}
								{#each probeJsonldTypes as jt (jt)}
									<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs text-[var(--color-primary)]">
										{t('sources.submit.probe.jsonld_chip', '{type} schema', { type: jt })}
									</span>
								{/each}
								{#if probeReport.has_websub && !probeMethods.some((m) => m.method === 'webhook')}
									<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs text-[var(--color-primary)]">
										{t('sources.submit.probe.websub_chip', 'WebSub')}
									</span>
								{/if}
								{#if probeReport.requires_js && !probeMethods.some((m) => m.method === 'render')}
									<span class="rounded-full bg-[var(--color-primary)]/10 px-2 py-0.5 text-xs text-[var(--color-primary)]">
										{t('sources.submit.method.render', 'Requires JS rendering')}
									</span>
								{/if}
							</div>
						</div>
					{/if}

					{#if probeChannels.length > 0}
						<div>
							<p class="mb-1 text-xs font-medium text-[var(--color-muted)]">
								{t('sources.submit.probe.channels', 'Proposed channels')}
							</p>
							<ul class="flex flex-col gap-1.5">
								{#each probeChannels as ch (ch.crawl_method + ch.entry_url)}
									<li class="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5">
										<div class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
											<span class="text-xs font-medium text-[var(--color-text)]">{ch.label}</span>
											<span class="rounded bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10px] text-[var(--color-muted)]">
												{methodLabel(ch.crawl_method)}
											</span>
											{#if ch.pipeline}
												<span class="text-[10px] text-[var(--color-muted)]">{ch.pipeline}</span>
											{/if}
										</div>
										{#if ch.entity_types && ch.entity_types.length > 0}
											<div class="mt-1 flex flex-wrap gap-1">
												{#each ch.entity_types as et (et)}
													<span class="rounded-full border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-subtle)]">
														{et}
													</span>
												{/each}
											</div>
										{/if}
									</li>
								{/each}
							</ul>
						</div>
					{/if}

					{#if probeWarnings.length > 0}
						<div>
							<p class="mb-1 text-xs font-medium text-[var(--color-muted)]">
								{t('sources.submit.probe.warnings', 'Warnings')}
							</p>
							<ul class="flex flex-col gap-0.5">
								{#each probeWarnings as w (w)}
									<li class="flex items-start gap-1 text-xs text-[var(--color-text-subtle)]">
										<AlertTriangle size={11} class="mt-0.5 shrink-0" />
										{w}
									</li>
								{/each}
							</ul>
						</div>
					{/if}
				</div>
			{/if}
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
				placeholder="daad.de"
				disabled={loading}
			/>
		</div>

		<div>
			<span class="mb-1 block text-sm font-medium" id="psf-categories-label">
				{t('sources.submit.categories', 'Categories')}
			</span>
			{#if categoryOptions.length > 0}
				{#if categories.length > 0}
					<div class="mb-1.5 flex flex-wrap gap-1.5" role="group" aria-labelledby="psf-categories-label">
						{#each categories as c (c)}
							<span
								class="inline-flex items-center gap-1 rounded-full border border-[var(--color-primary)] bg-[var(--color-primary)]/10 py-1 pl-2.5 pr-1 text-xs font-medium text-[var(--color-primary)]"
							>
								{sourceCategoryLabel(c)}
								<button
									type="button"
									aria-label={t('common.remove', 'Remove')}
									onclick={() => removeCategory(c)}
									disabled={loading}
									class="flex items-center justify-center rounded-full p-0.5 transition hover:bg-[var(--color-primary)]/20 disabled:opacity-50"
								>
									<X size={11} />
								</button>
							</span>
						{/each}
					</div>
				{/if}
				<Autocomplete
					id="psf-categories"
					placeholder={t('sources.submit.categories_placeholder', 'Search categories…')}
					ariaLabel={t('sources.submit.categories', 'Categories')}
					emptyMessage={t('sources.submit.categories_empty', 'No matching category')}
					fetcher={categoryFetcher}
					initialFetcher={categoryFetcher}
					selectedId={null}
					onSelect={(item) => {
						categories = [...categories, item.id];
					}}
				/>
				<p class="mt-1 text-xs text-[var(--color-muted)]">
					{t('sources.submit.categories_hint', 'Pick every domain this source covers — at least one.')}
				</p>
			{:else}
				<!-- Meta unavailable: free-text fallback (comma-separated) so the
				     form still submits a categories[] payload. -->
				<input
					id="psf-categories"
					name="categories"
					aria-labelledby="psf-categories-label"
					value={categories.join(', ')}
					oninput={(e) => {
						categories = e.currentTarget.value
							.split(',')
							.map((v) => v.trim())
							.filter(Boolean);
					}}
					class={inputClass()}
					placeholder="news, jobs"
					disabled={loading}
				/>
			{/if}
		</div>

		{#if kindOptions.length > 0}
			<div>
				<label class="mb-1 block text-sm font-medium" for="psf-source-kind">
					{t('sources.submit.source_kind', 'Source kind')}
				</label>
				<select id="psf-source-kind" name="source_kind" bind:value={sourceKind} class={inputClass()} disabled={loading}>
					{#each kindOptions as k (k.value)}
						<option value={k.value}>{k.label}</option>
					{/each}
				</select>
			</div>
		{/if}

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
				placeholder="Official DAAD scholarship database for international students"
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
