<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /federation/instances — §126 instance domain blocks (M2). EntityPage scaffold; create/delete only.
	import { EntityPage, type EntityPageConfig, type FieldDef } from '#lib/components/entity/index.js';
	import { federationAdminApi } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { Eye, Trash2 } from 'lucide-svelte';

	type InstanceBlock = {
		id: string;
		domain: string;
		reason?: string | null;
		created_at: string;
	};

	const fields: FieldDef[] = [
		{
			key: 'domain',
			label: t('federation.instances.domain', 'Domain'),
			type: 'text',
			placeholder: 'bad.example',
			required: true,
			help: t('federation.instances.domain_hint', 'Blocked domains can\u2019t deliver activities to our inbox and aren\u2019t fetched.')
		},
		{
			key: 'reason',
			label: t('federation.instances.reason', 'Reason'),
			type: 'textarea',
			placeholder: t('federation.instances.reason_placeholder', 'Optional operator note')
		}
	];

	const config: EntityPageConfig = {
		title: t('federation.instances.title', 'Instance Blocks'),
		description: t('federation.instances.description', 'Federation domain blocks — inbound activities and outbound fetches are refused for these instances.'),
		createLabel: t('federation.instances.create', 'Block Domain'),
		createPermission: 'federation.instances.manage',
		deletePermission: 'federation.instances.manage',
		pageSize: 25,
		columns: [
			{
				key: 'domain',
				label: t('federation.instances.domain', 'Domain'),
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-foreground)">${v}</span>`
			},
			{
				key: 'reason',
				label: t('federation.instances.reason', 'Reason'),
				render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v ?? '—'}</span>`
			},
			{ key: 'created_at', label: t('common.created_at', 'Created'), componentSnippet: DateCell }
		],
		actions: [
			{
				label: t('common.view', 'View'),
				icon: Eye,
				onclick: (row, ctx) => ctx?.openView(row)
			},
			{
				label: t('federation.instances.unblock', 'Unblock'),
				icon: Trash2,
				variant: 'destructive',
				onclick: (row, ctx) => ctx?.openDelete(row)
			}
		],
		fields,
		load: async ({ page, pageSize, q }) => {
			const params = new URLSearchParams({
				page: String(page + 1), // backend pages are 1-based
				limit: String(pageSize)
			});
			if (q) params.set('q', q);
			const result = await federationAdminApi.listInstances(params.toString());
			if (result.ok) {
				const d = result.data as { data?: InstanceBlock[]; meta?: { total?: number; total_pages?: number } };
				return { data: d.data ?? [], total: d.meta?.total ?? 0, totalPages: d.meta?.total_pages ?? 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load instance blocks'));
		},
		create: async (body) => {
			const res = await federationAdminApi.createInstance({
				domain: String(body.domain ?? '').trim(),
				reason: String(body.reason ?? '').trim() || undefined
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await federationAdminApi.deleteInstance(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: BlockView
	};
</script>

<svelte:head>
	<title>{t('federation.instances.title', 'Instance Blocks')} — {siteName()} Admin</title>
</svelte:head>

<EntityPage {config} />

{#snippet DateCell({ value }: { value: any; row: InstanceBlock })}
	<span class="text-xs" style="color: var(--color-muted)">{value ? new Date(String(value)).toLocaleString() : '—'}</span>
{/snippet}

{#snippet BlockView({ row }: { row: InstanceBlock })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.instances.domain', 'Domain')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.domain}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.instances.reason', 'Reason')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.reason || '—'}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created_at', 'Created')}</p>
			<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
		</div>
	</div>
{/snippet}
