<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import { comparisonAdminApi, type ComparisonType, type ComparisonTypeDetail, type ComparisonTypeAccess } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { GitCompare, ChevronDown, ChevronRight, Shield } from 'lucide-svelte';

	let types = $state<ComparisonType[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	let form = $state({
		name: '',
		slug: '',
		description: '',
		is_public: false
	});

	// Expanded detail state
	let expandedId = $state<string | null>(null);
	let detail = $state<ComparisonTypeDetail | null>(null);
	let detailLoading = $state(false);
	let access = $state<ComparisonTypeAccess | null>(null);
	let accessForm = $state({ allowed_groups: '', allowed_roles: '' });
	let accessSaving = $state(false);

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	async function load() {
		loading = true;
		errorMessage = '';
		const result = await comparisonAdminApi.listTypes();
		if (result.ok) {
			types = result.data ?? [];
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	async function toggleExpand(id: string) {
		if (expandedId === id) {
			expandedId = null;
			detail = null;
			access = null;
			return;
		}
		expandedId = id;
		detailLoading = true;
		detail = null;
		access = null;

		const [detailResult, accessResult] = await Promise.all([
			comparisonAdminApi.getType(id),
			comparisonAdminApi.getTypeAccess(id)
		]);

		if (detailResult.ok) {
			detail = detailResult.data;
		}
		if (accessResult.ok) {
			access = accessResult.data;
			accessForm = {
				allowed_groups: accessResult.data.allowed_groups.join(', '),
				allowed_roles: accessResult.data.allowed_roles.join(', ')
			};
		}
		detailLoading = false;
	}

	function openCreate() {
		editingId = null;
		form = { name: '', slug: '', description: '', is_public: false };
		showModal = true;
	}

	function openEdit(item: ComparisonType) {
		editingId = item.id;
		form = {
			name: item.name,
			slug: item.slug,
			description: item.description ?? '',
			is_public: item.is_public
		};
		showModal = true;
	}

	async function submit() {
		if (!form.name.trim()) return;
		saving = true;
		errorMessage = '';

		const slug = form.slug.trim() || slugify(form.name);
		const body = {
			name: form.name.trim(),
			slug,
			description: form.description.trim(),
			is_public: form.is_public
		};

		const result = editingId
			? await comparisonAdminApi.updateType(editingId, { name: body.name, description: body.description, is_public: body.is_public })
			: await comparisonAdminApi.createType(body);

		if (result.ok) {
			showModal = false;
			load();
			if (editingId && expandedId === editingId) {
				toggleExpand(editingId);
			}
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	function confirmDelete(id: string) {
		deleteTargetId = id;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTargetId) return;
		errorMessage = '';
		const result = await comparisonAdminApi.deleteType(deleteTargetId);
		if (result.ok) {
			showDeleteConfirm = false;
			if (expandedId === deleteTargetId) {
				expandedId = null;
				detail = null;
				access = null;
			}
			deleteTargetId = null;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	async function saveAccess() {
		if (!expandedId) return;
		accessSaving = true;
		const body = {
			allowed_groups: accessForm.allowed_groups.split(',').map(s => s.trim()).filter(Boolean),
			allowed_roles: accessForm.allowed_roles.split(',').map(s => s.trim()).filter(Boolean)
		};
		const result = await comparisonAdminApi.setTypeAccess(expandedId, body);
		if (result.ok) {
			access = body;
		} else {
			errorMessage = result.error;
		}
		accessSaving = false;
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('comparison_types.title')} — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('comparison_types.title')}</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">{t('comparison_types.description')}</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('comparison_types.add')}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('comparison_types.name')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('comparison_types.slug')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('comparison_types.description')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('comparison_types.public')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('comparison_types.items')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each types as item}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3">
							<button
								class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10"
								onclick={() => toggleExpand(item.id)}
								aria-label="Expand details"
							>
								{#if expandedId === item.id}
									<ChevronDown class="h-4 w-4" style="color: var(--color-muted)" />
								{:else}
									<ChevronRight class="h-4 w-4" style="color: var(--color-muted)" />
								{/if}
							</button>
						</td>
						<td class="px-4 py-3 font-medium">{item.name}</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{item.slug}</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{item.description || '—'}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {item.is_public ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-slate-500/15 text-slate-700 dark:text-slate-400'}">
								{item.is_public ? t('common.yes') : t('common.no')}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">—</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-2">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openEdit(item)}
								>
									{t('common.edit')}
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete(item.id)}
								>
									{t('common.delete')}
								</button>
							</div>
						</td>
					</tr>
					{#if expandedId === item.id}
						<tr style="border-color: var(--color-border)">
							<td colspan="7" class="px-4 py-4" style="background-color: var(--color-surface)">
								{#if detailLoading}
									<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
								{:else if detail}
									<div class="space-y-4">
										<!-- Dimensions -->
										<div>
											<h3 class="mb-2 flex items-center gap-2 text-sm font-semibold">
												<GitCompare class="h-4 w-4" style="color: var(--color-primary)" />
												{t('comparison_types.dimensions')}
											</h3>
											{#if detail.dimensions.length > 0}
												<div class="space-y-1">
													{#each detail.dimensions as dim}
														<div class="rounded-md border px-3 py-2 text-xs" style="border-color: var(--color-border)">
															<span class="font-medium">{dim.name}</span>
															{#if dim.description}<span style="color: var(--color-muted)"> — {dim.description}</span>{/if}
														</div>
													{/each}
												</div>
											{:else}
												<p class="text-xs" style="color: var(--color-muted)">No dimensions defined</p>
											{/if}
										</div>

										<!-- Item count -->
										<p class="text-xs" style="color: var(--color-muted)">
											{t('comparison_types.items')}: <span class="font-medium">{detail.item_count}</span>
										</p>

										<!-- Access Control -->
										<div>
											<h3 class="mb-2 flex items-center gap-2 text-sm font-semibold">
												<Shield class="h-4 w-4" style="color: var(--color-primary)" />
												{t('comparison_types.access')}
											</h3>
											<div class="space-y-3">
												<div>
													<label for="ct-allowed-groups" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.allowed_groups')}</label>
													<input
														id="ct-allowed-groups"
														type="text"
														class="w-full rounded-md border px-3 py-2 text-sm"
														style="border-color: var(--color-border); background-color: var(--color-background)"
														placeholder="group1, group2"
														bind:value={accessForm.allowed_groups}
													/>
												</div>
												<div>
													<label for="ct-allowed-roles" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.allowed_roles')}</label>
													<input
														id="ct-allowed-roles"
														type="text"
														class="w-full rounded-md border px-3 py-2 text-sm"
														style="border-color: var(--color-border); background-color: var(--color-background)"
														placeholder="admin, editor"
														bind:value={accessForm.allowed_roles}
													/>
												</div>
												<button
													class="rounded-md px-3 py-1.5 text-xs font-medium text-white transition-colors"
													style="background-color: var(--color-primary)"
													onclick={saveAccess}
													disabled={accessSaving}
												>
													{accessSaving ? t('common.saving') : t('common.save')}
												</button>
											</div>
										</div>
									</div>
								{:else}
									<p class="py-4 text-center text-sm" style="color: var(--color-muted)">{t('common.error')}</p>
								{/if}
							</td>
						</tr>
					{/if}
				{:else}
					<tr>
						<td colspan="7" class="px-4 py-8 text-center" style="color: var(--color-muted)">{t('comparison_types.empty')}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create/Edit Modal -->
<Modal bind:open={showModal} title={editingId ? t('comparison_types.edit') : t('comparison_types.add')} onConfirm={submit} confirmLabel={saving ? t('common.saving') : (editingId ? t('common.save') : t('common.create'))}>
	<div class="space-y-3">
		<div>
			<label for="ct-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.name')}</label>
			<input id="ct-name" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.name} oninput={() => { if (!editingId) form.slug = slugify(form.name); }} />
		</div>
		<div>
			<label for="ct-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.slug')}</label>
			<input id="ct-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.slug} />
		</div>
		<div>
			<label for="ct-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('comparison_types.description')}</label>
			<textarea id="ct-desc" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="2" bind:value={form.description}></textarea>
		</div>
		<div class="flex items-center gap-2">
			<input id="ct-public" type="checkbox" bind:checked={form.is_public} />
			<label for="ct-public" class="text-sm">{t('comparison_types.public')}</label>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title={t('common.delete')} onConfirm={executeDelete} confirmLabel={t('common.delete')}>
	<p class="text-sm" style="color: var(--color-muted)">{t('comparison_types.delete_confirm')}</p>
</Modal>
