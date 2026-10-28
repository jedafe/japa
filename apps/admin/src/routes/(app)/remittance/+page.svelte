<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /remittance — migrated to the EntityPage scaffold: server-paginated
	// fetch loop + client-side q/sort/filter (the backend only supports
	// limit/offset/active_only), ActionMenu row actions, SlideOver forms.
	//
	// Ref: japaconnect-engineering/sections/74-remittance-tracker.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { Button } from '$lib/components/ui/button';
	import { Banknote, Pencil, Download } from 'lucide-svelte';
	import { api, apiRaw, walletApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	type RemittanceProvider = {
		id: string;
		name: string;
		slug: string;
		website_url: string;
		data_source: string;
		is_active: boolean;
		is_benchmark: boolean;
		display_order: number;
		created_at: string;
	};

	const canManage = $derived(hasPermission('remittance.manage'));
	const canFetch = $derived(hasPermission('remittance.fetch'));

	let entityController = $state<EntityPageController | undefined>(undefined);
	let message = $state('');
	let messageType = $state<'success' | 'error' | ''>('');

	// ── Manual rate entry ────────────────────────────────────────────────────
	// POST /admin/remittance/providers/{id}/rates — the only write path for
	// manual data_source providers; the route exists in §74 but was never
	// consumed by the UI.
	let rateProvider = $state<RemittanceProvider | null>(null);
	let rateSlideOverOpen = $state(false);
	let rateForm = $state({
		send_currency: '',
		receive_currency: '',
		rate: '',
		fee_flat: '',
		fee_pct: '',
		transfer_time: '',
		source_url: ''
	});
	let rateSaving = $state(false);

	function openRateEntry(provider: RemittanceProvider) {
		rateProvider = provider;
		rateSlideOverOpen = true;
		rateForm = {
			send_currency: '',
			receive_currency: '',
			rate: '',
			fee_flat: '',
			fee_pct: '',
			transfer_time: '',
			source_url: ''
		};
	}

	async function submitManualRate() {
		if (!rateProvider) return;
		const rate = parseFloat(rateForm.rate);
		if (!(rate > 0) || !rateForm.send_currency || !rateForm.receive_currency) return;
		rateSaving = true;
		message = '';
		try {
			const result = await api(`/admin/remittance/providers/${rateProvider.id}/rates`, {
				method: 'POST',
				body: {
					send_currency: rateForm.send_currency,
					receive_currency: rateForm.receive_currency,
					rate,
					fee_flat: parseFloat(rateForm.fee_flat) || 0,
					fee_pct: parseFloat(rateForm.fee_pct) || 0,
					transfer_time: rateForm.transfer_time || null,
					source_url: rateForm.source_url || null
				}
			});
			if (result.ok) {
				messageType = 'success';
				message = t('remittance.rate_added', 'Manual rate recorded.');
				rateSlideOverOpen = false;
				rateProvider = null;
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('remittance.rate_error', 'Failed to record rate');
		} finally {
			rateSaving = false;
		}
	}

	async function triggerFetch(id: string) {
		message = '';
		try {
			const result = await api(`/admin/remittance/providers/${id}/fetch`, { method: 'POST' });
			if (result.ok) {
				messageType = 'success';
				message = t('remittance.fetch_triggered', 'Rate fetch triggered.');
			} else {
				messageType = 'error';
				message = result.error;
			}
		} catch (e) {
			messageType = 'error';
			message = e instanceof Error ? e.message : t('remittance.fetch_error', 'Failed to trigger fetch');
		}
	}

	// ── Currency pickers (rate entry) — /currencies managed list (§58). ─────
	let currencyItems = $state<AutocompleteItem[] | null>(null);

	async function fetchCurrencies(q: string): Promise<AutocompleteItem[]> {
		if (currencyItems === null) {
			const res = await walletApi.getCurrencies();
			currencyItems = res.ok
				? (res.data ?? []).map((c) => ({ id: c.code, label: c.name ? `${c.code} — ${c.name}` : c.code }))
				: [];
		}
		const query = q.toLowerCase();
		return currencyItems.filter((c) => query === '' || c.label.toLowerCase().includes(query));
	}
	function fetchCurrenciesInitial(): Promise<AutocompleteItem[]> {
		return fetchCurrencies('');
	}

	// data_source mirrors the remittance_providers CHECK constraint
	// ('api','manual') — §74 schema.
	const dataSourceOptions = [
		{ value: 'manual', label: t('remittance.data_source_manual', 'Manual') },
		{ value: 'api', label: t('remittance.data_source_api', 'API') }
	];

	const fields: FieldDef[] = [
		{ key: 'name', label: t('remittance.name', 'Name'), type: 'text', help: t('remittance.name_hint', 'The name shown in admin lists and on the site where public.'), required: true },
		{
			key: 'slug',
			label: t('remittance.slug', 'Slug'),
			type: 'text',
			placeholder: 'wise',
			required: true,
			readonlyInEdit: true,
			help: t('remittance.slug_hint', 'Immutable after creation.')
		},
		{ key: 'website_url', label: t('remittance.website', 'Website URL'), type: 'url', help: t('remittance.website_url_hint', 'Full URL including https://.'), placeholder: 'https://wise.com' },
		{
			key: 'data_source',
			label: t('remittance.data_source', 'Data Source'),
			type: 'select',
			help: t('remittance.data_source_hint', 'Where this data comes from — API, file, manual.'),
			options: dataSourceOptions,
			required: true,
			readonlyInEdit: true
		},
		{
			key: 'api_config',
			label: t('remittance.api_config', 'API Config (JSON)'),
			type: 'textarea',
			placeholder: '{"api_key": "…"}',
			help: t('remittance.api_config_hint', 'Provider credentials for API data_source — stored encrypted. Leave empty on edit to keep existing.')
		},
		{ key: 'display_order', label: t('remittance.order', 'Display Order'), type: 'number', help: t('remittance.display_order_hint', 'Lower numbers appear first.'), min: 0 },
		{ key: 'is_active', label: t('remittance.active', 'Active'), type: 'checkbox', help: t('remittance.is_active_hint', 'Inactive records are hidden or disabled on the site.') },
		{ key: 'is_benchmark', label: t('remittance.is_benchmark', 'Benchmark (mid-market reference)'), type: 'checkbox', help: t('remittance.is_benchmark_hint', 'Benchmark rows anchor comparisons.') }
	];

	const actions: ActionItem[] = [
		{
			label: t('common.edit', 'Edit'),
			icon: Pencil,
			onclick: (row: RemittanceProvider, ctx) => ctx?.openEdit(row),
			show: () => canManage
		},
		{
			label: t('remittance.enter_rate', 'Enter Rate'),
			icon: Banknote,
			onclick: (row: RemittanceProvider) => openRateEntry(row),
			show: (row: RemittanceProvider) => canManage && row.data_source === 'manual'
		},
		{
			label: t('remittance.fetch', 'Fetch Rates'),
			icon: Download,
			onclick: (row: RemittanceProvider) => triggerFetch(row.id),
			show: (row: RemittanceProvider) => canFetch && row.data_source === 'api'
		}
	];

	const config: EntityPageConfig = {
		title: t('remittance.title', 'Remittance Rate Tracker'),
		description: t('remittance.description', 'Manage remittance providers and trigger rate fetches.'),
		createLabel: t('remittance.create', 'Add Provider'),
		createPermission: 'remittance.manage',
		pageSize: 25,
		columns: [
			{
				key: 'name',
				label: t('remittance.name', 'Name'),
				sortable: true,
				render: (value: string) => `<span class="font-medium text-foreground">${value}</span>`
			},
			{
				key: 'slug',
				label: t('remittance.slug', 'Slug'),
				sortable: true,
				render: (value: string) => `<span class="font-mono text-xs text-muted-foreground">${value}</span>`
			},
			{
				key: 'data_source',
				label: t('remittance.data_source', 'Data Source'),
				sortable: true,
				render: (value: string) => `<span class="text-foreground">${value}</span>`
			},
			{
				key: 'website_url',
				label: t('remittance.website', 'Website'),
				render: (value: string) =>
					value
						? `<a href="${value}" target="_blank" rel="noopener" class="text-primary text-xs hover:underline">${value}</a>`
						: '<span class="text-muted-foreground text-xs">—</span>'
			},
			{
				key: 'is_active',
				label: t('remittance.active', 'Active'),
				sortable: true,
				render: (value: boolean) =>
					value
						? `<span class="inline-flex items-center rounded-md border border-success bg-success/15 px-2 py-0.5 text-xs font-medium text-success">${t('common.active', 'Active')}</span>`
						: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">${t('common.inactive', 'Inactive')}</span>`
			},
			{
				key: 'is_benchmark',
				label: t('remittance.benchmark', 'Benchmark'),
				sortable: true,
				render: (value: boolean) =>
					value
						? `<span class="inline-flex items-center rounded-md border border-info bg-info/15 px-2 py-0.5 text-xs font-medium text-info">${t('remittance.mid_market', 'Mid-market')}</span>`
						: `<span class="inline-flex items-center rounded-md border border-border bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">—</span>`
			},
			{
				key: 'display_order',
				label: t('remittance.order', 'Order'),
				sortable: true,
				render: (value: number) => `<span class="font-mono text-foreground">${value}</span>`
			}
		],
		filters: [
			{
				key: 'data_source',
				label: t('remittance.data_source', 'Data Source'),
				options: dataSourceOptions
			},
			{
				key: 'is_active',
				label: t('remittance.status', 'Status'),
				options: [
					{ value: 'true', label: t('remittance.active', 'Active') },
					{ value: 'false', label: t('remittance.inactive', 'Inactive') }
				]
			}
		],
		actions,
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			// The backend supports limit/offset only (no q/sort) — page through
			// the full provider list, then apply search/filter/sort client-side.
			// The list is small and curated (providers are admin-managed).
			const all: RemittanceProvider[] = [];
			const limit = 100;
			let offset = 0;
			let total = 0;
			do {
				const res = await apiRaw<{
					data: RemittanceProvider[];
					pagination: { total: number; limit: number; offset: number };
				}>(`/admin/remittance/providers?limit=${limit}&offset=${offset}`);
				if (!res.ok) throw new Error(res.error ?? t('remittance.load_error', 'Failed to load remittance providers'));
				const batch = res.data?.data ?? [];
				all.push(...batch);
				total = res.data?.pagination?.total ?? batch.length;
				offset += batch.length;
			} while (offset < total);

			let filtered = all;
			if (filter.data_source) {
				filtered = filtered.filter((p) => p.data_source === filter.data_source);
			}
			if (filter.is_active) {
				const want = filter.is_active === 'true';
				filtered = filtered.filter((p) => p.is_active === want);
			}
			const query = (q ?? '').trim().toLowerCase();
			if (query) {
				filtered = filtered.filter(
					(p) =>
						p.name.toLowerCase().includes(query) ||
						p.slug.toLowerCase().includes(query) ||
						(p.website_url ?? '').toLowerCase().includes(query)
				);
			}
			if (sort) {
				const [key, dir] = (sort as string).split(':') as [keyof RemittanceProvider, 'asc' | 'desc'];
				if (key) {
					const sign = dir === 'desc' ? -1 : 1;
					filtered = [...filtered].sort((a, b) => {
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
			}
			const filteredTotal = filtered.length;
			const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(filteredTotal / pageSize)) : 1;
			const start = page * pageSize;
			return { data: filtered.slice(start, start + pageSize), total: filteredTotal, totalPages };
		},
		create: async (body) => {
			let apiConfig: unknown;
			const rawConfig = String(body.api_config ?? '').trim();
			if (rawConfig) {
				try {
					apiConfig = JSON.parse(rawConfig);
				} catch {
					return { ok: false, error: t('remittance.api_config_invalid', 'API config must be valid JSON') };
				}
			}
			const res = await api('/admin/remittance/providers', {
				method: 'POST',
				body: {
					name: String(body.name ?? '').trim(),
					slug: String(body.slug ?? '').trim(),
					website_url: String(body.website_url ?? '').trim() || null,
					data_source: String(body.data_source ?? 'manual'),
					api_config: apiConfig,
					is_active: body.is_active === undefined ? true : !!body.is_active,
					is_benchmark: !!body.is_benchmark,
					display_order: Number(body.display_order ?? 0)
				}
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = {
				name: String(body.name ?? '').trim(),
				website_url: String(body.website_url ?? '').trim() || null,
				is_active: !!body.is_active,
				is_benchmark: !!body.is_benchmark,
				display_order: Number(body.display_order ?? 0)
			};
			// api_config only sent when the admin typed new JSON — an empty
			// field keeps the stored credentials.
			const rawConfig = String(body.api_config ?? '').trim();
			if (rawConfig) {
				try {
					payload.api_config = JSON.parse(rawConfig);
				} catch {
					return { ok: false, error: t('remittance.api_config_invalid', 'API config must be valid JSON') };
				}
			}
			const res = await api(`/admin/remittance/providers/${id}`, { method: 'PATCH', body: payload });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		}
	};
</script>

<svelte:head>
	<title>{t('remittance.title', 'Remittance Rate Tracker')} — {siteName()} Admin</title>
</svelte:head>

{#if message}
	<div
		class="mb-4 rounded-lg border p-3 text-sm"
		style={messageType === 'success'
			? 'border-color: var(--color-success); background: color-mix(in srgb, var(--color-success) 10%, transparent); color: var(--color-success)'
			: 'border-color: var(--color-danger); background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)'}
	>
		{message}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- Manual rate entry — POST /admin/remittance/providers/{id}/rates (§74). -->
<SlideOver bind:open={rateSlideOverOpen} title={t('remittance.enter_rate_title', 'Enter Manual Rate')}>
	{#if rateProvider}
		<p class="mb-3 text-sm" style="color: var(--color-muted)">{rateProvider.name}</p>
		<div class="space-y-3">
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="rate-send" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.send_currency', 'Send currency')}</label>
					<Autocomplete
						placeholder={t('remittance.send_currency', 'Send currency')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCurrencies}
						initialFetcher={fetchCurrenciesInitial}
						selectedId={rateForm.send_currency || null}
						selectedLabel={rateForm.send_currency || null}
						onSelect={(item) => (rateForm.send_currency = item.id)}
					/>
				</div>
				<div>
					<label for="rate-receive" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.receive_currency', 'Receive currency')}</label>
					<Autocomplete
						placeholder={t('remittance.receive_currency', 'Receive currency')}
						emptyMessage={t('common.no_results', 'No results found')}
						fetcher={fetchCurrencies}
						initialFetcher={fetchCurrenciesInitial}
						selectedId={rateForm.receive_currency || null}
						selectedLabel={rateForm.receive_currency || null}
						onSelect={(item) => (rateForm.receive_currency = item.id)}
					/>
				</div>
			</div>
			<div>
				<label for="rate-value" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.rate', 'Rate')}</label>
				<input id="rate-value" type="number" step="any" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={rateForm.rate} />
			</div>
			<div class="grid grid-cols-2 gap-3">
				<div>
					<label for="rate-fee-flat" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.fee_flat', 'Fee (flat, send ccy)')}</label>
					<input id="rate-fee-flat" type="number" step="any" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={rateForm.fee_flat} />
				</div>
				<div>
					<label for="rate-fee-pct" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.fee_pct', 'Fee (%)')}</label>
					<input id="rate-fee-pct" type="number" step="any" min="0" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={rateForm.fee_pct} />
				</div>
			</div>
			<div>
				<label for="rate-transfer-time" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.transfer_time', 'Transfer time')}</label>
				<input id="rate-transfer-time" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={rateForm.transfer_time} placeholder={t('remittance.transfer_time_hint', 'e.g. minutes, same day, 1-2 days')} />
			</div>
			<div>
				<label for="rate-source-url" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">{t('remittance.source_url', 'Source URL')}</label>
				<input id="rate-source-url" type="url" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)" bind:value={rateForm.source_url} />
			</div>
		</div>
	{/if}
	{#snippet footer()}
		<div class="mt-6 flex justify-end gap-2">
			<Button variant="outline" onclick={() => (rateSlideOverOpen = false)}>{t('common.cancel', 'Cancel')}</Button>
			<Button onclick={submitManualRate} disabled={rateSaving || !rateForm.send_currency || !rateForm.receive_currency || !(parseFloat(rateForm.rate) > 0)}>
				{rateSaving ? t('common.saving', 'Saving...') : t('common.save', 'Save')}
			</Button>
		</div>
	{/snippet}
</SlideOver>
