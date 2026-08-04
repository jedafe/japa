<script lang="ts">
	import { page } from '$app/state';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';

	// Ref: jc-admin-develop — RBAC gate for rule management
	const canManage = $derived(hasPermission('space.rules.manage'));

	type Rule = {
		id: string;
		title: string;
		description: string;
		sort_order: number;
		created_at: string;
	};

	const spaceId = $derived(page.params.spaceId);

	let rules = $state<Rule[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	let searchQuery = $state('');

	let showModal = $state(false);
	let editingRule = $state<Rule | null>(null);
	let formTitle = $state('');
	let formDescription = $state('');
	let saving = $state(false);

	let showDelete = $state(false);
	let deletingRule = $state<Rule | null>(null);

	const columns = [
		{ key: 'sort_order', label: '#', sortable: true, class: 'text-xs', render: (_v: number, _row: Rule, i?: number) => String((i ?? 0) + 1) },
		{ key: 'title', label: t('space_rules.title_label'), sortable: true },
		{ key: 'description', label: t('space_rules.description_label'), sortable: true, class: 'text-xs max-w-xs truncate', render: (v: string) => `<span style="color: var(--color-muted)">${v || '—'}</span>` },
		{ key: 'order', label: t('space_rules.order'), class: 'w-20', componentSnippet: RuleOrderCell },
		{ key: 'actions', label: '', class: 'w-32', componentSnippet: RuleActionsCell }
	];

	function filteredRules(): Rule[] {
		const q = searchQuery.toLowerCase().trim();
		if (!q) return rules;
		return rules.filter(r =>
			r.title.toLowerCase().includes(q) ||
			(r.description ?? '').toLowerCase().includes(q)
		);
	}

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

	$effect(() => {
		load();
	});
</script>

{#snippet RuleOrderCell({ value, row }: { value: any; row: Rule })}
	{@const idx = rules.findIndex(r => r.id === row.id)}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" disabled={idx === 0} onclick={() => moveUp(row, idx)}>
			&#9650;
		</Button>
		<Button variant="ghost" size="sm" disabled={idx === rules.length - 1} onclick={() => moveDown(row, idx)}>
			&#9660;
		</Button>
	</div>
{/snippet}

{#snippet RuleActionsCell({ value, row }: { value: any; row: Rule })}
	{#if canManage}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openEdit(row)} style="color: var(--color-info)">
			{t('common.edit')}
		</Button>
		<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
			{t('common.delete')}
		</Button>
	</div>
	{/if}
{/snippet}

<svelte:head>
	<title>Space Rules — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{t('space_rules.title')}</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('space_rules.description')}</p>
	</div>
	{#if canManage}
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		{t('space_rules.add')}
	</button>
	{/if}
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<DataTable
	columns={columns}
	data={filteredRules()}
	{loading}
	pageSize={25}
	emptyMessage={t('space_rules.empty')}
	searchable
	searchPlaceholder={t('common.search')}
	onsearch={(q) => { searchQuery = q; }}
/>

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
