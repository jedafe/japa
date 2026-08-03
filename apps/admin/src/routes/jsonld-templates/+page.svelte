<script lang="ts">
	import { jsonLdApi, type JsonLdTemplate } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { Plus, RefreshCw, Pencil, Trash2, Download, Upload, Eye } from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type TemplateForm = {
		content_type: string;
		is_active: boolean;
		content: string;
	};

	let templates = $state<JsonLdTemplate[]>([]);
	let loading = $state(true);
	let error = $state('');
	let success = $state('');
	let selectedIds = $state<Set<string>>(new Set());
	let bulkDeleting = $state(false);

	// Modal state
	let showTemplateModal = $state(false);
	let editingTemplate = $state<JsonLdTemplate | null>(null);
	let templateForm = $state<TemplateForm>({ content_type: '', is_active: true, content: '{}' });
	let saving = $state(false);
	let jsonError = $state('');

	// View modal
	let showViewModal = $state(false);
	let viewTarget = $state<JsonLdTemplate | null>(null);

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deletingTemplate = $state<JsonLdTemplate | null>(null);

	const canManageTemplates = $derived(hasPermission('content.jsonld.manage'));

	// Header actions are permission-gated: Refresh/Export are read-only and
	// available to any authenticated admin; Add/Import mutate and require
	// the content.jsonld.manage node (§9 RBAC, §26 wiring).
	const headerActions = $derived([
		{ label: t('jsonld-templates.refresh'), icon: RefreshCw, variant: 'outline' as const, onclick: load },
		{ label: t('jsonld-templates.export'), icon: Download, variant: 'outline' as const, onclick: exportTemplates },
		...(canManageTemplates
			? [
					{ label: t('jsonld-templates.import'), icon: Upload, variant: 'outline' as const, onclick: handleImport },
					{ label: t('jsonld-templates.create_title'), icon: Plus, variant: 'default' as const, onclick: openCreateTemplate }
				]
			: [])
	]);

	function schemaTypeOf(template: JsonLdTemplate): string {
		const body = (template.template ?? {}) as Record<string, unknown>;
		return (body['@type'] as string) ?? '—';
	}

	function openViewTemplate(template: JsonLdTemplate) {
		viewTarget = template;
		showViewModal = true;
	}

	const templateColumns = [
		{
			key: 'content_type',
			label: t('jsonld-templates.content_type'),
			sortable: true,
			render: (_: any, row: JsonLdTemplate) => `
				<div class="flex items-center gap-3">
					<div class="h-10 w-10 rounded-lg bg-warning/15 flex items-center justify-center">
						<svg class="h-5 w-5 text-warning-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"></path></svg>
					</div>
					<div>
						<p class="font-medium text-foreground">${row.content_type}</p>
					</div>
				</div>
			`
		},
		{
			key: 'template',
			label: t('jsonld-templates.schema_type'),
			sortable: false,
			render: (_: any, row: JsonLdTemplate) =>
				`<span class="font-mono text-sm text-foreground">${schemaTypeOf(row)}</span>`
		},
		{
			key: 'is_active',
			label: t('jsonld-templates.status'),
			sortable: true,
			render: (value: boolean) =>
				`<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${value ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border'}">${value ? t('jsonld-templates.status_active') : t('jsonld-templates.status_inactive')}</span>`
		},
		{
			key: 'updated_at',
			label: t('jsonld-templates.last_updated'),
			sortable: true,
			render: (value: string) =>
				`<span class="text-sm text-muted">${new Date(value).toLocaleDateString()}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			componentSnippet: TemplateActionsCell
		}
	];

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await jsonLdApi.listTemplates();
			if (result.ok) {
				templates = result.data ?? [];
			} else {
				error = t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	function openCreateTemplate() {
		editingTemplate = null;
		templateForm = { content_type: '', is_active: true, content: '{\n  "@context": "https://schema.org",\n  "@type": "Article"\n}' };
		jsonError = '';
		showTemplateModal = true;
	}

	function openEditTemplate(template: JsonLdTemplate) {
		editingTemplate = template;
		templateForm = {
			content_type: template.content_type,
			is_active: template.is_active,
			content: JSON.stringify(template.template ?? {}, null, 2)
		};
		jsonError = '';
		showTemplateModal = true;
	}

	function validateJson(json: string): boolean {
		try {
			JSON.parse(json);
			jsonError = '';
			return true;
		} catch (e) {
			jsonError = (e as Error).message;
			return false;
		}
	}

	function handleContentInput(e: Event) {
		const target = e.target as HTMLTextAreaElement;
		templateForm.content = target.value;
		validateJson(target.value);
	}

	async function saveTemplate() {
		if (!validateJson(templateForm.content)) return;
		if (!templateForm.content_type.trim()) return;

		saving = true;
		try {
			const templateBody = JSON.parse(templateForm.content) as Record<string, unknown>;
			const result = await jsonLdApi.upsertTemplate({
				content_type: templateForm.content_type.trim(),
				template: templateBody,
				is_active: templateForm.is_active
			});
			if (result.ok) {
				success = editingTemplate
					? t('jsonld-templates.updated')
					: t('jsonld-templates.created');
				showTemplateModal = false;
				await load();
			} else {
				error = t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDelete(template: JsonLdTemplate) {
		deletingTemplate = template;
		showDeleteModal = true;
	}

	async function deleteTemplate() {
		if (!deletingTemplate) return;
		try {
			const result = await jsonLdApi.deleteTemplate(deletingTemplate.id);
			if (result.ok) {
				success = t('jsonld-templates.deleted');
				templates = templates.filter((t) => t.id !== deletingTemplate!.id);
				selectedIds.delete(deletingTemplate.id);
				selectedIds = new Set(selectedIds);
			} else {
				error = t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			showDeleteModal = false;
			deletingTemplate = null;
		}
	}

	async function bulkDelete() {
		if (selectedIds.size === 0) return;
		bulkDeleting = true;
		try {
			let failures = 0;
			for (const id of selectedIds) {
				const result = await jsonLdApi.deleteTemplate(id);
				if (!result.ok) failures++;
			}
			if (failures === 0) {
				success = t('jsonld-templates.deleted');
				selectedIds = new Set();
				await load();
			} else {
				error = t('common.error');
				await load();
			}
		} catch {
			error = t('common.error');
		} finally {
			bulkDeleting = false;
		}
	}

	function exportTemplates() {
		const data = JSON.stringify(templates, null, 2);
		const blob = new Blob([data], { type: 'application/json' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = 'jsonld-templates.json';
		a.click();
		URL.revokeObjectURL(url);
	}

	function handleImport() {
		const input = document.createElement('input');
		input.type = 'file';
		input.accept = '.json';
		input.onchange = async (e) => {
			const file = (e.target as HTMLInputElement).files?.[0];
			if (!file) return;
			try {
				const text = await file.text();
				const parsed = JSON.parse(text);
				// Accept either an array of {content_type, template, is_active}
				// or the raw backend row shape ({id, content_type, template, ...}).
				const list: any[] = Array.isArray(parsed) ? parsed : Object.values(parsed);
				let imported = 0;
				for (const item of list) {
					const contentType = item.content_type;
					const template = item.template;
					if (!contentType || !template) continue;
					const result = await jsonLdApi.upsertTemplate({
						content_type: String(contentType),
						template: template as Record<string, unknown>,
						is_active: item.is_active !== false
					});
					if (result.ok) imported++;
				}
				success = t('jsonld-templates.imported');
				await load();
			} catch {
				error = t('jsonld-templates.import_failed');
			}
		};
		input.click();
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>{t('jsonld.title')} — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title={t('jsonld.title')}
	description={t('jsonld.description')}
	actions={headerActions}
/>

<div class="space-y-6">
	{#if error}
		<div class="rounded-lg border border-danger bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	{#if success}
		<div class="rounded-lg border border-success bg-success-50 px-4 py-3 text-sm text-success-700">
			{success}
		</div>
	{/if}

	{#if loading}
		<p class="py-8 text-center text-sm text-muted">{t('common.loading')}</p>
	{:else}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-foreground">{t('jsonld-templates.templates')}</h3>
				<p class="text-sm text-muted">
					{templates.length} {t('jsonld-templates.templates_count')}
				</p>
			</CardHeader>
			<CardContent class="p-0">
				<DataTable
					columns={templateColumns}
					data={templates}
					pageSize={20}
					emptyMessage={t('jsonld-templates.empty')}
					searchable
					onsearch={() => load()}
					selectable
					bind:selectedIds
				>
					{#snippet bulkActions()}
						{#if canManageTemplates}
							<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
								<Trash2 class="h-3.5 w-3.5 mr-1" />
								{bulkDeleting ? t('jsonld-templates.deleting') : t('jsonld-templates.delete')} ({selectedIds.size})
							</Button>
						{/if}
					{/snippet}
				</DataTable>
			</CardContent>
		</Card>

		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-foreground">{t('jsonld-templates.preview')}</h3>
				<p class="text-sm text-muted">{t('jsonld-templates.example_json_ld_output_for_search_engine')}</p>
			</CardHeader>
			<CardContent>
				{#if templates.length > 0}
					<div class="rounded-lg border border bg-surface p-4">
						<pre class="overflow-x-auto text-xs text-foreground-alt"><code>{JSON.stringify(templates[0]?.template ?? {}, null, 2)}</code></pre>
					</div>
				{:else}
					<p class="text-sm text-muted">{t('jsonld-templates.no_templates_to_preview')}</p>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Template Edit Modal -->
<Modal bind:open={showTemplateModal} title={editingTemplate ? t('jsonld-templates.edit_title') : t('jsonld-templates.create_title')} size="xl">
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="template-content-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('jsonld-templates.content_type')}</label>
				<input
					id="template-content-type"
					type="text"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={templateForm.content_type}
					disabled={!!editingTemplate}
					placeholder="e.g., Article"
				/>
			</div>
			<div>
				<label for="template-is-active" class="mb-1 block text-sm font-medium text-foreground-alt">{t('jsonld-templates.status')}</label>
				<label class="inline-flex items-center gap-2 pt-2">
					<input
						id="template-is-active"
						type="checkbox"
						class="h-4 w-4 rounded border"
						bind:checked={templateForm.is_active}
					/>
					<span class="text-sm text-foreground-alt">{templateForm.is_active ? t('jsonld-templates.status_active') : t('jsonld-templates.status_inactive')}</span>
				</label>
			</div>
		</div>
		<div>
			<label for="template-content" class="mb-1 block text-sm font-medium text-foreground-alt">
				{t('jsonld-templates.content')}
				{#if jsonError}
					<span class="text-danger-600 ml-2">({t('jsonld-templates.invalid_json')}: {jsonError})</span>
				{/if}
			</label>
			<textarea
				id="template-content"
				class="w-full rounded-lg border border bg-surface px-3 py-2 font-mono text-xs placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2 {jsonError ? 'border-danger-300' : ''}"
				rows="20"
				spellcheck="false"
				value={templateForm.content}
				oninput={handleContentInput}
				placeholder={t('jsonld-templates.placeholder')}
			></textarea>
		</div>
	</div>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showTemplateModal = false)}>{t('jsonld-templates.cancel')}</Button>
		<Button onclick={saveTemplate} disabled={saving || !templateForm.content_type.trim() || !!jsonError}>
			{saving ? t('jsonld-templates.saving') : editingTemplate ? t('jsonld-templates.save') : t('jsonld-templates.create_title')}
		</Button>
	</div>
</Modal>

<!-- View Modal -->
<Modal bind:open={showViewModal} title={t('jsonld-templates.view_title')} size="lg">
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jsonld-templates.content_type')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.content_type}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jsonld-templates.schema_type')}</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{schemaTypeOf(viewTarget)}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jsonld-templates.status')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.is_active ? t('jsonld-templates.status_active') : t('jsonld-templates.status_inactive')}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jsonld-templates.last_updated')}</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.updated_at).toLocaleDateString()}</p>
				</div>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('jsonld-templates.content')}</p>
				<pre class="mt-1 overflow-x-auto rounded-lg border p-3 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"><code>{JSON.stringify(viewTarget.template ?? {}, null, 2)}</code></pre>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('jsonld-templates.cancel')}</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title={t('jsonld-templates.delete_title')} size="sm">
	<p class="text-sm text-foreground-alt">
		{t('jsonld-templates.delete_confirm')}
	</p>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => (showDeleteModal = false)}>{t('jsonld-templates.cancel')}</Button>
		<Button variant="destructive" onclick={deleteTemplate}>{t('jsonld-templates.delete')}</Button>
	</div>
</Modal>

<!-- TemplateActionsCell -->
{#snippet TemplateActionsCell({ row }: { value: any; row: JsonLdTemplate })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openViewTemplate(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageTemplates}
			<Button variant="ghost" size="sm" onclick={() => openEditTemplate(row)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
