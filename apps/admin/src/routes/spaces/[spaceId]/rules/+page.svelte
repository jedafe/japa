<script lang="ts">
	// /spaces/[spaceId]/rules — migrated to shared components (ActionMenu,
	// SlideOver, shared Modal). Reorder arrows stay as a componentSnippet
	// (inline up/down UI that doesn't fit the ⋮ dropdown). Create/edit open
	// in a SlideOver; delete confirmation uses the shared Modal.
	import { page } from '$app/state';
	import { Modal } from '$lib/components/ui/modal';
	import { SlideOver } from '$lib/components/ui/slide-over';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { ActionItem } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Plus, Pencil, Trash2, RefreshCw, ArrowUp, ArrowDown } from 'lucide-svelte';

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

	let showSlideOver = $state(false);
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
		{ key: 'order', label: t('space_rules.order'), class: 'w-20', componentSnippet: RuleOrderCell }
	];

	const rowActions: ActionItem[] = [
		{
			label: t('common.edit'),
			icon: Pencil,
			onclick: (row: Rule) => openEdit(row),
			show: () => canManage
		},
		{
			label: t('common.delete'),
			icon: Trash2,
			variant: 'destructive',
			onclick: (row: Rule) => confirmDelete(row),
			show: () => canManage
		}
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
		showSlideOver = true;
	}

	function openEdit(rule: Rule) {
		editingRule = rule;
		formTitle = rule.title;
		formDescription = rule.description;
		showSlideOver = true;
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
			showSlideOver = false;
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
			<ArrowUp class="h-3 w-3" />
		</Button>
		<Button variant="ghost" size="sm" disabled={idx === rules.length - 1} onclick={() => moveDown(row, idx)}>
			<ArrowDown class="h-3 w-3" />
		</Button>
	</div>
{/snippet}

<svelte:head>
	<title>Space Rules — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('space_rules.title')}
	description={t('space_rules.description')}
	actions={[
		{ label: t('common.refresh'), icon: RefreshCw, variant: 'outline' as const, onclick: load },
		...(canManage ? [{ label: t('space_rules.add'), icon: Plus, onclick: openCreate }] : [])
	]}
/>

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
	actions={rowActions}
/>

<!-- Create / Edit SlideOver -->
<SlideOver bind:open={showSlideOver} title={editingRule ? t('space_rules.edit_title') : t('space_rules.add_title')}>
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
	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => { showSlideOver = false; }}>{t('common.cancel')}</Button>
			<Button onclick={saveRule} disabled={saving || !formTitle.trim()}>
				{saving ? t('common.saving') : t('common.save')}
			</Button>
		</div>
	{/snippet}
</SlideOver>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDelete} title={t('space_rules.delete_title')} size="sm">
	<p class="text-sm">{t('space_rules.delete_text').replace('{title}', deletingRule?.title ?? '')}</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showDelete = false; }}>{t('common.cancel')}</Button>
		<Button variant="destructive" onclick={deleteRule}>{t('common.delete')}</Button>
	</div>
</Modal>
