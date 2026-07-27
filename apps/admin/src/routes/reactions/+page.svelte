<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { reactionsAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { FilterBar } from '$lib/components/ui/filter-bar';
	import { Button } from '$lib/components/ui/button';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Eye, Pencil, Trash2 } from 'lucide-svelte';

	type ReactionType = {
		key: string;
		emoji: string;
		label: string;
		enabled: boolean;
		position: number;
		created_at: string;
	};

	let reactions = $state<ReactionType[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');
	let searchQuery = $state('');
	let filterValues = $state<Record<string, string>>({});

	let showModal = $state(false);
	let editingReaction = $state<ReactionType | null>(null);
	let form = $state({ key: '', label: '', emoji: '', enabled: true });
	let saving = $state(false);

	let showDeleteModal = $state(false);
	let deleteTarget = $state<ReactionType | null>(null);

	let showViewModal = $state(false);
	let viewTarget = $state<ReactionType | null>(null);

	const canManageReactions = $derived(hasPermission('content.reaction_types.manage'));

	const filterDefs = [
		{
			key: 'enabled',
			label: t('reactions.status', 'Status'),
			options: [
				{ value: 'true', label: t('reactions.active', 'Active') },
				{ value: 'false', label: t('reactions.inactive', 'Inactive') }
			]
		}
	];

	const columns = [
		{ key: 'emoji', label: t('reactions.emoji'), class: 'w-16', componentSnippet: EmojiCell },
		{ key: 'label', label: t('reactions.name'), sortable: true },
		{ key: 'key', label: t('reactions.key', 'Key'), sortable: true, render: (v: string) => `<span class="font-mono text-xs" style="color: var(--color-muted)">${v}</span>` },
		{ key: 'enabled', label: t('reactions.active'), sortable: true, componentSnippet: StatusCell },
		{ key: 'actions', label: t('common.actions', 'Actions'), class: 'w-32', componentSnippet: ReactionActionsCell }
	];

	const filteredReactions = $derived(
		searchQuery
			? reactions.filter(
					(r) =>
						r.key.toLowerCase().includes(searchQuery.toLowerCase()) ||
						r.label.toLowerCase().includes(searchQuery.toLowerCase()) ||
						r.emoji.toLowerCase().includes(searchQuery.toLowerCase())
				)
			: reactions
	);

	function openView(reaction: ReactionType) {
		viewTarget = reaction;
		showViewModal = true;
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const params = new URLSearchParams();
			if (filterValues.enabled) params.set('enabled', filterValues.enabled);
			if (searchQuery) params.set('q', searchQuery);
			const result = await reactionsAdminApi.list(params.toString() || undefined);
			if (result.ok) {
				reactions = (result.data as ReactionType[]) ?? [];
			} else {
				errorMessage = result.error as string;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function handleSearch(values: Record<string, string>) {
		searchQuery = values.search ?? '';
		filterValues = values;
		load();
	}

	function handleClear() {
		searchQuery = '';
		filterValues = {};
		load();
	}

	function openCreate() {
		editingReaction = null;
		form = { key: '', label: '', emoji: '', enabled: true };
		showModal = true;
	}

	function openEdit(reaction: ReactionType) {
		editingReaction = reaction;
		form = {
			key: reaction.key,
			label: reaction.label,
			emoji: reaction.emoji,
			enabled: reaction.enabled
		};
		showModal = true;
	}

	async function submit() {
		if (!form.key.trim() || !form.label.trim() || !form.emoji.trim()) return;
		saving = true;
		errorMessage = '';
		const body = {
			key: form.key.trim(),
			label: form.label.trim(),
			emoji: form.emoji.trim(),
			enabled: form.enabled
		};
		const result = editingReaction
			? await reactionsAdminApi.update(editingReaction.key, body)
			: await reactionsAdminApi.create(body);
		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(reaction: ReactionType) {
		deleteTarget = reaction;
		showDeleteModal = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		const result = await reactionsAdminApi.delete(deleteTarget.key);
		if (result.ok) {
			showDeleteModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('reactions.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('reactions.title')}</h1>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('reactions.manage_reaction_types_used_across_the_pl')}</p>

<div class="mb-4 flex justify-end">
	{#if canManageReactions}
		<button
			class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
			style="background-color: var(--color-primary)"
			onclick={openCreate}
		>
			{t('reactions.create')}
		</button>
	{/if}
</div>

<FilterBar filters={filterDefs} onsearch={handleSearch} onclear={handleClear} />

{#if loading && reactions.length === 0}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<DataTable
		{columns}
		data={filteredReactions}
		searchable={true}
		searchPlaceholder={t('common.search', 'Search')}
		onsearch={(q: string) => { searchQuery = q; }}
		pageSize={25}
		emptyMessage={t('reactions.no_reaction_types', 'No reaction types found')}
	/>
{/if}

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('reactions.view')}>
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reactions.emoji')}</p>
				<p class="mt-1 text-2xl" style="color: var(--color-foreground)">{viewTarget.emoji}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reactions.name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.label}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reactions.key', 'Key')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.key}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('reactions.active')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.enabled ? t('reactions.active') : t('reactions.inactive')}</p>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<button class="rounded-md px-4 py-2 text-sm font-medium transition-colors" style="border: 1px solid var(--color-border)" onclick={() => (showViewModal = false)}>{t('common.close')}</button>
	</div>
</Modal>

<!-- Create/Edit Modal -->
<Modal
	bind:open={showModal}
	title={editingReaction ? t('reactions.edit') : t('reactions.create')}
	onConfirm={submit}
	confirmLabel={saving ? t('common.saving') : editingReaction ? t('common.save') : t('reactions.create')}
>
	<div class="space-y-3">
		<div>
			<label for="rx-key" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('reactions.key', 'Key')}</label>
			<input
				id="rx-key"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="like"
				bind:value={form.key}
				disabled={!!editingReaction}
			/>
		</div>
		<div>
			<label for="rx-emoji" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('reactions.emoji')}</label>
			<input
				id="rx-emoji"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="👍"
				bind:value={form.emoji}
			/>
		</div>
		<div>
			<label for="rx-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('reactions.name')}</label>
			<input
				id="rx-name"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder="Like"
				bind:value={form.label}
			/>
		</div>
		<div class="flex items-center gap-2">
			<input id="rx-active" type="checkbox" bind:checked={form.enabled} />
			<label for="rx-active" class="text-sm">{t('reactions.active')}</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteModal} title={t('common.confirm')} onConfirm={executeDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">
		{t('reactions.delete_confirm', 'Are you sure you want to delete the reaction type')} <strong>{deleteTarget?.emoji} {deleteTarget?.label}</strong>?
	</p>
</Modal>

{#snippet EmojiCell({ value }: { value: any; row: ReactionType })}
	<span class="text-2xl">{value}</span>
{/snippet}

{#snippet StatusCell({ value }: { value: boolean; row: ReactionType })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">
		{value ? t('reactions.active') : t('reactions.inactive')}
	</span>
{/snippet}

{#snippet ReactionActionsCell({ row }: { value: any; row: ReactionType })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(row)} aria-label={t('common.view', 'View')}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageReactions}
			<Button variant="ghost" size="sm" onclick={() => openEdit(row)} aria-label={t('common.edit', 'Edit')}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)" aria-label={t('common.delete', 'Delete')}>
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
