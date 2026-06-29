<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	type Rule = {
		id: string;
		title: string;
		description: string;
		sort_order: number;
		created_at: string;
	};

	const spaceId = $derived($page.params.spaceId);

	let rules = $state<Rule[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	// Create / Edit modal
	let showModal = $state(false);
	let editingRule = $state<Rule | null>(null);
	let formTitle = $state('');
	let formDescription = $state('');
	let saving = $state(false);

	// Delete confirmation
	let showDelete = $state(false);
	let deletingRule = $state<Rule | null>(null);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<Rule[]>(`/spaces/${spaceId}/rules`);
			if (result.ok) {
				rules = result.data.sort((a, b) => a.sort_order - b.sort_order);
			}
		} catch {
			errorMessage = t('space_rules.load_error');
		} finally {
			loading = false;
		}
	}

	function openCreate() {
		editingRule = null;
		formTitle = '';
		formDescription = '';
		showModal = true;
	}

	function openEdit(rule: Rule) {
		editingRule = rule;
		formTitle = rule.title;
		formDescription = rule.description;
		showModal = true;
	}

	async function saveRule() {
		if (!formTitle.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			if (editingRule) {
				await api(`/spaces/${spaceId}/rules/${editingRule.id}`, {
					method: 'PATCH',
					body: { title: formTitle.trim(), description: formDescription.trim() }
				});
			} else {
				await api(`/spaces/${spaceId}/rules`, {
					method: 'POST',
					body: { title: formTitle.trim(), description: formDescription.trim(), sort_order: rules.length }
				});
			}
			showModal = false;
			load();
		} catch {
			errorMessage = t('space_rules.save_error');
		} finally {
			saving = false;
		}
	}

	function confirmDelete(rule: Rule) {
		deletingRule = rule;
		showDelete = true;
	}

	async function deleteRule() {
		if (!deletingRule) return;
		try {
			await api(`/spaces/${spaceId}/rules/${deletingRule.id}`, { method: 'DELETE' });
			showDelete = false;
			load();
		} catch {
			errorMessage = t('space_rules.delete_error');
		}
	}

	async function moveUp(rule: Rule, index: number) {
		if (index === 0) return;
		const prev = rules[index - 1];
		try {
			await api(`/spaces/${spaceId}/rules/${rule.id}`, { method: 'PATCH', body: { sort_order: prev.sort_order } });
			await api(`/spaces/${spaceId}/rules/${prev.id}`, { method: 'PATCH', body: { sort_order: rule.sort_order } });
			load();
		} catch {
			errorMessage = t('space_rules.reorder_error');
		}
	}

	async function moveDown(rule: Rule, index: number) {
		if (index >= rules.length - 1) return;
		const next = rules[index + 1];
		try {
			await api(`/spaces/${spaceId}/rules/${rule.id}`, { method: 'PATCH', body: { sort_order: next.sort_order } });
			await api(`/spaces/${spaceId}/rules/${next.id}`, { method: 'PATCH', body: { sort_order: rule.sort_order } });
			load();
		} catch {
			errorMessage = t('space_rules.reorder_error');
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>Space Rules — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('space_rules.title')}</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('space_rules.description')}</p>
	</div>
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('space_rules.add')}
	</button>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if rules.length === 0}
	<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<p class="text-sm" style="color: var(--color-muted)">{t('space_rules.empty')}</p>
	</div>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">#</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space_rules.title_label')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space_rules.description_label')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('space_rules.order')}</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each rules as rule, i}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{i + 1}</td>
						<td class="px-4 py-3 font-medium">{rule.title}</td>
						<td class="px-4 py-3 text-xs max-w-xs truncate" style="color: var(--color-muted)">{rule.description || '—'}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="rounded p-0.5 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-30"
									disabled={i === 0}
									onclick={() => moveUp(rule, i)}
									aria-label="Move up"
								>&#9650;</button>
								<button
									class="rounded p-0.5 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-30"
									disabled={i === rules.length - 1}
									onclick={() => moveDown(rule, i)}
									aria-label="Move down"
								>&#9660;</button>
							</div>
						</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-1">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openEdit(rule)}
								>
								{t('common.edit')}
							</button>
							<button
								class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
								style="color: var(--color-danger)"
								onclick={() => confirmDelete(rule)}
							>
								{t('common.delete')}
								</button>
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<!-- Create / Edit Modal -->
<Modal bind:open={showModal} title={editingRule ? t('space_rules.edit_title') : t('space_rules.add_title')} onConfirm={saveRule} confirmLabel={saving ? t('common.saving') : t('common.save')}>
	<div class="space-y-4">
		<div>
			<label for="rule-title" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space_rules.title_label')}</label>
			<input
				id="rule-title"
				type="text"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				placeholder={t('space_rules.title_placeholder')}
				bind:value={formTitle}
			/>
		</div>
		<div>
			<label for="rule-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space_rules.description_label')}</label>
			<textarea
				id="rule-desc"
				class="w-full rounded-md border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-surface)"
				rows="3"
				placeholder={t('space_rules.desc_placeholder')}
				bind:value={formDescription}
			></textarea>
		</div>
	</div>
</Modal>

<!-- Delete Confirmation -->
<Modal bind:open={showDelete} title={t('space_rules.delete_title')} onConfirm={deleteRule} confirmLabel={t('common.delete')}>
	<p class="text-sm">{t('space_rules.delete_text').replace('{title}', deletingRule?.title ?? '')}</p>
</Modal>
