<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /ads/networks — migrated to the EntityPage scaffold; CRUD entity with JSON textarea fields.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { adNetworksApi, type AdNetwork } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	// Ref: japaconnect-engineering/sections/25-ad-placement.md (Ad Networks)
	// Rule 16 — providers list fetched from the backend API, not hardcoded.
	let providers = $state<string[]>([]);
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	// Fetch allowed providers from the backend on mount.
	$effect(() => {
		adNetworksApi.listProviders().then((res) => {
			if (res.ok && res.data?.data) {
				providers = res.data.data;
			}
		});
	});

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => adNetworksApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete ad networks');
		} finally {
			bulkDeleting = false;
		}
	}

	// Reactive provider options — updates when the API response arrives.
	const providerOptions = $derived(providers.map((p) => ({ value: p, label: p })));

	// `slot_mapping`/`config` are JSON objects in the row; `load` stringifies them
	// so the textarea fields seed correctly. `api_key` is never returned by the
	// backend (security), so it seeds empty on edit — "leave empty to keep current".
	// `fields` is $derived so the provider <select> options update after the
	// providers API call resolves.
	const fields = $derived<FieldDef[]>([
		{ key: 'provider', label: t('networks.provider', 'Provider'), type: 'select', options: providerOptions, required: true },
		{ key: 'name', label: t('networks.name', 'Name'), type: 'text', placeholder: t('networks.name_placeholder', 'e.g. Production AdSense'), required: true },
		{ key: 'client_id', label: t('networks.client_id', 'Client ID'), type: 'text', placeholder: t('networks.client_id_placeholder', 'Optional') },
		{ key: 'api_key', label: t('networks.api_key', 'API Key'), type: 'password', placeholder: t('networks.api_key_keep', 'Leave empty to keep current key') },
		{ key: 'priority', label: t('networks.priority', 'Priority'), type: 'number', min: 0 },
		{ key: 'is_active', label: t('networks.active', 'Active'), type: 'checkbox' },
		{ key: 'slot_mapping', label: t('networks.slot_mapping', 'Slot Mapping'), type: 'textarea', placeholder: '{"leaderboard": "ca-pub-123/456"}' },
		{ key: 'config', label: t('networks.config', 'Config'), type: 'textarea', placeholder: '{"auto_ads": true, "ads_per_page": 3}' }
	]);

	function parseJsonField(raw: string, field: string): Record<string, unknown> | undefined {
		const trimmed = String(raw ?? '').trim();
		if (!trimmed) return undefined;
		try {
			const parsed = JSON.parse(trimmed);
			if (typeof parsed !== 'object' || Array.isArray(parsed)) {
				throw new Error(`${field}: ${t('networks.json_object_required', 'Must be a JSON object')}`);
			}
			return parsed as Record<string, unknown>;
		} catch (e) {
			throw new Error(`${field}: ${e instanceof Error ? e.message : t('networks.json_invalid', 'Invalid JSON')}`);
		}
	}

	// `config` is $derived so it picks up the reactive `fields` (which
	// depends on the providers API response).
	const config = $derived<EntityPageConfig>({
		title: t('networks.title', 'Ad Networks'),
		description: t('networks.description', 'Manage ad network integrations — configure providers, API keys, slot mappings, and priority'),
		createLabel: t('networks.create', 'Create Network'),
		createPermission: 'ad.network.manage',
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('networks.col_name', 'Name'),
				sortable: true,
				render: (v: string) => `<span class="font-medium">${v ?? '—'}</span>`
			},
			{
				key: 'provider',
				label: t('networks.col_provider', 'Provider'),
				sortable: true,
				render: (v: string) => {
					const colorMap: Record<string, string> = {
						adsense: 'var(--color-primary)',
						ezoic: 'var(--color-success)',
						media_net: 'var(--color-info)',
						custom: 'var(--color-muted)'
					};
					const c = colorMap[v] ?? 'var(--color-muted)';
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold" style="background: color-mix(in srgb, ${c} 15%, transparent); color: ${c}; border-color: ${c}">${v}</span>`;
				}
			},
			{
				key: 'client_id',
				label: t('networks.col_client_id', 'Client ID'),
				render: (v: string) => {
					if (!v) return `<span class="text-muted">—</span>`;
					const truncated = v.length > 20 ? v.slice(0, 20) + '…' : v;
					return `<span class="font-mono text-xs">${truncated}</span>`;
				}
			},
			{
				key: 'priority',
				label: t('networks.col_priority', 'Priority'),
				sortable: true,
				render: (v: number) => String(v)
			},
			{
				key: 'is_active',
				label: t('common.status', 'Status'),
				sortable: true,
				render: (v: boolean) =>
					v
						? `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-success/15 text-success border-success">${t('common.active', 'Active')}</span>`
						: `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold bg-surface text-foreground border-border">${t('common.inactive', 'Inactive')}</span>`
			}
		],
		filters: [
			{
				key: 'is_active',
				label: t('common.status', 'Status'),
				options: [
					{ value: 'true', label: t('common.active', 'Active') },
					{ value: 'false', label: t('common.inactive', 'Inactive') }
				]
			},
			{
				key: 'provider',
				label: t('networks.col_provider', 'Provider'),
				options: providerOptions
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			// The backend supports pagination but not text/provider/active filters,
			// so we fetch a large window and apply client-side search/filter/pagination.
			const params = new URLSearchParams();
			params.set('page_size', '1000');
			const result = await adNetworksApi.list(params.toString());
			if (result.ok) {
				const resp = result.data as Record<string, unknown> | undefined;
				let raw = (Array.isArray(resp) ? resp : (resp?.data as AdNetwork[]) ?? []) as AdNetwork[];
				if (filter.is_active) {
					const active = filter.is_active === 'true';
					raw = raw.filter((n) => n.is_active === active);
				}
				if (filter.provider) {
					raw = raw.filter((n) => n.provider === filter.provider);
				}
				const query = (q ?? '').trim().toLowerCase();
				if (query) {
					raw = raw.filter((n) =>
						n.name.toLowerCase().includes(query) ||
						n.provider.toLowerCase().includes(query)
					);
				}
				// Stringify JSON object fields so the textarea inputs seed correctly.
				const data = raw.map((n) => ({
					...n,
					slot_mapping: n.slot_mapping ? JSON.stringify(n.slot_mapping, null, 2) : '',
					config: n.config ? JSON.stringify(n.config, null, 2) : ''
				}));
				const total = data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				const start = page * pageSize;
				return { data: data.slice(start, start + pageSize), total, totalPages };
			}
			throw new Error(result.error ?? t('networks.load_error', 'Failed to load ad networks'));
		},
		create: async (body) => {
			const payload: Record<string, unknown> = {
				provider: body.provider ?? 'adsense',
				name: String(body.name ?? '').trim(),
				priority: Number(body.priority ?? 0) || 0,
				is_active: !!body.is_active
			};
			if (body.client_id != null && String(body.client_id).trim()) payload.client_id = String(body.client_id).trim();
			if (body.api_key != null && String(body.api_key).trim()) payload.api_key = String(body.api_key).trim();
			try {
				const slotMapping = parseJsonField(String(body.slot_mapping ?? ''), t('networks.slot_mapping', 'Slot Mapping'));
				if (slotMapping) payload.slot_mapping = slotMapping;
				const configParsed = parseJsonField(String(body.config ?? ''), t('networks.config', 'Config'));
				if (configParsed) payload.config = configParsed;
			} catch (e) {
				return { ok: false, error: e instanceof Error ? e.message : t('networks.json_invalid', 'Invalid JSON') };
			}
			const res = await adNetworksApi.create(payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = {
				provider: body.provider ?? 'adsense',
				name: String(body.name ?? '').trim(),
				priority: Number(body.priority ?? 0) || 0,
				is_active: !!body.is_active
			};
			if (body.client_id != null && String(body.client_id).trim()) payload.client_id = String(body.client_id).trim();
			if (body.api_key != null && String(body.api_key).trim()) payload.api_key = String(body.api_key).trim();
			try {
				const slotMapping = parseJsonField(String(body.slot_mapping ?? ''), t('networks.slot_mapping', 'Slot Mapping'));
				if (slotMapping) payload.slot_mapping = slotMapping;
				const configParsed = parseJsonField(String(body.config ?? ''), t('networks.config', 'Config'));
				if (configParsed) payload.config = configParsed;
			} catch (e) {
				return { ok: false, error: e instanceof Error ? e.message : t('networks.json_invalid', 'Invalid JSON') };
			}
			const res = await adNetworksApi.update(id, payload);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await adNetworksApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: NetworkView
	});
</script>

<svelte:head>
	<title>{t('networks.title', 'Ad Networks')} — {siteName()} Admin</title>
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

{#snippet NetworkView({ row }: { row: AdNetwork })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('networks.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('networks.provider', 'Provider')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.provider}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('networks.client_id', 'Client ID')}</p>
			<p class="mt-1 break-all font-mono text-sm" style="color: var(--color-foreground)">{row.client_id ?? '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('networks.priority', 'Priority')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.priority}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('networks.slot_mapping', 'Slot Mapping')}</p>
			<p class="mt-1 break-all font-mono text-xs" style="color: var(--color-foreground)">{row.slot_mapping ? JSON.stringify(row.slot_mapping, null, 2) : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('networks.config', 'Config')}</p>
			<p class="mt-1 break-all font-mono text-xs" style="color: var(--color-foreground)">{row.config ? JSON.stringify(row.config, null, 2) : '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_active ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}</p>
		</div>
	</div>
{/snippet}
