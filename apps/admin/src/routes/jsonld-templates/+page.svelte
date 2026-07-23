<script lang="ts">
	import { jsonLdApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Select } from '$lib/components/ui/select';
	import { Modal } from '$lib/components/ui/modal';
	import { FileJson, Plus, RefreshCw, Pencil, Trash2, Download, Upload, Eye } from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';

	type JsonLdTemplate = {
		id: string;
		name: string;
		type: string;
		content: Record<string, unknown>;
		is_active: boolean;
		created_at: string;
		updated_at: string;
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
	let templateForm = $state({ name: '', type: 'Article', content: '{}' });
	let saving = $state(false);
	let jsonError = $state('');

	// View modal
	let showViewModal = $state(false);
	let viewTarget = $state<JsonLdTemplate | null>(null);

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deletingTemplate = $state<JsonLdTemplate | null>(null);

	const canManageTemplates = $derived(hasPermission("content.jsonld.manage"));

	function openViewTemplate(template: JsonLdTemplate) {
		viewTarget = template;
		showViewModal = true;
	}

	const templateColumns = [
		{
			key: 'name',
			label: 'Template',
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-3">
					<div class="h-10 w-10 rounded-lg bg-warning/15 flex items-center justify-center">
						<svg class="h-5 w-5 text-warning-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"></path></svg>
					</div>
					<div>
						<p class="font-medium text-foreground">${row.name}</p>
						<p class="text-sm text-muted">${row.type}</p>
					</div>
				</div>
			`
		},
		{
			key: 'type',
			label: 'Schema Type',
			sortable: true,
			render: (value: string) => `<span class="font-mono text-sm text-foreground">${value}</span>`
		},
		{
			key: 'is_active',
			label: 'Status',
			sortable: true,
			render: (value: boolean) => `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${value ? 'bg-success/15 text-success border-success' : 'bg-surface text-foreground border'}">${value ? 'Active' : 'Inactive'}</span>`
		},
		{
			key: 'updated_at',
			label: 'Last Updated',
			sortable: true,
			render: (value: string) => `<span class="text-sm text-muted">${new Date(value).toLocaleDateString()}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			component: TemplateActionsCell
		}
	];

	async function load() {
		loading = true;
		error = '';
		try {
			const result = await jsonLdApi.getTemplates();
			if (result.ok) {
				const data = result.data as Record<string, unknown>;
				// Transform the object-based templates into array format
				const templateArray: JsonLdTemplate[] = [];
				for (const [key, value] of Object.entries(data)) {
					const template = value as any;
					templateArray.push({
						id: key,
						name: template.name || key,
						type: template['@type'] || 'Article',
						content: template,
						is_active: template.is_active !== false,
						created_at: template.created_at || new Date().toISOString(),
						updated_at: template.updated_at || new Date().toISOString()
					});
				}
				templates = templateArray;
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
		templateForm = { name: '', type: 'Article', content: '{}' };
		jsonError = '';
		showTemplateModal = true;
	}

	function openEditTemplate(templateId: string) {
		const template = templates.find((t) => t.id === templateId);
		if (!template) return;
		editingTemplate = template;
		templateForm = {
			name: template.name,
			type: template.type,
			content: JSON.stringify(template.content, null, 2)
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

		saving = true;
		try {
			const content = JSON.parse(templateForm.content);
			const templatesData: Record<string, unknown> = {};

			// Build the templates object
			for (const t of templates) {
				templatesData[t.id] = t.content;
			}

			if (editingTemplate) {
				templatesData[editingTemplate.id] = {
					...content,
					name: templateForm.name,
					'@type': templateForm.type,
					updated_at: new Date().toISOString()
				};
			} else {
				const id = templateForm.name.toLowerCase().replace(/\s+/g, '-');
				templatesData[id] = {
					...content,
					name: templateForm.name,
					'@type': templateForm.type,
					is_active: true,
					created_at: new Date().toISOString(),
					updated_at: new Date().toISOString()
				};
			}

			const result = await jsonLdApi.updateTemplates(templatesData);
			if (result.ok) {
				success = editingTemplate ? 'Template updated successfully' : 'Template created successfully';
				showTemplateModal = false;
				load();
			} else {
				error = t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			saving = false;
		}
	}

	function confirmDelete(templateId: string) {
		const template = templates.find((t) => t.id === templateId);
		if (!template) return;
		deletingTemplate = template;
		showDeleteModal = true;
	}

	async function deleteTemplate() {
		if (!deletingTemplate) return;
		try {
			const templatesData: Record<string, unknown> = {};
			for (const t of templates) {
				if (t.id !== deletingTemplate.id) {
					templatesData[t.id] = t.content;
				}
			}

			const result = await jsonLdApi.updateTemplates(templatesData);
			if (result.ok) {
				success = 'Template deleted successfully';
				templates = templates.filter((t) => t.id !== deletingTemplate!.id);
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
			const templatesData: Record<string, unknown> = {};
			for (const t of templates) {
				if (!selectedIds.has(t.id)) {
					templatesData[t.id] = t.content;
				}
			}

			const result = await jsonLdApi.updateTemplates(templatesData);
			if (result.ok) {
				success = `${selectedIds.size} template${selectedIds.size > 1 ? 's' : ''} deleted successfully`;
				selectedIds = new Set();
				load();
			} else {
				error = t('common.error');
			}
		} catch {
			error = t('common.error');
		} finally {
			bulkDeleting = false;
		}
	}

	function exportTemplates() {
		const data = JSON.stringify(
			templates.reduce((acc, t) => {
				acc[t.id] = t.content;
				return acc;
			}, {} as Record<string, unknown>),
			null,
			2
		);
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
				const data = JSON.parse(text);
				const result = await jsonLdApi.updateTemplates(data);
				if (result.ok) {
					success = 'Templates imported successfully';
					load();
				}
			} catch {
				error = 'Failed to import templates. Invalid JSON file.';
			}
		};
		input.click();
	}

  $effect(() => {
    load();
  });
</script>

<svelte:head>
	<title>JSON-LD Templates — JapaConnect Admin</title>
</svelte:head>

<PageHeader
	title="JSON-LD Templates"
	description="Manage structured data templates for SEO"
	actions={[
		{ label: 'Refresh', icon: RefreshCw, variant: 'outline', onclick: load },
		{ label: 'Import', icon: Upload, variant: 'outline', onclick: handleImport },
		{ label: 'Export', icon: Download, variant: 'outline', onclick: exportTemplates },
		{ label: 'Add Template', icon: Plus, variant: 'default', onclick: openCreateTemplate }
	]}
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
					{templates.length} template{templates.length !== 1 ? 's' : ''} configured
				</p>
			</CardHeader>
			<CardContent class="p-0">
				<!-- wire the search box to re-fetch templates; API ignores q today (see report) -->
				<DataTable
					columns={templateColumns}
					data={templates}
					pageSize={20}
					emptyMessage="No JSON-LD templates configured"
					searchable
					onsearch={() => load()}
					selectable
					bind:selectedIds
				>
					{#snippet bulkActions()}
						<Button variant="destructive" size="sm" onclick={bulkDelete} disabled={bulkDeleting || selectedIds.size === 0}>
							<Trash2 class="h-3.5 w-3.5 mr-1" />
							{bulkDeleting ? 'Deleting...' : 'Delete Selected'} ({selectedIds.size})
						</Button>
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
						<pre class="overflow-x-auto text-xs text-foreground-alt"><code>{JSON.stringify(templates[0]?.content || {}, null, 2)}</code></pre>
					</div>
				{:else}
					<p class="text-sm text-muted">{t('jsonld-templates.no_templates_to_preview')}</p>
				{/if}
			</CardContent>
		</Card>
	{/if}
</div>

<!-- Template Edit Modal -->
<Modal bind:open={showTemplateModal} title={editingTemplate ? 'Edit Template' : 'Create Template'} size="xl">
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="template-name" class="mb-1 block text-sm font-medium text-foreground-alt">{t('jsonld-templates.name')}</label>
				<input
					id="template-name"
					type="text"
					class="w-full rounded-lg border border bg-surface px-3 py-2 text-sm placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2"
					bind:value={templateForm.name}
					placeholder="e.g., Article Template"
				/>
			</div>
			<div>
			<label for="template-type" class="mb-1 block text-sm font-medium text-foreground-alt">{t('jsonld-templates.schema_type')}</label>
			<Select
				options={[
					{ value: 'Article', label: 'Article' },
					{ value: 'Organization', label: 'Organization' },
					{ value: 'Person', label: 'Person' },
					{ value: 'Product', label: 'Product' },
					{ value: 'Event', label: 'Event' },
					{ value: 'FAQPage', label: 'FAQ Page' },
					{ value: 'HowTo', label: 'How-To' },
					{ value: 'LocalBusiness', label: 'Local Business' },
					{ value: 'Course', label: 'Course' },
					{ value: 'JobPosting', label: 'Job Posting' }
				]}
				bind:value={templateForm.type}
			/>
			</div>
		</div>
		<div>
			<label for="template-content" class="mb-1 block text-sm font-medium text-foreground-alt">
				JSON-LD Content
				{#if jsonError}
					<span class="text-danger-600 ml-2">({jsonError})</span>
				{/if}
			</label>
			<textarea
				id="template-content"
				class="w-full rounded-lg border border bg-surface px-3 py-2 font-mono text-xs placeholder:text-muted focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2 {jsonError ? 'border-danger-300' : ''}"
				rows="20"
				spellcheck="false"
				value={templateForm.content}
				oninput={handleContentInput}
				placeholder={'{"@context": "https://schema.org", "@type": "Article", ...}'}
			></textarea>
		</div>
	</div>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => showTemplateModal = false}>{t('jsonld-templates.cancel')}</Button>
		<Button onclick={saveTemplate} disabled={saving || !templateForm.name || !!jsonError}>
			{saving ? 'Saving...' : editingTemplate ? 'Save Changes' : 'Create Template'}
		</Button>
	</div>
</Modal>

<!-- View Modal -->
<Modal bind:open={showViewModal} title="Template Details" size="lg">
	{#if viewTarget}
		<div class="space-y-4">
			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Name</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.name}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Schema Type</p>
					<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{viewTarget.type}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Status</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{viewTarget.is_active ? 'Active' : 'Inactive'}</p>
				</div>
				<div>
					<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Last Updated</p>
					<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{new Date(viewTarget.updated_at).toLocaleDateString()}</p>
				</div>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">Content</p>
				<pre class="mt-1 overflow-x-auto rounded-lg border p-3 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"><code>{JSON.stringify(viewTarget.content, null, 2)}</code></pre>
			</div>
		</div>
	{/if}
	<div class="mt-6 flex justify-end">
		<Button variant="outline" onclick={() => (showViewModal = false)}>{t('jsonld-templates.close')}</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title="Delete Template" size="sm">
	<p class="text-sm text-foreground-alt">
		Are you sure you want to delete <strong>{deletingTemplate?.name}</strong>? This action cannot be undone.
	</p>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => showDeleteModal = false}>{t('jsonld-templates.cancel')}</Button>
		<Button variant="destructive" onclick={deleteTemplate}>{t('jsonld-templates.delete')}</Button>
	</div>
</Modal>

<!-- TemplateActionsCell -->
{#snippet TemplateActionsCell({ value, row }: { value: any; row: JsonLdTemplate })}
	<div class="flex items-center gap-1">
		<Button variant="ghost" size="sm" onclick={() => openViewTemplate(row)}>
			<Eye class="h-3.5 w-3.5" />
		</Button>
		{#if canManageTemplates}
			<Button variant="ghost" size="sm" onclick={() => openEditTemplate(row.id)}>
				<Pencil class="h-3.5 w-3.5" />
			</Button>
			<Button variant="ghost" size="sm" onclick={() => confirmDelete(row.id)} style="color: var(--color-danger)">
				<Trash2 class="h-3.5 w-3.5" />
			</Button>
		{/if}
	</div>
{/snippet}
