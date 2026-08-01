<script lang="ts">
	import { guideCategoriesAdminApi, type GuideCategory } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Plus, Trash2, List, ExternalLink } from 'lucide-svelte';
	import Modal from '$lib/components/Modal.svelte';

	let categories = $state<GuideCategory[]>([]);
	let loading = $state(false);
	let errorMessage = $state('');
	let modalOpen = $state(false);
	let slug = $state('');
	let label = $state('');
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTarget = $state<GuideCategory | null>(null);

	const canManage = $derived(hasPermission('custom_fields.group.create'));
	const canDelete = $derived(hasPermission('custom_fields.group.delete'));

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await guideCategoriesAdminApi.list();
			if (result.ok) {
				categories = (Array.isArray(result.data) ? result.data : []) as GuideCategory[];
			} else {
				errorMessage = result.error;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		slug = '';
		label = '';
		modalOpen = true;
	}

	async function save() {
		if (!slug.trim() || !label.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await guideCategoriesAdminApi.create({
				slug: slug.trim().toLowerCase().replace(/\s+/g, '_'),
				label: label.trim()
			});
			if (!result.ok) {
				errorMessage = result.error;
				saving = false;
				return;
			}
			modalOpen = false;
			await load();
		} catch {
			errorMessage = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDelete(cat: GuideCategory) {
		deleteTarget = cat;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTarget) return;
		errorMessage = '';
		try {
			await guideCategoriesAdminApi.delete(deleteTarget.Slug);
			showDeleteConfirm = false;
			deleteTarget = null;
			await load();
		} catch {
			errorMessage = t('common.error');
		}
	}

	load();
</script>

<div class="space-y-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Guide Categories</h1>
			<p class="mt-1 text-sm" style="color: var(--color-muted)">
				Manage evergreen guide categories for the Guide content type. Each category has its own
				custom field group — click the fields icon to manage fields.
			</p>
		</div>
		{#if canManage}
			<button
				onclick={openCreate}
				class="inline-flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white"
				style="background-color: var(--color-primary)"
			>
				<Plus class="h-4 w-4" />
				{t('common.create')}
			</button>
		{/if}
	</div>

	{#if errorMessage}
		<div class="rounded-lg p-3 text-sm" style="background-color: var(--color-destructive-muted); color: var(--color-destructive)">
			{errorMessage}
		</div>
	{/if}

	{#if loading}
		<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}…</p>
	{:else}
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead style="background-color: var(--color-muted-muted)">
					<tr>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Slug</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Label</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">System</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">Field Group</th>
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted)">{t('common.actions')}</th>
					</tr>
				</thead>
				<tbody>
					{#each categories as cat (cat.Slug)}
						<tr class="border-t" style="border-color: var(--color-border)">
							<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-foreground)">{cat.Slug}</td>
							<td class="px-4 py-3" style="color: var(--color-foreground)">{cat.Label}</td>
							<td class="px-4 py-3">
								{#if cat.is_system}
									<span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" style="background-color: var(--color-primary-muted); color: var(--color-primary)">System</span>
								{:else}
									<span class="text-xs" style="color: var(--color-muted)">—</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								{#if cat.group_id}
									<a
										href={`/custom-fields/${cat.group_id}`}
										class="inline-flex items-center gap-1 text-xs font-medium hover:underline"
										style="color: var(--color-primary)"
									>
										<List class="h-3.5 w-3.5" />
										Manage Fields
										<ExternalLink class="h-3 w-3" />
									</a>
								{:else}
									<span class="text-xs" style="color: var(--color-muted)">No field group</span>
								{/if}
							</td>
							<td class="px-4 py-3">
								<div class="flex items-center gap-2">
									{#if canDelete && !cat.is_system}
										<button onclick={() => confirmDelete(cat)} class="p-1 rounded hover:opacity-70" title={t('common.delete')}>
											<Trash2 class="h-4 w-4" style="color: var(--color-destructive)" />
										</button>
									{/if}
								</div>
							</td>
						</tr>
					{:else}
						<tr class="border-t" style="border-color: var(--color-border)">
							<td colspan="5" class="px-4 py-8 text-center" style="color: var(--color-muted)">
								No guide categories found. Create one to get started.
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<Modal bind:open={modalOpen} title="New Guide Category" confirmLabel={t('common.create')} onConfirm={save}>
	<div class="space-y-4">
		<div>
			<label for="gc-slug" class="mb-1 block text-sm font-medium">Slug</label>
			<input
				id="gc-slug"
				type="text"
				bind:value={slug}
				placeholder="e.g. visa, scholarship, travel_routes"
				required
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				Machine-readable token (snake_case). Cannot be changed after creation. A custom field group
				named "guide_{slug}_fields" will be created automatically.
			</p>
		</div>
		<div>
			<label for="gc-label" class="mb-1 block text-sm font-medium">Label</label>
			<input
				id="gc-label"
				type="text"
				bind:value={label}
				placeholder="e.g. Visa Guide, Scholarship Guide"
				required
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-background); color: var(--color-foreground)"
			/>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				Human-readable name shown in the composer's category dropdown.
			</p>
		</div>
		{#if errorMessage}
			<p class="text-sm" style="color: var(--color-destructive)">{errorMessage}</p>
		{/if}
		{#if saving}
			<p class="text-sm" style="color: var(--color-muted)">{t('common.saving')}…</p>
		{/if}
	</div>
</Modal>

<Modal bind:open={showDeleteConfirm} title="Delete Guide Category" confirmLabel={t('common.delete')} onConfirm={executeDelete}>
	<p class="text-sm" style="color: var(--color-muted)">
		Delete <strong>{deleteTarget?.Label}</strong>? This will remove the category, its custom field
		group, and all fields within that group. This action cannot be undone.
	</p>
</Modal>
