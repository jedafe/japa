<script lang="ts">
	// /travel — Travel Search providers (Section 67, ADR-085).
	// Migrated to the EntityPage scaffold. Uses `formSnippet` for the JSON
	// config textareas (api_config + affiliate_config) because EntityForm's
	// textarea does not round-trip JSON objects — the snippet stringifies on
	// edit-seed and the create/update handlers JSON.parse before sending.
	//
	// Ref: japaconnect-engineering/sections/67-travel-search.md
	//      japaconnect-engineering/adr/ADR-085-travel-search.md
	import { EntityPage, type EntityPageConfig, type FieldDef } from '$lib/components/entity';
	import { travelAdminApi, type TravelProvider } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	// Normalize an API row for the scaffold. affiliate_config arrives as a
	// JSON object (or null); stringify it so the textarea shows valid JSON.
	// api_config is never returned by the backend (write-only) — leave blank.
	function mapProvider(raw: TravelProvider): TravelProvider & {
		affiliate_config_text: string;
		api_config_text: string;
	} {
		return {
			...raw,
			affiliate_config_text: raw.affiliate_config
				? JSON.stringify(raw.affiliate_config, null, 2)
				: '{}',
			api_config_text: ''
		};
	}

	// Parse a JSON config textarea. Returns { ok, value?, error? }.
	function parseJSONConfig(
		text: string,
		fieldLabel: string
	): { ok: true; value: unknown } | { ok: false; error: string } {
		const trimmed = (text ?? '').trim();
		if (trimmed === '') return { ok: true, value: undefined };
		try {
			return { ok: true, value: JSON.parse(trimmed) };
		} catch {
			return {
				ok: false,
				error: t('travel.config_invalid', '{field} is not valid JSON.').replace(
					'{field}',
					fieldLabel
				)
			};
		}
	}

	// `fields` drives EntityForm rendering + edit-form value seeding. The
	// JSON config fields use `textarea`; the snippet below overrides their
	// rendering but the fields array is still required for seeding.
	const fields: FieldDef[] = [
		{ key: 'name', label: t('travel.name', 'Name'), type: 'text', required: true, placeholder: 'Skyscanner' },
		{ key: 'slug', label: t('travel.slug', 'Slug'), type: 'text', required: true, placeholder: 'skyscanner', readonlyInEdit: true },
		{
			key: 'provider_type',
			label: t('travel.type', 'Provider Type'),
			type: 'select',
			required: true,
			options: [
				{ value: 'flight', label: t('travel.type_flight', 'Flights') },
				{ value: 'hotel', label: t('travel.type_hotel', 'Hotels') },
				{ value: 'both', label: t('travel.type_both', 'Flights & Hotels') }
			]
		},
		{ key: 'api_config_text', label: t('travel.api_config', 'API Config (JSON)'), type: 'textarea' },
		{ key: 'affiliate_config_text', label: t('travel.affiliate_config', 'Affiliate Config (JSON)'), type: 'textarea' },
		{ key: 'is_active', label: t('travel.active', 'Active'), type: 'checkbox' }
	];

	const config: EntityPageConfig = {
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
			{ key: 'provider_type', label: t('travel.type', 'Type'), sortable: true },
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
				options: [
					{ value: 'flight', label: t('travel.type_flight', 'Flights') },
					{ value: 'hotel', label: t('travel.type_hotel', 'Hotels') },
					{ value: 'both', label: t('travel.type_both', 'Flights & Hotels') }
				]
			}
		],
		fields,
		load: async ({ q, filter }) => {
			const params = new URLSearchParams();
			if (filter.is_active) params.set('active_only', filter.is_active === 'true' ? 'true' : 'false');
			if (filter.provider_type) params.set('provider_type', filter.provider_type);
			if (q) params.set('q', q);
			const result = await travelAdminApi.list(params.toString() || undefined);
			if (result.ok) {
				const data = (result.data as TravelProvider[]) ?? [];
				const mapped = data.map(mapProvider);
				return { data: mapped, total: mapped.length, totalPages: 1 };
			}
			throw new Error(result.error ?? t('travel.load_error', 'Failed to load travel providers'));
		},
		create: async (body) => {
			const apiCfg = parseJSONConfig(String(body.api_config_text ?? ''), t('travel.api_config', 'API Config'));
			if (!apiCfg.ok) return { ok: false, error: apiCfg.error };
			const affCfg = parseJSONConfig(
				String(body.affiliate_config_text ?? ''),
				t('travel.affiliate_config', 'Affiliate Config')
			);
			if (!affCfg.ok) return { ok: false, error: affCfg.error };
			const res = await travelAdminApi.create({
				name: String(body.name ?? '').trim(),
				slug: String(body.slug ?? '').trim(),
				provider_type: String(body.provider_type ?? 'flight'),
				api_config: apiCfg.value,
				affiliate_config: affCfg.value,
				is_active: body.is_active === undefined ? true : !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (_id, body) => {
			// api_config is write-only and never returned by the backend, so
			// an empty textarea on edit means "leave unchanged". A non-empty
			// value replaces the stored config.
			const apiText = String(body.api_config_text ?? '').trim();
			const apiCfg = apiText ? parseJSONConfig(apiText, t('travel.api_config', 'API Config')) : { ok: true as const, value: undefined };
			if (!apiCfg.ok) return { ok: false, error: apiCfg.error };
			const affCfg = parseJSONConfig(
				String(body.affiliate_config_text ?? ''),
				t('travel.affiliate_config', 'Affiliate Config')
			);
			if (!affCfg.ok) return { ok: false, error: affCfg.error };
			const res = await travelAdminApi.update(_id, {
				name: String(body.name ?? '').trim(),
				provider_type: String(body.provider_type ?? 'flight'),
				api_config: apiCfg.value,
				affiliate_config: affCfg.value,
				is_active: body.is_active === undefined ? true : !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: ProviderView,
		formSnippet: ConfigForm
	};
</script>

<svelte:head>
	<title>{t('travel.title', 'Travel Search')} — JapaConnect Admin</title>
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

<!-- Config form — overrides the api_config_text + affiliate_config_text
     textareas with monospace-styled JSON editors. Binds to value[fieldKey]. -->
{#snippet ConfigForm({ value, editing }: { value: Record<string, any>; editing: boolean })}
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
				readonly={editing}
				disabled={editing}
			/>
		</div>
		<div>
			<label for="travel-type" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">
				{t('travel.type', 'Provider Type')}
			</label>
			<select
				id="travel-type"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.provider_type}
			>
				<option value="flight">{t('travel.type_flight', 'Flights')}</option>
				<option value="hotel">{t('travel.type_hotel', 'Hotels')}</option>
				<option value="both">{t('travel.type_both', 'Flights & Hotels')}</option>
			</select>
		</div>
		<div>
			<label for="travel-api-config" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">
				{t('travel.api_config', 'API Config (JSON)')}
			</label>
			<textarea
				id="travel-api-config"
				class="w-full rounded-md border px-3 py-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="4"
				bind:value={value.api_config_text}
				placeholder={'{"endpoint":"https://api.example.com/search","api_key":"..."}'}
			></textarea>
			{#if editing}
				<p class="mt-1 text-xs" style="color: var(--color-muted)">
					{t('travel.api_config_hint', 'Leave blank to keep existing credentials. Enter new JSON to replace.')}
				</p>
			{/if}
		</div>
		<div>
			<label for="travel-affiliate-config" class="mb-1 block text-sm font-medium" style="color: var(--color-muted)">
				{t('travel.affiliate_config', 'Affiliate Config (JSON)')}
			</label>
			<textarea
				id="travel-affiliate-config"
				class="w-full rounded-md border px-3 py-2 text-sm font-mono"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				rows="3"
				bind:value={value.affiliate_config_text}
				placeholder={'{"aff_id":"your-affiliate-id","aff_sub":"japaconnect"}'}
			></textarea>
		</div>
		<div class="flex items-center gap-2">
			<input type="checkbox" id="travel-active" bind:checked={value.is_active} />
			<label for="travel-active" class="text-sm" style="color: var(--color-foreground)">
				{t('travel.active', 'Active')}
			</label>
		</div>
	</div>
{/snippet}

<!-- View slide-over body -->
{#snippet ProviderView({ row }: { row: TravelProvider & { affiliate_config_text: string } })}
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
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.provider_type}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('travel.active', 'Active')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_active ? t('travel.active_label', 'Active') : t('travel.inactive_label', 'Inactive')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('travel.affiliate_config', 'Affiliate Config')}</p>
			<pre class="mt-1 break-all rounded-md border p-2 text-xs font-mono" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{row.affiliate_config_text}</pre>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('travel.created', 'Created')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
