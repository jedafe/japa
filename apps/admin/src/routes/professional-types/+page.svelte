<script lang="ts">
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Badge } from '$lib/components/ui/badge';
	import { professionalAdminApi, type ProfileType } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, RefreshCw, Settings2, Eye, Pencil, Trash2 } from 'lucide-svelte';

	let types = $state<ProfileType[]>([]);
	let loading = $state(true);
	let error = $state('');

	let showModal = $state(false);
	let saving = $state(false);
	let form = $state({ name: '', slug: '', description: '' });
	let editingType = $state<ProfileType | null>(null);

	let showViewModal = $state(false);
	let viewTarget = $state<ProfileType | null>(null);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<ProfileType | null>(null);

	// Backend gates /profiles/types create/edit/delete on professional_profile.type.create
	const canManageTypes = $derived(hasPermission('professional_profile.type.create'));

	let expandedId = $state<string | null>(null);
	let detailFields = $state<Array<{ name: string; field_type: string; required: boolean }>>([]);
	let detailLoading = $state(false);

	function slugify(text: string): string {
		return text.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
	}

	async function load() {
		loading = true;
		error = '';
		const result = await professionalAdminApi.listTypes();
		if (result.ok) {
			types = result.data ?? [];
		} else {
			error = result.error;
		}
		loading = false;
	}

	async function toggleExpand(id: string) {
		if (expandedId === id) {
			expandedId = null;
			detailFields = [];
			return;
		}
		expandedId = id;
		detailLoading = true;
		detailFields = [];

		const result = await professionalAdminApi.getType(id);
		if (result.ok && result.data) {
			detailFields = (result.data as any).fields ?? [];
		}
		detailLoading = false;
	}

	function openCreate() {
		editingType = null;
		form = { name: '', slug: '', description: '' };
		showModal = true;
	}

	function openEdit(pt: ProfileType) {
		editingType = pt;
		form = { name: pt.name, slug: pt.slug, description: pt.description ?? '' };
		showModal = true;
	}

	function openView(pt: ProfileType) {
		viewTarget = pt;
		showViewModal = true;
	}

	function confirmDelete(pt: ProfileType) {
		deleteTarget = pt;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		error = '';
		try {
			const result = await professionalAdminApi.deleteType(deleteTarget.id);
			if (result.ok) {
				showDeleteConfirm = false;
				deleteTarget = null;
				await load();
			} else {
				error = result.error;
			}
		} catch {
			error = t('common.error');
		}
	}

	async function submit() {
		if (!form.name.trim()) return;
		saving = true;
		error = '';

		const slug = form.slug.trim() || slugify(form.name);
		const body = { name: form.name.trim(), slug, description: form.description.trim() };
		const result = editingType
			? await professionalAdminApi.updateType(editingType.id, body)
			: await professionalAdminApi.createType(body);

		if (result.ok) {
			showModal = false;
			editingType = null;
			await load();
		} else {
			error = result.error;
		}
		saving = false;
	}

	const columns = [
		{ key: 'name', label: t('professional_types.name', 'Name'), sortable: true },
		{ key: 'slug', label: t('professional_types.slug', 'Slug'), class: 'font-mono text-xs' },
		{ key: 'description', label: t('professional_types.description', 'Description'), render: (v: string) => v || '—' },
		{ key: 'profiles', label: t('professional_types.profiles', 'Profiles'), render: () => '—' },
		{ key: 'actions', label: t('common.actions', 'Actions'), class: 'w-32', component: TypeActionsCell }
	];

	function handleRowClick(row: ProfileType) {
		toggleExpand(row.id);
	}
</script>

<PageHeader
	title={t('professional_types.title', 'Profile Types')}
	description={t('professional_types.description', 'Manage professional profile types, members, and invitations')}
	actions={[
		...(canManageTypes ? [{ label: t('professional_types.add', 'Add Profile Type'), icon: Plus, onclick: openCreate }] : []),
		{ label: t('common.refresh', 'Refresh'), icon: RefreshCw, variant: 'outline', onclick: load }
	]}
/>

