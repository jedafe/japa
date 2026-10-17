<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import {
		deleteComparison,
		getFreshness,
		promoteComparison
	} from '$lib/api/jce';
	import { t } from '$lib/i18n';
	import {
		deduplicateMatrix,
		uniqueDimensions,
		uniqueResolvedItems
	} from '$lib/utils/comparison';
	import { formatMoney } from '$lib/utils/currency';
	import type {
		FreshnessSummary,
		JCEDimension,
		MatrixDimension,
		MatrixItem,
		ResolvedItem
	} from '$lib/types/jce';
	import type { PageData } from './$types';
	import FreshnessChip from '$lib/components/compare/FreshnessChip.svelte';
	import RefreshButton from '$lib/components/compare/RefreshButton.svelte';
	import { ArrowLeft, Pencil, Sparkles, Trash2, ExternalLink } from 'lucide-svelte';
	import { safeJsonLdScript, sanitizeContentHtml } from '$lib/utils/safeHtml';

	let { data }: { data: PageData } = $props();

	let comparison = $derived(data.comparison);
	let matrix = $derived(deduplicateMatrix(data.matrix ?? []));
	let uniqueDims = $derived(uniqueDimensions(comparison.dimensions ?? []));
	let resolvedItems = $derived(uniqueResolvedItems(data.resolvedItems ?? []));
	let isOwner = $derived(data.isOwner ?? false);
	let mainOrigin = $derived(data.mainOrigin);
	let typeSlug = $derived(data.typeSlug ?? '');
	let user = $derived(data.user ?? null);
	let features = $derived((data.features as Record<string, boolean> | undefined) ?? {});

	let freshness = $state<FreshnessSummary | null>(null);
	let freshnessTimeout: ReturnType<typeof setTimeout> | null = null;

	function scheduleFreshness(contentID: string) {
		if (freshnessTimeout) clearTimeout(freshnessTimeout);
		freshnessTimeout = setTimeout(() => {
			getFreshness(contentID).then((f) => {
				freshness = f;
			});
		}, 300);
	}

	$effect(() => {
		if (comparison?.content?.id && user && features['feature.jce.refresh']) {
			scheduleFreshness(comparison.content.id);
		}
	});

	let deleting = $state(false);
	let promoting = $state(false);
	let error = $state('');

	async function remove() {
		if (!isOwner) return;
		deleting = true;
		error = '';
		try {
			await deleteComparison(comparison.content.id);
			goto('/compare');
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.compare.errors.delete_failed');
		} finally {
			deleting = false;
		}
	}

	function navigateToMain(path: string) {
		if (!mainOrigin) {
			throw new Error(t('jce.compare.errors.no_main_origin'));
		}
		if (mainOrigin === page.url.origin) {
			goto(path);
		} else {
			window.location.href = `${mainOrigin}${path}`;
		}
	}

	async function promote() {
		if (!isOwner) return;
		promoting = true;
		error = '';
		try {
			const promoted = await promoteComparison(comparison.content.id);
			if (promoted.data?.id) {
				// Ref: sections/30-generic-comparison-system.md (JCE promotion to detailed composer)
				navigateToMain(`/create?type=comparison&draft=${promoted.data.id}`);
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.compare.errors.promote_failed');
		} finally {
			promoting = false;
		}
	}

	function edit() {
		const ids = resolvedItems.map((i: ResolvedItem) => i.id).join(',');
		const url = new URL('/compare', page.url);
		if (typeSlug) url.searchParams.set('type', typeSlug);
		url.searchParams.set('preselect', ids);
		goto(url);
	}

	// Section 26 — full head tags for the quick-compare surface: title,
	// description (body excerpt with title fallback so it is never empty),
	// self-referencing canonical on the clean slug URL, OG/Twitter tags,
	// and ItemList + Table JSON-LD for the ranked items/matrix.
	const headTitle = $derived(`${comparison.content.title} — ${t('jce.compare.title', 'Compare')}`);
	const headDescription = $derived(
		(comparison.content.body_html ?? comparison.content.body ?? '')
			.replace(/<[^>]*>/g, ' ')
			.replace(/\s+/g, ' ')
			.trim()
			.slice(0, 160) || comparison.content.title
	);
	const canonical = $derived(`${page.url.origin}/compare/${comparison.content.slug}`);
	const compareJsonLd = $derived(
		JSON.stringify({
			'@context': 'https://schema.org',
			'@type': 'ItemList',
				name: comparison.content.title,
				description: headDescription,
				url: canonical,
				numberOfItems: resolvedItems.length,
				itemListElement: resolvedItems.map((item, i) => ({
					'@type': 'ListItem',
					position: i + 1,
					name: item.title,
					...(item.description ? { description: item.description } : {})
				}))
			})
	);
	const compareTableJsonLd = $derived(
		uniqueDims.length
			? JSON.stringify({
					'@context': 'https://schema.org',
					'@type': 'Table',
					about: comparison.content.title,
					url: canonical
				})
			: ''
	);

	function cellFor(item: MatrixItem, dim: JCEDimension): MatrixDimension | undefined {
		return item.dimensions.find((d: MatrixDimension) => d.dimension_id === dim.id);
	}

	function formatValue(cell?: MatrixDimension) {
		if (!cell) return '—';
		if (
			cell.currency_code &&
			cell.value_rating !== undefined &&
			cell.value_rating !== null
		) {
			return formatMoney(cell.value_rating, cell.currency_code, page.data.currencies);
		}
		if (cell.value !== undefined && cell.value !== null) return cell.value;
		if (cell.value_rating !== undefined && cell.value_rating !== null) return cell.value_rating;
		if (cell.value_boolean !== undefined && cell.value_boolean !== null) return cell.value_boolean ? t('jce.common.yes') : t('jce.common.no');
		return '—';
	}
</script>

<svelte:head>
	<title>{headTitle}</title>
	<meta name="description" content={headDescription} />
	<link rel="canonical" href={canonical} />
	<meta property="og:title" content={comparison.content.title} />
	<meta property="og:description" content={headDescription} />
	<meta property="og:type" content="article" />
	<meta property="og:url" content={canonical} />
	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={comparison.content.title} />
	<meta name="twitter:description" content={headDescription} />
	{@html safeJsonLdScript(compareJsonLd)}
	{#if compareTableJsonLd}
		{@html safeJsonLdScript(compareTableJsonLd)}
	{/if}
</svelte:head>

<div class="mx-auto max-w-5xl space-y-6 p-4">
	<div class="flex flex-wrap items-start justify-between gap-4">
		<div class="space-y-1">
			<a
				href="/compare"
				class="inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
			>
				<ArrowLeft size={16} /> {t('jce.compare.back')}
			</a>
			<h1 class="text-2xl font-bold text-[var(--color-text)] sm:text-3xl">{comparison.content.title}</h1>
			<p class="text-sm capitalize text-[var(--color-muted)]">
				{comparison.metadata.depth} · {comparison.content.status} · {comparison.content.visibility}
			</p>
		</div>
		{#if isOwner}
			<div class="flex flex-wrap gap-2">
				<button
					type="button"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
					onclick={edit}
				>
					<Pencil size={12} /> {t('jce.compare.edit_selection')}
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50"
					disabled={promoting}
					onclick={promote}
				>
					<Sparkles size={12} /> {promoting ? t('jce.common.promoting') : t('jce.compare.promote_to_detailed')}
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-red-500/10 px-3 py-1.5 text-xs font-medium text-red-500 transition-colors hover:bg-red-500/20 disabled:opacity-50"
					disabled={deleting}
					onclick={remove}
				>
					<Trash2 size={12} /> {deleting ? t('jce.common.deleting') : t('jce.common.delete')}
				</button>
			</div>
		{/if}
	</div>

	{#if error}
		<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
			{t(error)}
		</p>
	{/if}

	<section>
		<h2 class="mb-3 text-lg font-semibold text-[var(--color-text)]">{t('jce.compare.items_heading')}</h2>
		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each resolvedItems as item (item.id)}
				<div
					class="space-y-2 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
				>
					<h3 class="font-medium text-[var(--color-text)]">{item.title}</h3>
					{#if item.description}
						<p class="text-sm text-[var(--color-muted)]">{item.description}</p>
					{/if}
					{#if item.url}
						<a
							href={item.url}
							target="_blank" rel="noopener noreferrer"
							class="inline-flex items-center gap-1 text-xs text-[var(--color-text)] underline"
						>
							<ExternalLink size={12} /> {t('jce.common.view')}
						</a>
					{/if}
				</div>
			{/each}
		</div>
	</section>

	{#if matrix.length > 0}
		<section>
			<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
				<h2 class="text-lg font-semibold text-[var(--color-text)]">{t('jce.compare.matrix_heading')}</h2>
				<div class="flex flex-wrap items-center gap-2">
					{#if user && features['feature.jce.refresh']}
						<RefreshButton comparisonID={comparison.content.id} />
					{/if}
					{#if user && features['feature.jce.refresh'] && freshness}
						<FreshnessChip summary={freshness} />
					{/if}
				</div>
			</div>
			<div class="prose-table-wrapper">
				<table class="text-sm">
					<thead>
						<tr>
							<th class="text-[var(--color-text)]">{t('jce.compare.matrix_dimension')}</th>
							{#each matrix as item (item.id)}
								<th class="text-[var(--color-text)]">
									<div>{item.title}</div>
								</th>
							{/each}
						</tr>
					</thead>
					<tbody>
						{#each uniqueDims as dim (dim.id)}
							<tr>
								<td class="font-medium text-[var(--color-text)]">{dim.label}</td>
								{#each matrix as item (item.id)}
									<td class="text-[var(--color-muted)]">
										{formatValue(cellFor(item, dim))}
									</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}

	{#if comparison.metadata.depth === 'quick'}
		<!-- Localized notice; translations are admin-editable (untrusted) so the
		composed HTML is re-sanitized before render. The <strong> wrapper is in
		the allowlist so it survives; only the name payload is escaped. -->
		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 text-sm text-[var(--color-muted)]">
			{@html sanitizeContentHtml(t('jce.compare.quick_comparison_notice')
				.replace('{promote}', '<strong style="color: var(--color-text)">' + t('jce.compare.promote_to_detailed') + '</strong>'))}
		</div>
	{/if}
</div>
