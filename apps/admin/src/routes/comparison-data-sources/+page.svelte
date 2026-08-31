<script lang="ts">
	// /comparison-data-sources — admin registry for JCE comparison data sources.
	// Uses the EntityPage scaffold; the form is a custom snippet because the config
	// field must be edited as JSON text and parsed before submission.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import {
		listDataSources,
		createDataSource,
		updateDataSource,
		deleteDataSource
	} from '$lib/api/comparison-data-sources';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import DataSourceForm from '$lib/components/jce/DataSourceForm.svelte';
	import type { DataSource } from '$lib/types/jce';
	import { Trash2 } from 'lucide-svelte';

	type Source = DataSource;

	const canManage = $derived(hasPermission('jce.data_source.manage'));

	const sourceKindOptions = $derived([
		{ value: 'official', label: t('data_sources.official', 'Official') },
		{ value: 'third_party', label: t('data_sources.third_party', 'Third party') },
		{ value: 'ai_estimate', label: t('data_sources.ai_estimate', 'AI estimate') }
	]);

	// Fields drive EntityForm seeding and create-form defaults. The actual
	// form UI is rendered via the DataSourceForm snippet below.
	const fields: FieldDef[] = $derived([
		{ key: 'source_name', label: t('data_sources.source_name', 'Source name'), type: 'text', required: true },
		{
			key: 'source_kind',
			label: t('data_sources.source_kind', 'Kind'),
			type: 'select',
			options: sourceKindOptions,
			required: true
		},
		{ key: 'source_url', label: t('data_sources.source_url', 'Source URL'), type: 'url' },
		{ key: 'entity_type', label: t('data_sources.entity_type', 'Entity type'), type: 'text' },
		{ key: 'entity_id', label: t('data_sources.entity_id', 'Entity ID'), type: 'text' },
		{ key: 'dimension_slug', label: t('data_sources.dimension_slug', 'Dimension slug'), type: 'text' },
		{ key: 'trust_level', label: t('data_sources.trust_level', 'Trust level'), type: 'number' }
	]);

	function parseConfig(value: Record<string, any>): { ok: true; config: Record<string, unknown> } | { ok: false; error: string } {
		const raw = value.config;
		if (raw == null || raw === '') return { ok: true, config: {} };
		if (typeof raw === 'string') {
			try {
				return { ok: true, config: JSON.parse(raw) };
			} catch {
				return { ok: false, error: t('data_sources.config_invalid', 'Config JSON is invalid') };
			}
		}
		if (typeof raw === 'object' && !Array.isArray(raw)) return { ok: true, config: raw };
		return { ok: false, error: t('data_sources.config_invalid', 'Config JSON is invalid') };
	}

	function normalizeBody(
		value: Record<string, any>
	): { ok: true; body: Record<string, any> } | { ok: false; error: string } {
		const source_name = String(value.source_name ?? '').trim();
		if (!source_name) {
			return { ok: false, error: t('data_sources.source_name_required', 'Source name is required') };
		}
		const source_kind = String(value.source_kind ?? '').trim();
		if (!source_kind) {
			return { ok: false, error: t('data_sources.source_kind_required', 'Source kind is required') };
		}
		const trustLevelNum = Number(value.trust_level ?? 3);
		const trust_level = Number.isNaN(trustLevelNum) ? 3 : Math.min(5, Math.max(1, trustLevelNum));
		const source_url = (value.source_url ?? '').trim() || null;
		const entity_type = (value.entity_type ?? '').trim() || null;
		const entity_id = (value.entity_id ?? '').trim() || null;
		const dimension_slug = (value.dimension_slug ?? '').trim() || null;

		const configRes = parseConfig(value);
		if (!configRes.ok) return { ok: false, error: configRes.error };

		return {
			ok: true,
			body: {
				source_name,
				source_kind,
				trust_level,
				source_url,
				entity_type,
				entity_id,
				dimension_slug,
				config: configRes.config
			}
		};
	}

	function applySort<T extends Record<string, any>>(data: T[], sort: string): T[] {
		if (!sort) return data;
		const [key, dir] = sort.split(':') as [string, 'asc' | 'desc'];
		if (!key) return data;
		const sign = dir === 'desc' ? -1 : 1;
		return [...data].sort((a, b) => {
			const av = a[key];
			const bv = b[key];
			if (av == null && bv == null) return 0;
			if (av == null) return 1 * sign;
			if (bv == null) return -1 * sign;
			if (typeof av === 'string' && typeof bv === 'string') return av.localeCompare(bv) * sign;
			if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * sign;
			if (typeof av === 'boolean' && typeof bv === 'boolean') return (Number(av) - Number(bv)) * sign;
			return String(av).localeCompare(String(bv)) * sign;
		});
	}

	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return;
		if (!canManage) {
			pageError = t('common.permission_denied', 'Permission denied');
			return;
		}
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => deleteDataSource(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete data sources');
		} finally {
			bulkDeleting = false;
		}
	}

	const config: EntityPageConfig = $derived.by(() => ({
		title: t('data_sources.title', 'Comparison data sources'),
		description: t('data_sources.description', 'Manage comparison data sources and their trust settings'),
		createLabel: t('data_sources.add', 'Add data source'),
		createPermission: 'jce.data_source.manage',
		pageSize: 25,
		columns: [
			{
				key: 'source_name',
				label: t('data_sources.source_name', 'Source name'),
				sortable: true,
				render: (v: string) => `<span class="font-medium" style="color: var(--color-foreground)">${v}</span>`
			},
			{
				key: 'source_kind',
				label: t('data_sources.source_kind', 'Kind'),
				render: (v: string) =>
					`<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: var(--color-muted-muted); color: var(--color-muted)">${t('data_sources.' + v, v)}</span>`
			},
			{
				key: 'trust_level',
				label: t('data_sources.trust_level', 'Trust level')
			},
			{
				key: 'freshness_score',
				label: t('data_sources.freshness_score', 'Freshness')
			},
			{
				key: 'source_url',
				label: t('data_sources.source_url', 'Source URL'),
				render: (v: string | null) =>
					v
						? `<a href="${v}" target="_blank" rel="noreferrer" class="text-xs underline" style="color: var(--color-primary)">${v}</a>`
						: '—'
			},
			{
				key: 'entity_type',
				label: t('data_sources.entity_type', 'Entity type'),
				render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>`
			},
			{
				key: 'dimension_slug',
				label: t('data_sources.dimension_slug', 'Dimension slug'),
				render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v || '—'}</span>`
			}
		],
		filters: [
			{
				key: 'source_kind',
				label: t('data_sources.source_kind', 'Kind'),
				options: sourceKindOptions
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			const { data, total } = await listDataSources({ page: page + 1, pageSize });
			let filtered: DataSource[] = data;
			if (filter.source_kind) {
				filtered = filtered.filter((s: DataSource) => s.source_kind === filter.source_kind);
			}
			const query = (q ?? '').trim().toLowerCase();
			if (query) {
				filtered = filtered.filter(
					(s: DataSource) =>
						s.source_name.toLowerCase().includes(query) ||
						(s.entity_type ?? '').toLowerCase().includes(query) ||
						(s.dimension_slug ?? '').toLowerCase().includes(query)
				);
			}
			filtered = applySort(filtered, sort);
			const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
			return { data: filtered, total, totalPages };
		},
		create: async (body) => {
			if (!canManage) return { ok: false, error: t('common.permission_denied', 'Permission denied') };
			const normalized = normalizeBody(body);
			if (!normalized.ok) return { ok: false, error: normalized.error };
			const res = await createDataSource(normalized.body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			if (!canManage) return { ok: false, error: t('common.permission_denied', 'Permission denied') };
			const normalized = normalizeBody(body);
			if (!normalized.ok) return { ok: false, error: normalized.error };
			const res = await updateDataSource(id, normalized.body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			if (!canManage) return { ok: false, error: t('common.permission_denied', 'Permission denied') };
			const res = await deleteDataSource(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: SourceView,
		formSnippet: SourceForm
	}));
</script>

<svelte:head>
	<title>{t('data_sources.title', 'Comparison data sources')} — {t('common.admin_title_suffix', 'JapaConnect Admin')}</title>
</svelte:head>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet SourceForm({ value, editing }: { value: Record<string, any>; editing: Source | null })}
	<DataSourceForm {value} {editing} />
{/snippet}

{#snippet SourceView({ row }: { row: Source })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('data_sources.source_name', 'Source name')}
			</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.source_name}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('data_sources.source_kind', 'Kind')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">
					{t('data_sources.' + row.source_kind, row.source_kind)}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('data_sources.trust_level', 'Trust level')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.trust_level}</p>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('data_sources.freshness_score', 'Freshness')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.freshness_score}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('data_sources.last_verified', 'Last verified')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.last_verified_at ?? '—'}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('data_sources.source_url', 'Source URL')}
			</p>
			{#if row.source_url}
				<a
					href={row.source_url}
					target="_blank"
					rel="noreferrer"
					class="mt-1 break-all text-sm underline"
					style="color: var(--color-primary)"
				>
					{row.source_url}
				</a>
			{:else}
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">—</p>
			{/if}
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('data_sources.entity_type', 'Entity type')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.entity_type ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('data_sources.dimension_slug', 'Dimension slug')}
				</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.dimension_slug ?? '—'}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('data_sources.config', 'Config (JSON)')}
			</p>
			<pre class="mt-1 max-h-48 overflow-auto rounded-md border p-2 text-xs font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{JSON.stringify(row.config, null, 2)}</pre>
		</div>
	</div>
{/snippet}
