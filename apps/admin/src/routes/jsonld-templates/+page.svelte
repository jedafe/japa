<script lang="ts">
	import { onMount } from 'svelte';
	import { jsonLdApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import PageHeader from '$lib/components/layout/page-header.svelte';
	import { Card, CardContent, CardHeader } from '$lib/components/ui/card';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Modal } from '$lib/components/ui/modal';
	import { FileJson, Plus, RefreshCw, Pencil, Trash2, Download, Upload } from 'lucide-svelte';

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

	// Modal state
	let showTemplateModal = $state(false);
	let editingTemplate = $state<JsonLdTemplate | null>(null);
	let templateForm = $state({ name: '', type: 'Article', content: '{}' });
	let saving = $state(false);
	let jsonError = $state('');

	// Delete confirmation
	let showDeleteModal = $state(false);
	let deletingTemplate = $state<JsonLdTemplate | null>(null);

	const templateColumns = [
		{
			key: 'name',
			label: 'Template',
			sortable: true,
			render: (_: any, row: any) => `
				<div class="flex items-center gap-3">
					<div class="h-10 w-10 rounded-lg bg-warning-100 flex items-center justify-center">
						<svg class="h-5 w-5 text-warning-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"></path></svg>
					</div>
					<div>
						<p class="font-medium text-gray-900">${row.name}</p>
						<p class="text-sm text-gray-500">${row.type}</p>
					</div>
				</div>
			`
		},
		{
			key: 'type',
			label: 'Schema Type',
			sortable: true,
			render: (value: string) => `<span class="font-mono text-sm text-gray-900">${value}</span>`
		},
		{
			key: 'is_active',
			label: 'Status',
			sortable: true,
			render: (value: boolean) => `<span class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold ${value ? 'bg-success-100 text-success-800 border-success-200' : 'bg-gray-100 text-gray-800 border-gray-200'}">${value ? 'Active' : 'Inactive'}</span>`
		},
		{
			key: 'updated_at',
			label: 'Last Updated',
			sortable: true,
			render: (value: string) => `<span class="text-sm text-gray-500">${new Date(value).toLocaleDateString()}</span>`
		},
		{
			key: 'actions',
			label: '',
			class: 'w-24',
			render: (_: any, row: any) => `
				<div class="flex items-center gap-1">
					<button onclick="window.__editTemplate('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-gray-700 hover:bg-gray-100" title="Edit">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path></svg>
					</button>
					<button onclick="window.__deleteTemplate('${row.id}')" class="inline-flex items-center justify-center rounded-md px-2 py-1.5 text-sm font-medium text-danger-700 hover:bg-danger-50" title="Delete">
						<svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
					</button>
				</div>
			`
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

	// Expose functions for DataTable inline actions
	if (typeof window !== 'undefined') {
		(window as any).__editTemplate = openEditTemplate;
		(window as any).__deleteTemplate = confirmDelete;
	}

	onMount(load);
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
		<div class="rounded-lg border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
			{error}
		</div>
	{/if}

	{#if success}
		<div class="rounded-lg border border-success-200 bg-success-50 px-4 py-3 text-sm text-success-700">
			{success}
		</div>
	{/if}

	{#if loading}
		<p class="py-8 text-center text-sm text-gray-500">{t('common.loading')}</p>
	{:else}
		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-gray-900">Templates</h3>
				<p class="text-sm text-gray-500">
					{templates.length} template{templates.length !== 1 ? 's' : ''} configured
				</p>
			</CardHeader>
			<CardContent class="p-0">
				<DataTable
					columns={templateColumns}
					data={templates}
					pageSize={20}
					emptyMessage="No JSON-LD templates configured"
				/>
			</CardContent>
		</Card>

		<Card>
			<CardHeader>
				<h3 class="text-lg font-semibold text-gray-900">Preview</h3>
				<p class="text-sm text-gray-500">Example JSON-LD output for search engines</p>
			</CardHeader>
			<CardContent>
				{#if templates.length > 0}
					<div class="rounded-lg border border-gray-200 bg-gray-50 p-4">
						<pre class="overflow-x-auto text-xs text-gray-700"><code>{JSON.stringify(templates[0]?.content || {}, null, 2)}</code></pre>
					</div>
				{:else}
					<p class="text-sm text-gray-500">No templates to preview</p>
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
				<label for="template-name" class="mb-1 block text-sm font-medium text-gray-700">Name</label>
				<input
					id="template-name"
					type="text"
					class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
					bind:value={templateForm.name}
					placeholder="e.g., Article Template"
				/>
			</div>
			<div>
				<label for="template-type" class="mb-1 block text-sm font-medium text-gray-700">Schema Type</label>
				<select
					id="template-type"
					class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
					bind:value={templateForm.type}
				>
					<option value="Article">Article</option>
					<option value="Organization">Organization</option>
					<option value="Person">Person</option>
					<option value="Product">Product</option>
					<option value="Event">Event</option>
					<option value="FAQPage">FAQ Page</option>
					<option value="HowTo">How-To</option>
					<option value="LocalBusiness">Local Business</option>
					<option value="Course">Course</option>
					<option value="JobPosting">Job Posting</option>
				</select>
			</div>
		</div>
		<div>
			<label for="template-content" class="mb-1 block text-sm font-medium text-gray-700">
				JSON-LD Content
				{#if jsonError}
					<span class="text-danger-600 ml-2">({jsonError})</span>
				{/if}
			</label>
			<textarea
				id="template-content"
				class="w-full rounded-lg border border-gray-300 bg-white px-3 py-2 font-mono text-xs placeholder:text-gray-500 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2 {jsonError ? 'border-danger-300' : ''}"
				rows="20"
				spellcheck="false"
				value={templateForm.content}
				oninput={handleContentInput}
				placeholder={'{"@context": "https://schema.org", "@type": "Article", ...}'}
			></textarea>
		</div>
	</div>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => showTemplateModal = false}>Cancel</Button>
		<Button onclick={saveTemplate} disabled={saving || !templateForm.name || !!jsonError}>
			{saving ? 'Saving...' : editingTemplate ? 'Save Changes' : 'Create Template'}
		</Button>
	</div>
</Modal>

<!-- Delete Confirmation Modal -->
<Modal bind:open={showDeleteModal} title="Delete Template" size="sm">
	<p class="text-sm text-gray-600">
		Are you sure you want to delete <strong>{deletingTemplate?.name}</strong>? This action cannot be undone.
	</p>
	<div class="flex justify-end gap-3 pt-4">
		<Button variant="outline" onclick={() => showDeleteModal = false}>Cancel</Button>
		<Button variant="destructive" onclick={deleteTemplate}>Delete</Button>
	</div>
</Modal>
