<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /plans — migrated to the EntityPage scaffold.
	//
	// The plan form includes a feature-list editor (add/remove/toggle) and a
	// currency <Select> whose options are loaded dynamically from the wallet API,
	// neither of which the declarative EntityForm can render, so the form body is
	// supplied via the `formSnippet` escape hatch (the `fields` array is still
	// passed so EntityPage can seed the edit-form value). The "Manage Tracks"
	// sub-flow is kept as a page-local <Modal> triggered from a custom row action.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { plansApi, walletApi, type SubscriptionPlan, type PlanFeature, api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Eye, Pencil, Trash2, Plus, X, Check, Star, ListChecks } from 'lucide-svelte';

	type Plan = SubscriptionPlan;

	const canManage = $derived(hasPermission('payments.plans.manage'));

	// Currencies are fetched once on mount so the form <Select> and the table
	// price renders can look up symbols (Rule 16 — no hardcoded data).
	let currencies = $state<{ code: string; symbol: string; name?: string }[]>([]);
	const currencyOptions = $derived(
		currencies.map((c) => ({ value: c.code, label: c.name ? `${c.name} (${c.code})` : c.code }))
	);
	const currencySymbolMap = $derived(new Map(currencies.map((c) => [c.code, c.symbol])));
	const currencyNameMap = $derived(new Map(currencies.map((c) => [c.code, c.name ?? ''])));

	const statuses = [
		{ value: 'active', label: t('plans.status_active', 'Active') },
		{ value: 'inactive', label: t('plans.status_inactive', 'Inactive') }
	];

	$effect(() => {
		void (async () => {
			const currResult = await walletApi.getCurrencies();
			if (currResult.ok && currResult.data) {
				currencies = currResult.data;
			}
		})();
	});

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	// The backend serializes JSONB features as a raw JSON string ([]byte), so
	// parse it into an array here for the UI (mirrors the legacy load()).
	function mapPlan(raw: any): Plan {
		return {
			...raw,
			features: typeof raw.features === 'string' ? JSON.parse(raw.features as unknown as string) : (raw.features ?? [])
		};
	}

	// ── Manage Tracks sub-flow (page-local Modal) ───────────────────────────
	let showTracksModal = $state(false);
	let tracksPlanId = $state<string | null>(null);
	let tracksPlanName = $state('');
	let allTracks = $state<{ id: string; title: string; slug: string }[]>([]);
	let selectedTrackIds = $state<Set<string>>(new Set());
	let loadingTracks = $state(false);
	let savingTracks = $state(false);

	async function openManageTracks(plan: Plan) {
		tracksPlanId = plan.id;
		tracksPlanName = plan.name;
		showTracksModal = true;
		loadingTracks = true;
		selectedTrackIds = new Set();

		// Fetch all learning tracks and the plan's current track list in parallel.
		const [tracksRes, planTracksRes] = await Promise.all([
			api<{ id: string; title: string; slug: string }[]>('/admin/academy/tracks'),
			plansApi.listTracks(plan.id)
		]);
		if (tracksRes.ok && tracksRes.data) {
			allTracks = tracksRes.data;
		}
		if (planTracksRes.ok && planTracksRes.data) {
			selectedTrackIds = new Set(planTracksRes.data.map((tr) => tr.track_id));
		}
		loadingTracks = false;
	}

	function toggleTrackSelection(trackId: string) {
		const next = new Set(selectedTrackIds);
		if (next.has(trackId)) {
			next.delete(trackId);
		} else {
			next.add(trackId);
		}
		selectedTrackIds = next;
	}

	async function saveTracks() {
		if (!tracksPlanId || savingTracks) return;
		savingTracks = true;
		const result = await plansApi.setTracks(tracksPlanId, Array.from(selectedTrackIds));
		savingTracks = false;
		if (result.ok) {
			showTracksModal = false;
		}
	}

	// ── Bulk delete ───── EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => plansApi.delete(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('plans.delete_failed', 'Failed to delete selected plans');
		} finally {
			bulkDeleting = false;
		}
	}

	// `fields` is used by EntityPage to seed the form value on edit; the actual
	// inputs are rendered by the `formSnippet` below (the rich plan form).
	const tierOptions = [
		{ value: 'regular', label: t('plans.tier.regular', 'Regular') },
		{ value: 'pro', label: t('plans.tier.pro', 'Pro') },
		{ value: 'business', label: t('plans.tier.business', 'Business') }
	];

	const fields: FieldDef[] = [
		{ key: 'name', label: t('plans.name', 'Name'), type: 'text' },
		{ key: 'slug', label: t('plans.slug', 'Slug'), type: 'text' },
		{ key: 'description', label: t('plans.description', 'Description'), type: 'textarea' },
		{ key: 'features', label: t('plans.features', 'Features'), type: 'text' },
		{ key: 'monthly_amount', label: t('plans.monthly', 'Monthly'), type: 'number' },
		{ key: 'yearly_amount', label: t('plans.yearly', 'Yearly'), type: 'number' },
		{ key: 'currency', label: t('plans.currency', 'Currency'), type: 'text' },
		{ key: 'status', label: t('plans.status', 'Status'), type: 'text' },
		{ key: 'sort_order', label: t('plans.sort_order', 'Sort Order'), type: 'number' },
		{ key: 'highlighted', label: t('plans.highlighted', 'Highlighted'), type: 'checkbox' },
		{ key: 'tier', label: t('plans.tier', 'Tier'), type: 'select', options: tierOptions }
	];

	// Row actions: View (always) + Manage Tracks / Edit / Delete (permission-gated).
	// Providing `actions` replaces the default set, so View/Edit/Delete are included explicitly.
	const config: EntityPageConfig = {
		title: t('nav.plans', 'Subscription Plans'),
		description: t('plans.description_long', 'Create, edit, and manage subscription plan types, pricing, and feature lists.'),
		createLabel: t('plans.add', 'Add Plan'),
		createPermission: 'payments.plans.manage',
		pageSize: 25,
		columns: [
			{
				key: 'sort_order',
				label: '#',
				class: 'w-10 text-center',
				render: (v: number) => `<span class="text-xs text-muted">${v}</span>`
			},
			{
				key: 'name',
				label: t('plans.name', 'Name'),
				render: (v: string, row: Plan) => {
					const star = row.highlighted ? '<span class="ms-1 inline-block text-warning">★</span>' : '';
					return `<span class="font-medium">${v}${star}</span>`;
				}
			},
			{ key: 'slug', label: t('plans.slug', 'Slug'), class: 'font-mono text-xs' },
			{
				key: 'monthly_amount',
				label: t('plans.monthly', 'Monthly'),
				render: (v: number, row: Plan) => {
					if (v === 0) return `<span class="text-success text-xs">${t('common.free', 'Free')}</span>`;
					const sym = currencySymbolMap.get(row.currency) ?? row.currency;
					return `<span class="text-xs">${sym}${(v / 100).toLocaleString()}</span>`;
				}
			},
			{
				key: 'yearly_amount',
				label: t('plans.yearly', 'Yearly'),
				render: (v: number, row: Plan) => {
					if (v === 0) return '<span class="text-muted text-xs">—</span>';
					const sym = currencySymbolMap.get(row.currency) ?? row.currency;
					return `<span class="text-xs">${sym}${(v / 100).toLocaleString()}</span>`;
				}
			},
			{
				key: 'features',
				label: t('plans.features', 'Features'),
				sortable: false,
				render: (v: PlanFeature[]) => {
					const count = v?.filter((f) => f.included).length ?? 0;
					return `<span class="text-xs text-muted">${count} included</span>`;
				}
			},
			{
				key: 'status',
				label: t('plans.status', 'Status'),
				render: (v: string) =>
					v === 'active'
						? `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-success/15 text-success">${t('common.active', 'Active')}</span>`
						: `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-muted/15 text-muted">${t('common.inactive', 'Inactive')}</span>`
			},
			{
				key: 'tier',
				label: t('plans.tier', 'Tier'),
				render: (v: string) =>
					v
						? `<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-info/15 text-info">${v}</span>`
						: '<span class="inline-block rounded-full px-2 py-0.5 text-xs bg-muted/15 text-muted">—</span>'
			}
		],
		actions: [
			{ label: t('common.view', 'View'), icon: Eye, onclick: (_row, ctx) => ctx?.openView(_row) },
			{
				label: t('plans.manage_tracks', 'Manage Tracks'),
				icon: ListChecks,
				onclick: (row) => openManageTracks(row),
				show: () => canManage
			},
			{
				label: t('common.edit', 'Edit'),
				icon: Pencil,
				onclick: (_row, ctx) => ctx?.openEdit(_row),
				show: () => canManage
			},
			{
				label: t('common.delete', 'Delete'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (_row, ctx) => ctx?.openDelete(_row),
				show: () => canManage
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
			const result = await api<Plan[]>('/admin/plans?page_size=1000');
			if (result.ok) {
				let data = ((result.data ?? []) as Plan[]).map(mapPlan);
				const query = (q ?? '').toLowerCase();
				if (query) {
					data = data.filter((row) =>
						[row.name, row.slug, row.description, row.status].some((v) =>
							String(v ?? '').toLowerCase().includes(query)
						)
					);
				}
				filter;
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = data.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				return {
					data: data.slice(offset, offset + size),
					total,
					totalPages
				};
			}
			throw new Error(result.error ?? t('plans.load_failed', 'Failed to load plans'));
		},
		create: async (body) => {
			const res = await plansApi.create({
				slug: String(body.slug ?? '').trim() || slugify(String(body.name ?? '')),
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim(),
				features: Array.isArray(body.features) ? body.features : [],
				monthly_amount: Number(body.monthly_amount ?? 0),
				yearly_amount: Number(body.yearly_amount ?? 0),
				currency: String(body.currency ?? 'NGN'),
				highlighted: !!body.highlighted,
				sort_order: Number(body.sort_order ?? 0),
				status: String(body.status ?? 'active'),
			tier: (body.tier as 'regular' | 'pro' | 'business') || 'regular'
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await plansApi.update(id, {
				slug: String(body.slug ?? '').trim() || slugify(String(body.name ?? '')),
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim(),
				features: Array.isArray(body.features) ? body.features : [],
				monthly_amount: Number(body.monthly_amount ?? 0),
				yearly_amount: Number(body.yearly_amount ?? 0),
				currency: String(body.currency ?? 'NGN'),
				highlighted: !!body.highlighted,
				sort_order: Number(body.sort_order ?? 0),
				status: String(body.status ?? 'active'),
			tier: (body.tier as 'regular' | 'pro' | 'business') || 'regular'
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await plansApi.delete(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: PlanView,
		formSnippet: PlanForm
	};
</script>

<svelte:head>
	<title>{t('nav.plans', 'Subscription Plans')} — {siteName()} Admin</title>
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

<!-- Manage Tracks Modal (page-local sub-flow) -->
<Modal bind:open={showTracksModal} title={t('plans.manage_tracks_title', 'Manage Tracks — {name}').replace('{name}', tracksPlanName)} size="lg">
	<div class="space-y-3">
		<p class="text-xs" style="color: var(--color-muted)">
			{t('plans.tracks_hint', 'Select which courses are included in this subscription plan. When no tracks are selected, the plan includes ALL tracks (Pro = all).')}
		</p>
		{#if loadingTracks}
			<p class="text-sm" style="color: var(--color-muted)">{t('plans.loading_tracks', 'Loading tracks...')}</p>
		{:else}
			<div class="max-h-80 space-y-1 overflow-y-auto">
				{#each allTracks as track (track.id)}
					<label
						class="flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm hover:bg-surface-hover"
						style="border-color: var(--color-border)"
					>
						<input
							type="checkbox"
							checked={selectedTrackIds.has(track.id)}
							onchange={() => toggleTrackSelection(track.id)}
							class="h-4 w-4 rounded"
						/>
						<span class="flex-1">{track.title}</span>
						<span class="font-mono text-xs" style="color: var(--color-muted)">{track.slug}</span>
					</label>
				{/each}
				{#if allTracks.length === 0}
					<p class="text-sm" style="color: var(--color-muted)">{t('plans.no_tracks', 'No tracks available.')}</p>
				{/if}
			</div>
			<p class="text-xs" style="color: var(--color-muted)">
				{selectedTrackIds.size === 0
					? t('plans.all_tracks_included', 'All tracks included (no filter)')
					: t('plans.tracks_selected', '{count} track(s) selected').replace('{count}', String(selectedTrackIds.size))}
			</p>
		{/if}
	</div>

	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => (showTracksModal = false)}>
			{t('common.cancel', 'Cancel')}
		</Button>
		<Button onclick={saveTracks} disabled={savingTracks}>
			{savingTracks ? t('plans.saving', 'Saving...') : t('plans.save_tracks', 'Save Tracks')}
		</Button>
	</div>
</Modal>

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

<!-- View modal body (mirrors the legacy view) -->
{#snippet PlanView({ row }: { row: Plan })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.name}{#if row.highlighted}<span class="ms-1 inline-block text-warning">★</span>{/if}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.slug', 'Slug')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.slug}</p>
		</div>
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.description', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.tier', 'Tier')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{#if row.tier}
						<span class="inline-block rounded-full bg-info/15 px-2 py-0.5 text-xs text-info">{t('plans.tier.' + row.tier, row.tier)}</span>
					{:else}
						—
					{/if}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.monthly', 'Monthly')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.monthly_amount === 0
						? t('plans.free', 'Free')
						: `${currencySymbolMap.get(row.currency) ?? row.currency}${(row.monthly_amount / 100).toLocaleString()}`}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.yearly', 'Yearly')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.yearly_amount === 0
						? '—'
						: `${currencySymbolMap.get(row.currency) ?? row.currency}${(row.yearly_amount / 100).toLocaleString()}`}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.currency', 'Currency')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{currencyNameMap.get(row.currency) ? `${row.currency} — ${currencyNameMap.get(row.currency)}` : row.currency}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.sort_order', 'Sort Order')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.sort_order}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.features', 'Features')}</p>
			{#if row.features && row.features.length > 0}
				<ul class="mt-1 space-y-1">
					{#each row.features as feature (feature.text)}
						<li class="flex items-center gap-2 text-sm" style="color: var(--color-foreground)">
							{#if feature.included}
								<Check class="h-3.5 w-3.5 text-success" />
							{:else}
								<X class="h-3.5 w-3.5 text-muted" />
							{/if}
							<span>{feature.text}</span>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="mt-1 text-sm" style="color: var(--color-muted)">—</p>
			{/if}
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('plans.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.status === 'active' ? t('plans.status_active', 'Active') : t('plans.status_inactive', 'Inactive')}
			</p>
		</div>
	</div>
{/snippet}

<!-- Create/Edit form body (rich form: feature-list editor + dynamic currency select) -->
{#snippet PlanForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<div class="space-y-3">
		<div>
			<label for="plan-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.name', 'Name')}</label>
			<input
				id="plan-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('plans.ph_pro', 'Pro')}
				bind:value={value.name}
				oninput={() => {
					if (!editing) value.slug = slugify(String(value.name ?? ''));
				}}
			/>
		</div>
		<div>
			<label for="plan-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.slug', 'Slug')}</label>
			<input
				id="plan-slug"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm font-mono"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="pro"
				bind:value={value.slug}
			/>
		</div>
		<div>
			<label for="plan-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.description', 'Description')}</label>
			<textarea
				id="plan-desc"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				rows="2"
				bind:value={value.description}
			></textarea>
		</div>

		<!-- Feature list editor -->
		<div>
			<span class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.features', 'Features')}</span>
			<div class="space-y-1.5">
				{#each value.features as feature, idx (idx)}
					<div class="flex items-center gap-2">
						<button
							type="button"
							class="shrink-0 rounded p-1"
							onclick={() => {
								feature.included = !feature.included;
							}}
							title={feature.included ? t('plans.included', 'Included') : t('plans.not_included', 'Not included')}
						>
							{#if feature.included}
								<Check class="h-4 w-4 text-success" />
							{:else}
								<X class="h-4 w-4 text-muted" />
							{/if}
						</button>
						<input
							type="text"
							class="flex-1 rounded-md border px-2 py-1 text-xs"
							style="border-color: var(--color-border); background-color: var(--color-surface)"
							bind:value={value.features[idx].text}
						/>
						<button
							type="button"
							class="shrink-0 rounded p-1"
							style="color: var(--color-danger)"
							onclick={() => {
								value.features = value.features.filter((_: any, i: number) => i !== idx);
							}}
						>
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					</div>
				{/each}
			</div>
			<div class="mt-2 flex items-center gap-2">
				<input
					type="text"
					class="flex-1 rounded-md border px-2 py-1.5 text-xs"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					placeholder={t('plans.add_feature', 'Add a feature…')}
					bind:value={value._newFeatureText}
					onkeydown={(e) => {
						if (e.key === 'Enter') {
							e.preventDefault();
							const text = String(value._newFeatureText ?? '').trim();
							if (text) {
								value.features = [...(value.features ?? []), { text, included: true }];
								value._newFeatureText = '';
							}
						}
					}}
				/>
				<button
					type="button"
					class="shrink-0 rounded-md border px-2 py-1.5 text-xs"
					style="border-color: var(--color-border)"
					onclick={() => {
						const text = String(value._newFeatureText ?? '').trim();
						if (text) {
							value.features = [...(value.features ?? []), { text, included: true }];
							value._newFeatureText = '';
						}
					}}
				>
					<Plus class="h-3.5 w-3.5" />
				</button>
			</div>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="plan-monthly" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.monthly_kobo', 'Monthly (in kobo/cents)')}</label>
				<input
					id="plan-monthly"
					type="number"
					min="0"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					bind:value={value.monthly_amount}
				/>
			</div>
			<div>
				<label for="plan-yearly" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.yearly_kobo', 'Yearly (in kobo/cents)')}</label>
				<input
					id="plan-yearly"
					type="number"
					min="0"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					bind:value={value.yearly_amount}
				/>
			</div>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="plan-currency" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.currency', 'Currency')}</label>
				<Select options={currencyOptions} bind:value={value.currency} />
			</div>
			<div>
				<label for="plan-status" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.status', 'Status')}</label>
				<Select options={statuses} bind:value={value.status} />
			</div>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="plan-tier" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.tier', 'Tier')}</label>
				<Select options={tierOptions} bind:value={value.tier} />
			</div>
			<div class="flex items-end gap-2 pb-2">
				<input id="plan-highlight" type="checkbox" bind:checked={value.highlighted} />
				<label for="plan-highlight" class="flex items-center gap-1 text-sm">
					<Star class="h-3.5 w-3.5" /> {t('plans.highlighted', 'Highlighted')}
				</label>
			</div>
		</div>

		<div class="grid grid-cols-2 gap-3">
			<div>
				<label for="plan-sort" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('plans.sort_order', 'Sort Order')}</label>
				<input
					id="plan-sort"
					type="number"
					min="0"
					class="w-full rounded-md border px-3 py-2 text-sm"
					style="border-color: var(--color-border); background-color: var(--color-surface)"
					bind:value={value.sort_order}
				/>
			</div>
		</div>
	</div>
{/snippet}