{#if error}
	<div class="mb-4 rounded-lg border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{error}
	</div>
{/if}

<Card>
	<CardContent class="p-0">
		{#if loading}
			<div class="flex items-center justify-center py-12">
				<p class="text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
			</div>
		{:else}
			<DataTable
				{columns}
				data={types}
				onrowclick={handleRowClick}
				emptyMessage={t('professional_types.empty', 'No profile types configured')}
			/>
		{/if}
	</CardContent>
</Card>

{#if expandedId && !loading}
	{@const selectedType = types.find(pt => pt.id === expandedId)}
	<Card class="mt-4">
		<CardHeader>
			<div class="flex items-center justify-between">
				<h3 class="flex items-center gap-2 text-sm font-semibold">
					<Settings2 class="h-4 w-4" style="color: var(--color-primary)" />
					{selectedType?.name ?? t('professional_types.fields', 'Custom Fields')}
				</h3>
				<Button variant="ghost" size="sm" onclick={() => { expandedId = null; detailFields = []; }}>
					{t('common.close', 'Close')}
				</Button>
			</div>
		</CardHeader>
		<CardContent>
			{#if detailLoading}
				<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('common.loading', 'Loading...')}</p>
			{:else if detailFields.length > 0}
				<div class="space-y-1">
					{#each detailFields as field}
						<div class="flex items-center gap-2 rounded-lg border px-3 py-2 text-xs" style="border-color: var(--color-border)">
							<span class="font-medium">{field.name}</span>
							<Badge variant="default">{field.field_type}</Badge>
							{#if field.required}
								<Badge variant="warning">required</Badge>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<p class="text-xs" style="color: var(--color-muted)">{t('professional_types.no_fields', 'No custom fields defined')}</p>
			{/if}
		</CardContent>
	</Card>
{/if}

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('professional_types.details', 'Profile Type Details')} size="sm">
	{#if viewTarget}
		<div class="space-y-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('professional_types.name', 'Name')}</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('professional_types.slug', 'Slug')}</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.slug}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('professional_types.description', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{viewTarget.description || '—'}</p>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('common.close', 'Close')}</Button>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('common.confirmDelete', 'Confirm Delete')}>
	{#if deleteTarget}
		<p class="text-sm" style="color: var(--color-foreground)">
			{t('professional_types.delete_confirm', 'Delete profile type')} <strong>{deleteTarget.name}</strong>?
		</p>
		<p class="mt-2 text-sm" style="color: var(--color-danger)">{t('common.cannotUndo', 'This action cannot be undone.')}</p>
	{/if}
	<div class="mt-4 flex justify-end gap-2">
		<Button variant="ghost" onclick={() => (showDeleteConfirm = false)}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={executeDelete}>{t('common.delete', 'Delete')}</Button>
	</div>
</Modal>

<Modal bind:open={showModal} title={editingType ? t('professional_types.edit', 'Edit Profile Type') : t('professional_types.add', 'Add Profile Type')}>
	<form class="space-y-4" onsubmit={(e) => { e.preventDefault(); submit(); }}>
		<div>
			<label for="pt-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('professional_types.name', 'Name')}</label>
			<input
				id="pt-name"
				type="text"
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				bind:value={form.name}
				oninput={() => { form.slug = slugify(form.name); }}
			/>
		</div>
		<div>
			<label for="pt-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('professional_types.slug', 'Slug')}</label>
			<input
				id="pt-slug"
				type="text"
				class="w-full rounded-lg border px-3 py-2 text-sm font-mono"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				bind:value={form.slug}
			/>
		</div>
		<div>
			<label for="pt-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('professional_types.description', 'Description')}</label>
			<textarea
				id="pt-desc"
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background)"
				rows="2"
				bind:value={form.description}
			></textarea>
		</div>
		<div class="flex justify-end gap-2 pt-2">
			<Button variant="outline" type="button" onclick={() => showModal = false}>{t('common.cancel', 'Cancel')}</Button>
			<Button type="submit" disabled={saving}>{saving ? t('common.saving', 'Saving...') : editingType ? t('common.save', 'Save') : t('common.create', 'Create')}</Button>
		</div>
	</form>
</Modal>

<!-- TypeActionsCell component -->
{#snippet TypeActionsCell(pt: ProfileType)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openView(pt)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageTypes}
			<Button variant="ghost" size="sm" onclick={() => openEdit(pt)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(pt)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
