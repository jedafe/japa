<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/stores';
	import { deleteComparison, promoteComparison } from '$lib/api/jce';
	import { t } from '$lib/i18n';
	import type { JCEDimension, MatrixDimension, MatrixItem, ResolvedItem } from '$lib/types/jce';
	import type { PageData } from './$types';
	import { ArrowLeft, Pencil, Sparkles, Trash2, ExternalLink } from 'lucide-svelte';

	let { data }: { data: PageData } = $props();

	let comparison = $derived(data.comparison);
	let matrix = $derived(data.matrix ?? []);
	let resolvedItems = $derived(data.resolvedItems ?? []);
	let isOwner = $derived(data.isOwner ?? false);
	let mainOrigin = $derived(data.mainOrigin ?? 'http://localhost:3000');
	let typeSlug = $derived(data.typeSlug ?? '');

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

	async function promote() {
		if (!isOwner) return;
		promoting = true;
		error = '';
		try {
			const promoted = await promoteComparison(comparison.content.id);
			if (promoted?.id) {
				window.location.href = `${mainOrigin}/create?type=comparison&draft=${promoted.id}`;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('jce.compare.errors.promote_failed');
		} finally {
			promoting = false;
		}
	}

	function edit() {
		const ids = resolvedItems.map((i: ResolvedItem) => i.id).join(',');
		const url = new URL('/compare', $page.url);
		if (typeSlug) url.searchParams.set('type', typeSlug);
		url.searchParams.set('preselect', ids);
		goto(url);
	}

	function cellFor(item: MatrixItem, dim: JCEDimension): MatrixDimension | undefined {
		return item.dimensions.find((d: MatrixDimension) => d.dimension_id === dim.id);
	}

	function formatValue(cell?: MatrixDimension) {
		if (!cell) return '—';
		if (cell.value !== undefined && cell.value !== null) return cell.value;
		if (cell.value_rating !== undefined && cell.value_rating !== null) return cell.value_rating;
		if (cell.value_boolean !== undefined && cell.value_boolean !== null) return cell.value_boolean ? t('jce.common.yes') : t('jce.common.no');
		return '—';
	}
</script>

<div class="max-w-5xl mx-auto p-4 sm:p-6 space-y-6">
	<div class="flex items-start justify-between gap-4 flex-wrap">
		<div class="space-y-1">
			<a
				href="/compare"
				class="inline-flex items-center gap-1 text-sm transition-colors hover:opacity-80"
				style="color: var(--color-muted)"
			>
				<ArrowLeft size={14} /> {t('jce.compare.back')}
			</a>
			<h1 class="text-2xl sm:text-3xl font-bold" style="color: var(--color-text)">{comparison.content.title}</h1>
			<p class="text-sm capitalize" style="color: var(--color-muted)">
				{comparison.metadata.depth} · {comparison.content.status} · {comparison.content.visibility}
			</p>
		</div>
		{#if isOwner}
			<div class="flex flex-wrap gap-2">
				<button
					type="button"
					class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium transition-colors"
					style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
					onclick={edit}
				>
					<Pencil size={12} /> {t('jce.compare.edit_selection')}
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium transition-colors disabled:opacity-50"
					style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
					disabled={promoting}
					onclick={promote}
				>
					<Sparkles size={12} /> {promoting ? t('jce.common.promoting') : t('jce.compare.promote_to_detailed')}
				</button>
				<button
					type="button"
					class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-button)] text-xs font-medium transition-colors disabled:opacity-50"
					style="background-color: rgba(239,68,68,0.1); color: #ef4444"
					disabled={deleting}
					onclick={remove}
				>
					<Trash2 size={12} /> {deleting ? t('jce.common.deleting') : t('jce.common.delete')}
				</button>
			</div>
		{/if}
	</div>

	{#if error}
		<div class="rounded-[var(--radius-button)] p-4 text-sm" role="alert" style="background-color: rgba(239,68,68,0.1); color: #ef4444; border: 1px solid rgba(239,68,68,0.2)">
			{t(error)}
		</div>
	{/if}

	<section>
		<h2 class="text-lg font-semibold mb-3" style="color: var(--color-text)">{t('jce.compare.items_heading')}</h2>
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
			{#each resolvedItems as item (item.id)}
				<div
					class="rounded-[var(--radius-card)] border p-4 space-y-2"
					style="background-color: var(--color-surface); border-color: var(--color-border)"
				>
					<h3 class="font-medium" style="color: var(--color-text)">{item.title}</h3>
					{#if item.description}
						<p class="text-sm" style="color: var(--color-muted)">{item.description}</p>
					{/if}
					{#if item.url}
						<a
							href={item.url}
							target="_blank"
							class="inline-flex items-center gap-1 text-xs underline"
							style="color: var(--color-text)"
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
			<h2 class="text-lg font-semibold mb-3" style="color: var(--color-text)">{t('jce.compare.matrix_heading')}</h2>
			<div class="overflow-x-auto rounded-[var(--radius-card)] border" style="border-color: var(--color-border)">
				<table class="min-w-full text-sm">
					<thead>
						<tr style="background-color: var(--color-bg); border-bottom: 1px solid var(--color-border)">
							<th class="p-3 text-left font-medium" style="color: var(--color-text)">{t('jce.compare.matrix_dimension')}</th>
							{#each matrix as item (item.id)}
								<th class="p-3 text-left font-medium" style="color: var(--color-text)">{item.title}</th>
							{/each}
						</tr>
					</thead>
					<tbody class="divide-y" style="border-color: var(--color-border)">
						{#each comparison.dimensions as dim (dim.id)}
							<tr style="background-color: var(--color-surface)">
								<td class="p-3 font-medium" style="color: var(--color-text)">{dim.label}</td>
								{#each matrix as item (item.id)}
									<td class="p-3" style="color: var(--color-muted)">{formatValue(cellFor(item, dim))}</td>
								{/each}
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>
	{/if}

	{#if comparison.metadata.depth === 'quick'}
		<div class="rounded-[var(--radius-card)] border p-4 text-sm" style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-muted)">
			{@html t('jce.compare.quick_comparison_notice')
				.replace('{promote}', '<strong style="color: var(--color-text)">' + t('jce.compare.promote_to_detailed') + '</strong>')}
		</div>
	{/if}
</div>
