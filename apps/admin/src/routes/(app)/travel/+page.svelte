<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /travel — Travel Search providers (Section 67, ADR-085).
	// Migrated to the EntityPage scaffold. Uses `formSnippet` for structured
	// config editing — api_config renders as endpoint/key inputs plus an
	// `extra` key-value editor, affiliate_config as a key-value editor —
	// because EntityForm has no map-editor field type. Serialization helpers
	// (buildApiConfig/pairsToObject) translate rows back into the API's flat
	// object contract; api_config is write-only, so credential fields start
	// blank on edit (blank = keep existing; extra-only merges server-side).
	//
	// Ref: japaconnect-engineering/sections/67-travel-search.md
	//      japaconnect-engineering/adr/ADR-085-travel-search.md
	import { EntityPage, type EntityPageConfig, type FieldDef } from '$lib/components/entity';
	import { travelAdminApi, adminCatalogApi, type TravelProvider, type CatalogOption } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Autocomplete from '$lib/components/ui/Autocomplete.svelte';
	import type { AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';

	// Key/value rows edited by the config form (api_config.extra and
	// affiliate_config are flat string maps in the API contract).
	type ConfigPair = { key: string; value: string };

	function objectToPairs(obj: Record<string, unknown> | null | undefined): ConfigPair[] {
		return Object.entries(obj ?? {}).map(([key, v]) => ({
			key,
			value: typeof v === 'string' ? v : JSON.stringify(v)
		}));
	}

	function pairsToObject(pairs: ConfigPair[] | undefined): Record<string, string> | undefined {
		const out: Record<string, string> = {};
		for (const p of pairs ?? []) {
			const key = p.key.trim();
			if (key) out[key] = p.value;
		}
		return Object.keys(out).length > 0 ? out : undefined;
	}

	// api_config payload: {endpoint, api_key, extra?} (ADR-085). All fields
	// blank → undefined, which means "leave existing credentials unchanged"
	// on PATCH (api_config is write-only and never returned by the API).
	function buildApiConfig(body: Record<string, any>): Record<string, unknown> | undefined {
		const endpoint = String(body.api_endpoint ?? '').trim();
		const apiKey = String(body.api_key ?? '').trim();
		const extra = pairsToObject(body.api_extra_pairs as ConfigPair[] | undefined);
		if (!endpoint && !apiKey && !extra) return undefined;
		return { endpoint, api_key: apiKey, ...(extra ? { extra } : {}) };
	}

	// Normalize an API row for the scaffold. affiliate_config arrives as a
	// JSON object (or null) → convert to key/value rows for the editor.
	// api_config is never returned by the backend (write-only) — leave blank.
	function mapProvider(raw: TravelProvider): TravelProvider & {
		affiliate_pairs: ConfigPair[];
		api_endpoint: string;
		api_key: string;
		api_extra_pairs: ConfigPair[];
	} {
		return {
			...raw,
			affiliate_pairs: objectToPairs(raw.affiliate_config),
			api_endpoint: '',
			api_key: '',
			api_extra_pairs: []
		};
	}

	let travelProviderTypeOptions = $state<CatalogOption[]>([
		{ value: 'flight', label: t('travel.type_flight', 'Flights') },
		{ value: 'hotel', label: t('travel.type_hotel', 'Hotels') },
		{ value: 'both', label: t('travel.type_both', 'Flights & Hotels') }
	]);

	$effect(() => {
		void (async () => {
			const res = await adminCatalogApi.get('travel.provider_type');
			if (res.ok && res.data?.length) travelProviderTypeOptions = res.data;
		})();
	});

	const providerTypeLabel = (v: string) =>
		travelProviderTypeOptions.find((o) => o.value === v)?.label ?? v;

	// `fields` drives EntityForm rendering + edit-form value seeding. The
	// JSON config fields use `textarea`; the snippet below overrides their
	// rendering but the fields array is still required for seeding.
	const fields = $derived<FieldDef[]>([
		{ key: 'name', label: t('travel.name', 'Name'), type: 'text', required: true, placeholder: 'Skyscanner' },
		{ key: 'slug', label: t('travel.slug', 'Slug'), type: 'text', required: true, placeholder: 'skyscanner', readonlyInEdit: true },
		{
			key: 'provider_type',
			label: t('travel.type', 'Provider Type'),
			type: 'select',
			required: true,
			options: travelProviderTypeOptions
		},
		{ key: 'api_endpoint', label: t('travel.api_endpoint', 'API Endpoint'), type: 'text' },
		{ key: 'api_key', label: t('travel.api_key', 'API Key'), type: 'text' },
		{ key: 'api_extra_pairs', label: t('travel.api_extra', 'Extra Settings'), type: 'textarea' },
		{ key: 'affiliate_pairs', label: t('travel.affiliate_config', 'Affiliate Params'), type: 'textarea' },
		{ key: 'is_active', label: t('travel.active', 'Active'), type: 'checkbox' }
	]);

	const config = $derived<EntityPageConfig>({
		title: t('travel.title', 'Travel Search'),
		description: t('travel.description', 'Manage travel search providers and configuration.'),
		createLabel: t('travel.create', 'Add Provider'),
		createPermission: 'travel.manage',
		editPermission: 'travel.manage',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('travel.name', 'Name'), sortable: true },
			{
				key: 'slug',
				label: t('travel.slug', 'Slug'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'provider_type',
				label: t('travel.type', 'Type'),
				sortable: true,
				render: (v: string) => providerTypeLabel(v)
			},
			{ key: 'is_active', label: t('travel.active', 'Active'), sortable: true, componentSnippet: StatusCell },
			{
				key: 'created_at',
				label: t('travel.created', 'Created'),
				sortable: true,
				render: (v: string) =>
					`<span class="text-xs" style="color: var(--color-muted)">${v ? new Date(v).toLocaleDateString() : '—'}</span>`
			}
		],
		filters: [
			{
				key: 'is_active',
				label: t('travel.active', 'Active'),
				options: [
					{ value: 'true', label: t('travel.active_label', 'Active') },
					{ value: 'false', label: t('travel.inactive_label', 'Inactive') }
				]
			},
			{
				key: 'provider_type',
				label: t('travel.type', 'Type'),
				options: travelProviderTypeOptions
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort, filter }) => {
			// The list contract (§67) accepts is_active/provider_type/q only —
			// no page/page_size/sort params — so the API filters the list and
			// sorting + pagination are applied client-side on the result.
			const params = new URLSearchParams();
			if (q) params.set('q', q);
			if (filter?.is_active) params.set('is_active', filter.is_active);
			if (filter?.provider_type) params.set('provider_type', filter.provider_type);
			const result = await travelAdminApi.list(params.toString());
			if (result.ok) {
				let data = (result.data as TravelProvider[]) ?? [];
				// EntityPage emits sort as "key:dir" — apply it here because the
				// backend list query is fixed `ORDER BY name ASC`.
				const [sortKey, sortDir] = (sort || '').split(':');
				if (sortKey) {
					const dirMul = sortDir === 'desc' ? -1 : 1;
					data = [...data].sort((a, b) => {
						const av = a[sortKey as keyof TravelProvider] as unknown;
						const bv = b[sortKey as keyof TravelProvider] as unknown;
						if (av == null && bv == null) return 0;
						if (av == null) return 1;
						if (bv == null) return -1;
						if (typeof av === 'boolean' || typeof bv === 'boolean') {
							return (Number(av) - Number(bv)) * dirMul;
						}
						if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dirMul;
						return String(av).localeCompare(String(bv)) * dirMul;
					});
				}
				const mapped = data.map(mapProvider);
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = mapped.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				return {
					data: mapped.slice(offset, offset + size),
					total,
					totalPages
				};
			}
			throw new Error(result.error ?? t('travel.load_error', 'Failed to load travel providers'));
		},
		create: async (body) => {
			const name = String(body.name ?? '').trim();
			const slug = String(body.slug ?? '').trim();
			const providerType = String(body.provider_type ?? '');
			if (!name || !slug || !providerType) {
				return {
					ok: false,
					error: t('travel.required_fields', 'Name, slug and provider type are required.')
				};
			}
			const res = await travelAdminApi.create({
				name,
				slug,
				provider_type: providerType,
				api_config: buildApiConfig(body),
				affiliate_config: pairsToObject(body.affiliate_pairs as ConfigPair[] | undefined),
				is_active: body.is_active === undefined ? true : !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (_id, body) => {
			const name = String(body.name ?? '').trim();
			const providerType = String(body.provider_type ?? '');
			if (!name || !providerType) {
				return {
					ok: false,
					error: t('travel.required_fields_short', 'Name and provider type are required.')
				};
			}
			const res = await travelAdminApi.update(_id, {
				name,
				provider_type: providerType,
				api_config: buildApiConfig(body),
				// Send the editor state verbatim — removing every row clears
				// the stored affiliate params (empty object on PATCH).
				affiliate_config:
					pairsToObject(body.affiliate_pairs as ConfigPair[] | undefined) ?? {},
				is_active: body.is_active === undefined ? undefined : !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await travelAdminApi.remove(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error ?? t('travel.delete_error', 'Failed to delete provider') };
		},
		viewSnippet: ProviderView,
		formSnippet: ConfigForm
	});
</script>

<svelte:head>
	<title>{t('travel.title', 'Travel Search')} — {siteName()} Admin</title>
</svelte:head>

<EntityPage {config} />

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet StatusCell({ value }: { value: boolean; row: TravelProvider })}
	<span
		class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium {value
			? 'border-success bg-success/15 text-success'
			: 'border-border bg-muted text-muted-foreground'}"
	>
		{value ? t('travel.active_label', 'Active') : t('travel.inactive_label', 'Inactive')}
	</span>
{/snippet}

<!-- Config form — api_config edits as endpoint/key/extra rows and
     affiliate_config as key/value parameter rows (no raw JSON). -->
{#snippet ConfigForm({ value, editing }: { value: Record<string, any>; editing: TravelProvider | null })}
	<div class="space-y-3">
		<div>
			<label for="travel-name" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">
				{t('travel.name', 'Name')}
			</label>
			<input
				id="travel-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.name}
				placeholder="Skyscanner"
			/>
		</div>
		<div>
			<label for="travel-slug" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">
				{t('travel.slug', 'Slug')}
			</label>
			<input
				id="travel-slug"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.slug}
				placeholder="skyscanner"
				readonly={!!editing}
				disabled={!!editing}
			/>
		</div>
		<div>
			<label for="travel-type" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">
				{t('travel.type', 'Provider Type')}
			</label>
			<Autocomplete
				id="travel-type"
				class="w-full"
				placeholder={t('travel.select_type', 'Select provider type')}
				emptyMessage={t('common.no_results', 'No results found')}
				fetcher={async (query: string) => {
					const needle = query.trim().toLowerCase();
					return travelProviderTypeOptions
						.filter((o) => !needle || o.label.toLowerCase().includes(needle) || o.value.toLowerCase().includes(needle))
						.map((o) => ({ id: o.value, label: o.label }));
				}}
				initialFetcher={async () =>
					travelProviderTypeOptions.map((o) => ({ id: o.value, label: o.label }))}
				selectedId={value.provider_type || null}
				selectedLabel={value.provider_type ? providerTypeLabel(value.provider_type) : null}
				onSelect={(item: AutocompleteItem) => {
					value.provider_type = item.id;
				}}
				onClear={() => {
					value.provider_type = '';
				}}
			/>
		</div>
		<div class="space-y-2">
			<p class="text-sm font-medium" style="color: var(--color-muted)">
				{t('travel.api_config', 'API Config')}
			</p>
			<div>
				<label for="travel-api-endpoint" class="mb-1 block text-xs" style="color: var(--color-muted)">
					{t('travel.api_endpoint', 'Endpoint URL')}
				</label>
				<input
					id="travel-api-endpoint"
					type="url"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={value.api_endpoint}
					placeholder="https://api.example.com/search"
				/>
			</div>
			<div>
				<label for="travel-api-key" class="mb-1 block text-xs" style="color: var(--color-muted)">
					{t('travel.api_key', 'API Key')}
				</label>
				<input
					id="travel-api-key"
					type="text"
					class="w-full rounded-md border px-3 py-2 text-sm font-mono"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={value.api_key}
					placeholder={t('travel.api_key_placeholder', 'Sent as a Bearer token')}
				/>
			</div>
			<div>
				<p class="mb-1 text-xs" style="color: var(--color-muted)">
					{t('travel.api_extra', 'Extra settings')}
				</p>
				{@render PairEditor({
					pairs: (value.api_extra_pairs ?? []) as ConfigPair[],
					add: () =>
						((value.api_extra_pairs ??= []) as ConfigPair[]).push({ key: '', value: '' }),
					remove: (i: number) =>
						(value.api_extra_pairs as ConfigPair[] | undefined)?.splice(i, 1),
					addLabel: t('travel.add_setting', 'Add setting')
				})}
			</div>
			{#if editing}
				<p class="text-xs" style="color: var(--color-muted)">
					{t('travel.api_config_hint', 'Leave blank to keep existing credentials. Any value entered here replaces them.')}
				</p>
			{/if}
		</div>
		<div>
			<p class="mb-1 text-sm font-medium" style="color: var(--color-muted)">
				{t('travel.affiliate_config', 'Affiliate Params')}
			</p>
			{@render PairEditor({
				pairs: (value.affiliate_pairs ?? []) as ConfigPair[],
				add: () =>
					((value.affiliate_pairs ??= []) as ConfigPair[]).push({ key: '', value: '' }),
				remove: (i: number) =>
					(value.affiliate_pairs as ConfigPair[] | undefined)?.splice(i, 1),
				addLabel: t('travel.add_param', 'Add parameter')
			})}
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('travel.affiliate_hint', 'Appended to booking links as URL parameters (e.g. aff_id, aff_sub).')}
			</p>
		</div>
		<div class="flex items-center gap-2">
			<!-- New providers default to active (the create handler treats an
			     unset is_active as true) — mirror that default in the checkbox
			     so the visible state matches what will be submitted. -->
			<input
				type="checkbox"
				id="travel-active"
				checked={value.is_active ?? !editing}
				onchange={(e) => (value.is_active = e.currentTarget.checked)}
			/>
			<label for="travel-active" class="text-sm" style="color: var(--color-foreground)">
				{t('travel.active', 'Active')}
			</label>
		</div>
	</div>
{/snippet}

<!-- Key/value row editor for flat config maps (extra settings, affiliate
     params). Mutations go through the add/remove callbacks so state writes
     stay inside event handlers (mutating in {@const}/render is illegal). -->
{#snippet PairEditor({ pairs, add, remove, addLabel }: {
	pairs: ConfigPair[];
	add: () => void;
	remove: (i: number) => void;
	addLabel: string;
})}
	<div class="space-y-2">
		{#each pairs as pair, i (i)}
			<div class="flex items-center gap-2">
				<input
					type="text"
					class="w-2/5 rounded-md border px-3 py-1.5 text-sm font-mono"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={pair.key}
					placeholder={t('travel.param_name', 'Parameter')}
					aria-label={t('travel.param_name', 'Parameter')}
				/>
				<input
					type="text"
					class="flex-1 rounded-md border px-3 py-1.5 text-sm font-mono"
					style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
					bind:value={pair.value}
					placeholder={t('travel.param_value', 'Value')}
					aria-label={t('travel.param_value', 'Value')}
				/>
				<button
					type="button"
					class="rounded-md px-2 py-1 text-sm"
					style="color: var(--color-muted)"
					onclick={() => remove(i)}
					aria-label={t('common.remove', 'Remove')}
				>
					✕
				</button>
			</div>
		{/each}
		<button
			type="button"
			class="rounded-md border px-3 py-1.5 text-xs font-medium"
			style="border-color: var(--color-border); color: var(--color-primary)"
			onclick={add}
		>
			+ {addLabel}
		</button>
	</div>
{/snippet}

<!-- View slide-over body -->
{#snippet ProviderView({ row }: { row: TravelProvider & { affiliate_pairs: ConfigPair[] } })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('travel.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('travel.slug', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('travel.type', 'Type')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{providerTypeLabel(row.provider_type)}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('travel.active', 'Active')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_active ? t('travel.active_label', 'Active') : t('travel.inactive_label', 'Inactive')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('travel.affiliate_config', 'Affiliate Params')}</p>
			{#if row.affiliate_pairs?.length}
				<div class="mt-1 space-y-1">
					{#each row.affiliate_pairs as pair (pair.key)}
						<div class="flex items-center gap-2 text-sm">
							<span class="font-mono text-xs" style="color: var(--color-muted)">{pair.key}</span>
							<span style="color: var(--color-foreground)">{pair.value}</span>
						</div>
					{/each}
				</div>
			{:else}
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">—</p>
			{/if}
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('travel.created', 'Created')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
