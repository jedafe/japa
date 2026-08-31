<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import {
		getDataSource,
		updateDataSource,
		deleteDataSource
	} from '$lib/api/comparison-data-sources';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import DataSourceForm from '$lib/components/jce/DataSourceForm.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { ArrowLeft, Trash2 } from 'lucide-svelte';
	import type { DataSource } from '$lib/types/jce';

	const id = $derived(page.params.id ?? '');
	const canManage = $derived(hasPermission('jce.data_source.manage'));

	let source = $state<DataSource | null>(null);
	let loading = $state(true);
	let error = $state('');
	let saving = $state(false);
	let formValue = $state<Record<string, any>>({});
	let deleteOpen = $state(false);

	function sourceToForm(s: DataSource): Record<string, any> {
		return {
			source_name: s.source_name,
			source_kind: s.source_kind,
			source_url: s.source_url ?? '',
			entity_type: s.entity_type ?? '',
			entity_id: s.entity_id ?? '',
			dimension_slug: s.dimension_slug ?? '',
			trust_level: s.trust_level,
			config: s.config
		};
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const data = await getDataSource(id);
			if (data) {
				source = data;
				formValue = sourceToForm(data);
			} else {
				error = t('data_sources.not_found', 'Data source not found');
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('common.error', 'Failed to load data source');
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (id) {
			load();
		}
	});

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

	async function save() {
		if (!canManage) {
			error = t('common.permission_denied', 'Permission denied');
			return;
		}
		const normalized = normalizeBody(formValue);
		if (!normalized.ok) {
			error = normalized.error;
			return;
		}
		saving = true;
		error = '';
		const res = await updateDataSource(id, normalized.body);
		if (res.ok) {
			goto('/comparison-data-sources');
		} else {
			error = res.error;
			saving = false;
		}
	}

	async function confirmDelete() {
		if (!canManage) return;
		deleteOpen = false;
		const res = await deleteDataSource(id);
		if (res.ok) {
			goto('/comparison-data-sources');
		} else {
			error = res.error;
		}
	}
</script>

<svelte:head>
	<title>{t('data_sources.title', 'Comparison data sources')} — {t('common.admin_title_suffix', 'JapaConnect Admin')}</title>
</svelte:head>

<div class="space-y-6">
	<a
		href="/comparison-data-sources"
		class="inline-flex items-center gap-1.5 text-sm hover:opacity-70"
		style="color: var(--color-muted)"
	>
		<ArrowLeft class="h-4 w-4" />
		{t('data_sources.back_to_list', 'Back to data sources')}
	</a>

	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">
				{source?.source_name ?? t('data_sources.title', 'Comparison data sources')}
			</h1>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">
				{t('data_sources.description', 'Manage comparison data sources and their trust settings')}
			</p>
		</div>
		{#if canManage && source}
			<Button variant="destructive" onclick={() => (deleteOpen = true)}>
				<Trash2 class="mr-2 h-4 w-4" />
				{t('common.delete', 'Delete')}
			</Button>
		{/if}
	</div>

	{#if error}
		<div
			class="rounded-lg p-3 text-sm"
			style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
		>
			{error}
		</div>
	{/if}

	{#if loading}
		<p class="text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading…')}</p>
	{:else if source}
		<div class="rounded-lg border p-4" style="border-color: var(--color-border)">
			<DataSourceForm bind:value={formValue} editing={source} />

			<div class="mt-6 flex justify-end gap-2">
				<Button variant="outline" onclick={() => goto('/comparison-data-sources')}>
					{t('common.cancel', 'Cancel')}
				</Button>
				<Button onclick={save} disabled={saving || !canManage}>
					{saving ? t('common.saving', 'Saving…') : t('common.save', 'Save Changes')}
				</Button>
			</div>
		</div>
	{:else}
		<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border)">
			<p class="text-sm" style="color: var(--color-muted)">
				{t('data_sources.not_found', 'Data source not found')}
			</p>
		</div>
	{/if}
</div>

<Modal bind:open={deleteOpen} title={t('common.confirmDelete', 'Confirm Delete')}>
	<p class="text-sm" style="color: var(--color-muted)">
		{t('data_sources.delete_confirm', 'Are you sure you want to delete this data source? This action cannot be undone.')}
	</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (deleteOpen = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button variant="destructive" onclick={confirmDelete}>
			{t('common.delete', 'Delete')}
		</Button>
	</div>
</Modal>
