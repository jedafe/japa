<script lang="ts">
	// /wallet/credit-packs — migrated to the EntityPage scaffold; declarative CRUD entity.
	// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { walletApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Trash2 } from 'lucide-svelte';

	type CreditPack = {
		id: string;
		name: string;
		description: string | null;
		credit_amount: number;
		bonus_credits: number;
		min_user_level: string | null;
		is_active: boolean;
		position: number;
	};

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
			await Promise.all(idArr.map((id) => walletApi.deleteCreditPack(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('app.wallet.error_delete_packs', 'Failed to delete selected packs');
		} finally {
			bulkDeleting = false;
		}
	}

	const fields: FieldDef[] = [
		{ key: 'name', label: t('app.wallet.pack_name', 'Pack Name'), type: 'text', help: t('app.wallet.name_hint', 'The name shown in admin lists and on the site where public.'), placeholder: 'e.g. Starter Pack', required: true },
		{ key: 'description', label: t('app.wallet.pack_description', 'Description'), type: 'textarea', help: t('app.wallet.description_hint', 'Shown in detail views — keep it concise.'), placeholder: 'Optional description shown to users' },
		{ key: 'credit_amount', label: t('app.wallet.credit_amount', 'Credit Amount'), type: 'number', help: t('app.wallet.credit_amount_hint', 'Amount in wallet credits.'), min: 1, required: true },
		{ key: 'bonus_credits', label: t('app.wallet.bonus_credits', 'Bonus Credits'), type: 'number', help: t('app.wallet.bonus_credits_hint', 'Credits granted as a bonus on this action.'), min: 0 },
		{ key: 'min_user_level', label: t('app.wallet.min_user_level', 'Min User Level'), type: 'text', help: t('app.wallet.min_user_level_hint', 'Minimum account level required.'), placeholder: 'e.g. pro (optional)' },
		{ key: 'position', label: t('app.wallet.position', 'Position'), type: 'number', help: t('app.wallet.position_hint', 'Ordering position — lower numbers show first.'), min: 0 }
	];

	const config: EntityPageConfig = {
		title: t('app.wallet.credit_packs_title', 'Credit Pack Management'),
		description: t('app.wallet.credit_packs_desc', 'Create and manage credit bundles with optional bonus credits'),
		createLabel: t('app.wallet.add_pack', 'Add Pack'),
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('app.wallet.pack_name', 'Pack Name'), sortable: true, render: (v: string) => `<span class="font-medium">${v}</span>` },
			{ key: 'credit_amount', label: t('app.wallet.credit_amount', 'Credit Amount'), sortable: true, render: (v: number) => (v ?? 0).toLocaleString() },
			{
				key: 'bonus_credits',
				label: t('app.wallet.bonus_credits', 'Bonus Credits'),
				render: (v: number) => (v > 0 ? `<span class="text-success font-semibold">+${v.toLocaleString()}</span>` : '<span class="text-muted">—</span>')
			},
			{
				key: 'total',
				label: t('app.wallet.total_credits', 'Total Credits'),
				render: (_: unknown, row: CreditPack) => (row.credit_amount + row.bonus_credits).toLocaleString()
			},
			{ key: 'min_user_level', label: t('app.wallet.min_user_level', 'Min User Level'), render: (v: string | null) => v ?? '<span class="text-muted">—</span>' },
			{ key: 'position', label: t('app.wallet.position', 'Position'), sortable: true, render: (v: number) => String(v) },
			{
				key: 'is_active',
				label: t('common.status', 'Status'),
				render: (_: unknown, row: CreditPack) => {
					const label = row.is_active ? t('common.active', 'Active') : t('common.inactive', 'Inactive');
					const cls = row.is_active ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border';
					return `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${cls}">${label}</span>`;
				}
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
			}
		],
		bulkActions: [
			{ label: t('common.deleteSelected', 'Delete Selected'), icon: Trash2, variant: 'destructive', onclick: bulkDelete }
		],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const result = await walletApi.listCreditPacks();
			if (result.ok) {
				let data = (result.data as CreditPack[] | { data: CreditPack[] }) as any;
				if (!Array.isArray(data)) data = (data as any)?.data ?? [];
				if (filter.is_active) data = data.filter((p: CreditPack) => String(p.is_active) === filter.is_active);
				if (q) {
					const lq = q.toLowerCase();
					data = data.filter((p: CreditPack) => p.name?.toLowerCase().includes(lq) || (p.description ?? '').toLowerCase().includes(lq));
				}
				const total = data.length;
				const totalPages = pageSize > 0 ? Math.max(1, Math.ceil(total / pageSize)) : 1;
				const start = page * pageSize;
				return { data: data.slice(start, start + pageSize), total, totalPages };
			}
			throw new Error(result.error ?? t('app.wallet.error_load_packs', 'Failed to load credit packs'));
		},
		create: async (body) => {
			const payload: Record<string, unknown> = {
				name: String(body.name ?? '').trim(),
				credit_amount: Number(body.credit_amount ?? 0),
				bonus_credits: Number(body.bonus_credits ?? 0),
				position: Number(body.position ?? 0)
			};
			if (body.description) payload.description = String(body.description).trim();
			if (body.min_user_level) payload.min_user_level = String(body.min_user_level).trim();
			const res = await walletApi.createCreditPack(payload as any);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const payload: Record<string, unknown> = {
				name: String(body.name ?? '').trim(),
				credit_amount: Number(body.credit_amount ?? 0),
				bonus_credits: Number(body.bonus_credits ?? 0),
				position: Number(body.position ?? 0)
			};
			if (body.description) payload.description = String(body.description).trim();
			if (body.min_user_level) payload.min_user_level = String(body.min_user_level).trim();
			const res = await walletApi.updateCreditPack(id, payload as any);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await walletApi.deleteCreditPack(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: PackView
	};
</script>

<svelte:head>
	<title>{t('app.wallet.credit_packs_title', 'Credit Pack Management')} — Admin</title>
</svelte:head>

{#if pageError}
	<div class="mb-4 rounded-lg p-3 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

{#snippet PackView({ row }: { row: CreditPack })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.pack_name', 'Pack Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.pack_description', 'Description')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description ?? '—'}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.credit_amount', 'Credit Amount')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.credit_amount.toLocaleString()}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.bonus_credits', 'Bonus Credits')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.bonus_credits > 0 ? `+${row.bonus_credits.toLocaleString()}` : '—'}</p>
			</div>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.min_user_level', 'Min User Level')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.min_user_level ?? '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('app.wallet.position', 'Position')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.position}</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.is_active ? t('common.active', 'Active') : t('common.inactive', 'Inactive')}</p>
		</div>
	</div>
{/snippet}
