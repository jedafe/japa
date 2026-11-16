<script lang="ts">
	import { siteName } from '#lib/stores/site.svelte.js';
	// /federation/actors — §126 member federated actors list + M3a force rotate-key (incident path).
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '#lib/components/entity/index.js';
	import { Modal } from '#lib/components/ui/modal/index.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { federationAdminApi } from '#lib/api.js';
	import { hasPermission } from '#lib/stores/permissions.svelte.js';
	import { t } from '#lib/stores/i18n.svelte.js';
	import { Eye, KeyRound } from 'lucide-svelte';

	type FederatedActor = {
		id: string;
		handle: string;
		username: string;
		display_name?: string | null;
		actor_url: string;
		status: string;
		public_key_id?: string | null;
		key_rotated_at?: string | null;
		created_at: string;
	};

	let entityController = $state<EntityPageController | undefined>(undefined);
	let rotateTarget = $state<FederatedActor | null>(null);
	let showRotateConfirm = $state(false);
	let acting = $state(false);
	let pageError = $state('');

	function openRotate(row: FederatedActor) {
		rotateTarget = row;
		pageError = '';
		showRotateConfirm = true;
	}

	async function confirmRotate() {
		if (!rotateTarget || acting) return;
		acting = true;
		pageError = '';
		try {
			const res = await federationAdminApi.rotateActorKey(rotateTarget.id);
			if (res.ok) {
				rotateTarget = null;
				showRotateConfirm = false;
				entityController?.reload();
			} else {
				pageError = res.error ?? t('common.error', 'Key rotation failed');
			}
		} finally {
			acting = false;
		}
	}

	// No create/edit for actors — members provision their own via the site.
	const fields: FieldDef[] = [];

	const config: EntityPageConfig = {
		title: t('federation.actors.title', 'Federated Actors'),
		description: t('federation.actors.description', 'Member Person actors serving ActivityPub. Rotate-key is the incident path — it bypasses the member cooldown.'),
		hideCreate: true,
		pageSize: 25,
		columns: [
			{
				key: 'handle',
				label: t('federation.actors.handle', 'Handle'),
				render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-foreground)">${v}</span>`
			},
			{
				key: 'display_name',
				label: t('federation.actors.display_name', 'Display name'),
				render: (v: string | null) => `<span class="text-xs" style="color: var(--color-muted)">${v ?? '—'}</span>`
			},
			{
				key: 'actor_url',
				label: t('federation.actors.actor_url', 'Actor URL'),
				sortable: true,
				render: (v: string) => `<span class="font-mono text-xs break-all" style="color: var(--color-muted)">${v}</span>`
			},
			{ key: 'status', label: t('federation.actors.status', 'Status'), sortable: true, componentSnippet: StatusCell },
			{ key: 'created_at', label: t('common.created_at', 'Created'), sortable: true, componentSnippet: DateCell }
		],
		actions: [
			{
				label: t('common.view', 'View'),
				icon: Eye,
				onclick: (row, ctx) => ctx?.openView(row)
			},
			{
				label: t('federation.actors.rotate_key', 'Rotate Key'),
				icon: KeyRound,
				show: (row: FederatedActor) => row.status === 'active' && hasPermission('federation.actors.manage'),
				onclick: (row: FederatedActor) => openRotate(row)
			}
		],
		fields,
		load: async ({ page, pageSize, q, sort }) => {
			const params = new URLSearchParams({
				page: String(page + 1), // backend pages are 1-based
				limit: String(pageSize)
			});
			if (q) params.set('q', q);
			if (sort) {
				const [key, dir] = sort.split(':');
				params.set('sort', key);
				params.set('dir', dir === 'desc' ? 'desc' : 'asc');
			}
			const result = await federationAdminApi.listActors(params.toString());
			if (result.ok) {
				const d = result.data as { data?: FederatedActor[]; meta?: { total?: number; total_pages?: number } };
				return { data: d.data ?? [], total: d.meta?.total ?? 0, totalPages: d.meta?.total_pages ?? 1 };
			}
			throw new Error(result.error ?? t('common.error', 'Failed to load federated actors'));
		},
		viewSnippet: ActorView
	};
</script>

<svelte:head>
	<title>{t('federation.actors.title', 'Federated Actors')} — {siteName()} Admin</title>
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

<!-- Rotate-key confirm — §126 M3a incident path. -->
<Modal bind:open={showRotateConfirm} title={t('federation.actors.rotate_key', 'Rotate Key')}>
	{#if rotateTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('federation.actors.rotate_confirm', 'Force-rotate the signing keypair for')} <strong>{rotateTarget.handle}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-muted)">
			{t('federation.actors.rotate_note', 'The member cooldown is bypassed. Remote instances re-learn the new key via an Update{Person} fanout to the actor\u2019s followers; in-flight deliveries re-sign with the rotated key.')}
		</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showRotateConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="default" onclick={confirmRotate} disabled={acting}>
			{acting ? t('common.working', 'Working…') : t('federation.actors.rotate_key', 'Rotate Key')}
		</Button>
	</div>
</Modal>

{#snippet StatusCell({ value }: { value: any; row: FederatedActor })}
	{@const tones: Record<string, string> = {
		active: 'bg-success/15 text-success',
		disabled: 'bg-warning/15 text-warning',
		tombstone: 'bg-danger/15 text-danger'
	}}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {tones[String(value)] ?? 'bg-muted/15 text-muted'}">
		{value}
	</span>
{/snippet}

{#snippet DateCell({ value }: { value: any; row: FederatedActor })}
	<span class="text-xs" style="color: var(--color-muted)">{value ? new Date(String(value)).toLocaleString() : '—'}</span>
{/snippet}

{#snippet ActorView({ row }: { row: FederatedActor })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.actors.handle', 'Handle')}</p>
			<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{row.handle}</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.actors.actor_url', 'Actor URL')}</p>
			<p class="mt-1 break-all text-sm font-mono" style="color: var(--color-foreground)">{row.actor_url}</p>
		</div>
		{#if row.public_key_id}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.actors.key_id', 'Public key ID')}</p>
				<p class="mt-1 break-all text-xs font-mono" style="color: var(--color-muted)">{row.public_key_id}</p>
			</div>
		{/if}
		<div class="grid grid-cols-3 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.actors.status', 'Status')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.status}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('federation.actors.key_rotated', 'Key rotated')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.key_rotated_at ? new Date(row.key_rotated_at).toLocaleString() : '—'}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('common.created_at', 'Created')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.created_at ? new Date(row.created_at).toLocaleString() : '—'}</p>
			</div>
		</div>
	</div>
{/snippet}
