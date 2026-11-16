<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /federation/groups — §126 M3 space Group actors (FEP-1b12). Create provisions; delete tombstones + fans out Delete{Group}.
	import { EntityPage, type EntityPageConfig, type FieldDef } from '#lib/components/entity/index.js';
	import { federationAdminApi, spacesAdminApi } from '#lib/api.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { Eye, Trash2 } from 'lucide-svelte';

	type GroupActor = {
		id: string;
		actor_url: string;
		preferred_username: string;
		status: string;
		space_id?: string;
		space_slug: string;
		space_name: string;
		created_at: string;
	};

	type AdminSpace = { id: string; name: string; slug: string };

	// Space picker — only public + active spaces can hold a Group actor;
	// the backend 409s anything else and the form surfaces that error.
	const spaceLabel = new Map<string, string>();

	async function fetchSpaceOptions(q: string): Promise<{ id: string; label: string; sublabel?: string }[]> {
		const params = new URLSearchParams({ visibility: 'public', status: 'active', limit: '20' });
		if (q) params.set('q', q);
		const res = await spacesAdminApi.list(params.toString());
		if (!res.ok) return [];
		const d = res.data as { data?: { items?: AdminSpace[] } };
		return (d?.data?.items ?? []).map((s) => {
			const label = `${s.name} (${s.slug})`;
			spaceLabel.set(s.id, label);
			return { id: s.id, label, sublabel: s.slug };
		});
	}

	function fetchSpaceOptionsInitial(): Promise<{ id: string; label: string; sublabel?: string }[]> {
		return fetchSpaceOptions('');
	}

	function getSpaceSelectedLabel(id: string): string {
		return spaceLabel.get(id) ?? id;
	}

	const fields: FieldDef[] = [
		{
			key: 'space_id',
			label: t('federation.groups.space', 'Space'),
			type: 'autocomplete',
			placeholder: t('federation.groups.space_select', 'Search public spaces…'),
			emptyMessage: t('common.no_results', 'No results found'),
			fetcher: fetchSpaceOptions,
			initialFetcher: fetchSpaceOptionsInitial,
			getSelectedLabel: getSpaceSelectedLabel,
			required: true,
			help: t('federation.groups.space_hint', 'Only public, active spaces can federate as a Group actor.')
		}
	];

	const config: EntityPageConfig = {
		title: t('federation.groups.title', 'Federation Group Actors'),
		description: t('federation.groups.description', 'Spaces federated as ActivityPub Group actors — remote users follow a space and receive boosted member posts.'),
		createLabel: t('federation.groups.create', 'Provision Group Actor'),
		createPermission: 'federation.actors.manage',
		deletePermission: 'federation.actors.manage',
		pageSize: 25,
		columns: [
			{ key: 'space_name', label: t('federation.groups.space_name', 'Space') },
			{
				key: 'space_slug',
				label: t('federation.groups.space_slug', 'Slug'),
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>`
			},
			{
				key: 'actor_url',
				label: t('federation.groups.actor_url', 'Actor URL'),
				render: (v: string) => `<span class="font-mono text-xs break-all" style="color: var(--color-foreground)">${v}</span>`
			},
			{ key: 'status', label: t('federation.groups.status', 'Status'), componentSnippet: StatusCell },
			{ key: 'created_at', label: t('common.created_at', 'Created'), componentSnippet: DateCell }
		],
		actions: [
			{
				label: t('common.view', 'View'),
				icon: Eye,
				onclick: (row, ctx) => ctx?.openView(row)
			},
			{
				label: t('federation.groups.tombstone', 'Tombstone'),
				icon: Trash2,
				variant: 'destructive',
				show: (row: GroupActor) => row.status === 'active',
				onclick: (row, ctx) => ctx?.openDelete(row)
			}
		],
		fields,
		isDeleted: (row: GroupActor) => row.status === 'tombstone',
		load: async ({ page, pageSize }) => {
			const params = new URLSearchParams({
				page: String(page + 1), // backend pages are 1-based
				limit: String(pageSize)
			});
			const result = await federationAdminApi.listGroups(params.toString());
			if (result.ok) {
				const d = result.data as { groups?: GroupActor[]; total?: number; total_pages?: number };
				return { data: d.groups ?? [], total: d.total ?? 0, totalPages: d.total_pages ?? 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load group actors'));
		},
		create: async (body) => {
			const res = await federationAdminApi.createGroup({ space_id: String(body.space_id ?? '').trim() });
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await federationAdminApi.deleteGroup(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: GroupView
	};
</script>

<svelte:head>
	<title>{t('federation.groups.title', 'Federation Group Actors')} — {siteName()} Admin</title>
</svelte:head>

<EntityPage {config} />

{#snippet StatusCell({ value }: { value: any; row: GroupActor })}
	{@const tones: Record<string, string> = {
		active: 'bg-success/15 text-success',
		disabled: 'bg-warning/15 text-warning',
		tombstone: 'bg-danger/15 text-danger'
	}}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {tones[String(value)] ?? 'bg-muted/15 text-muted'}">
		{value}
	</span>
{/snippet}

{#snippet DateCell({ value }: { value: any; row: GroupActor })}
	<span class="text-xs" style="color: var(--color-muted)">{value ? new Date(String(value)).toLocaleString() : '—'}</span>
{/snippet}

{#snippet GroupView({ row }: { row: GroupActor })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.groups.space_name', 'Space')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.space_name} <span class="font-mono text-xs" style="color: var(--color-muted)">({row.space_slug})</span></p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.groups.actor_url', 'Actor URL')}</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.actor_url}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.groups.handle', 'Handle')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">!{row.preferred_username}</p>
		</div>
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.groups.status', 'Status')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created_at', 'Created')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
			</div>
		</div>
	</div>
{/snippet}
