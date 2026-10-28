<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /jsonld-templates — migrated to the EntityPage scaffold.
	//
	// The form has a JSON content editor with live JSON-validation feedback, so
	// it is rendered via the `formSnippet` escape hatch (a `fields` array is still
	// passed so EntityPage seeds the edit-form value). Export/import are kept as
	// page-local header actions rendered above the scaffold.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { Button } from '$lib/components/ui/button';
	import { api, jsonLdApi, contentAdminApi, type JsonLdTemplate } from '$lib/api';
	import { JsonValueEditor } from '@japaconnect/ui';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Trash2, Download, Upload } from 'lucide-svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';

	type Template = JsonLdTemplate & { content: string };

	const canManageTemplates = $derived(hasPermission('content.jsonld.manage'));

	const DEFAULT_TEMPLATE = '{\n  "@context": "https://schema.org",\n  "@type": "Article"\n}';

	// Content types are fetched from GET /content-types so the dropdown stays in
	// sync with the backend's canonical list (Rule 16) — no free-text input that
	// could allow typos or invalid content_type keys.
	let contentTypeOptions = $state<Array<{ value: string; label: string }>>([]);

	$effect(() => {
		void (async () => {
			const res = await contentAdminApi.listContentTypes();
			if (res.ok && res.data) {
				contentTypeOptions = res.data.map((ct) => ({
					value: ct.value,
					label: t(`content.type_${ct.value}`, ct.label)
				}));
			}
		})();
	});

	// Normalize an API row: stringify `template` into a `content` string so the
	// formSnippet's JsonValueEditor can bind to `value.content` and EntityPage's edit
	// seeding (row[field.key]) picks it up.
	function mapTemplate(raw: JsonLdTemplate): Template {
		return { ...raw, content: JSON.stringify(raw.template ?? {}, null, 2) };
	}

	function schemaTypeOf(row: JsonLdTemplate): string {
		const body = (row.template ?? {}) as Record<string, unknown>;
		return (body['@type'] as string) ?? '—';
	}

	// Live JSON validation helper used inside the formSnippet.
	function validateJson(content: string | undefined): string | null {
		const text = String(content ?? '').trim();
		if (!text) return null; // empty is fine — caught by create/update
		try {
			JSON.parse(text);
			return null;
		} catch (e) {
			return (e as Error).message;
		}
	}

	// Build the upsert body from the form value: parse the JSON content string
	// back into the `template` object the API expects.
	function toUpsertBody(formValue: Record<string, any>): { ok: true; body: { content_type: string; template: Record<string, unknown>; is_active: boolean } } | { ok: false; error: string } {
		const content_type = String(formValue.content_type ?? '').trim();
		if (!content_type) {
			return { ok: false, error: t('jsonld-templates.content_type', 'Content Type') + ' required' };
		}
		try {
			const template = JSON.parse(String(formValue.content ?? '{}')) as Record<string, unknown>;
			return { ok: true, body: { content_type, template, is_active: !!formValue.is_active } };
		} catch (e) {
			return { ok: false, error: t('jsonld-templates.invalid_json', 'Invalid JSON') + ': ' + (e as Error).message };
		}
	}

	// ── Bulk delete ───── EntityPage auto-reloads + clears selection after a bulk action handler completes.
	let entityController = $state<EntityPageController | undefined>(undefined);
	let pageError = $state('');
	let bulkDeleting = $state(false);

	async function bulkDelete(ids: Set<string>) {
		const idArr = Array.from(ids);
		if (idArr.length === 0) return;
		if (bulkDeleting) return; // in-flight guard
		bulkDeleting = true;
		pageError = '';
		try {
			await Promise.all(idArr.map((id) => jsonLdApi.deleteTemplate(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to delete templates');
		} finally {
			bulkDeleting = false;
		}
	}

	// ── Export / Import (page-local header actions) ──────────────────────────
	let importing = $state(false);

	async function exportTemplates() {
		try {
			const result = await jsonLdApi.listTemplates();
			if (!result.ok) {
				pageError = result.error ?? t('common.error', 'Failed to export templates');
				return;
			}
			const data = JSON.stringify(result.data ?? [], null, 2);
			const blob = new Blob([data], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = 'jsonld-templates.json';
			a.click();
			URL.revokeObjectURL(url);
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('common.error', 'Failed to export templates');
		}
	}

	function handleImport() {
		const input = document.createElement('input');
		input.type = 'file';
		input.accept = '.json';
		input.onchange = async (e) => {
			const file = (e.target as HTMLInputElement).files?.[0];
			if (!file) return;
			importing = true;
			pageError = '';
			try {
				const text = await file.text();
				const parsed = JSON.parse(text);
				// Accept either an array of {content_type, template, is_active}
				// or the raw backend row shape ({id, content_type, template, ...}).
				const list: any[] = Array.isArray(parsed) ? parsed : Object.values(parsed);
				for (const item of list) {
					const contentType = item.content_type;
					const template = item.template;
					if (!contentType || !template) continue;
					await jsonLdApi.upsertTemplate({
						content_type: String(contentType),
						template: template as Record<string, unknown>,
						is_active: item.is_active !== false
					});
				}
				entityController?.reload();
			} catch {
				pageError = t('jsonld-templates.import_failed', 'Failed to import templates. Invalid JSON file.');
			} finally {
				importing = false;
			}
		};
		input.click();
	}

	// `fields` drives edit-form seeding; the actual inputs are rendered by the
	// `formSnippet` below (structured JSON editor with validation feedback).
	const fields: FieldDef[] = [
		{ key: 'content_type', label: t('jsonld-templates.content_type', 'Content Type'), type: 'text', required: true },
		{ key: 'is_active', label: t('jsonld-templates.status', 'Status'), type: 'checkbox' },
		{ key: 'content', label: t('jsonld-templates.content', 'JSON-LD Content'), type: 'json' }
	];

	const config: EntityPageConfig = {
		title: t('jsonld.title', 'JSON-LD Templates'),
		description: t('jsonld.description', 'Manage structured data templates for content SEO.'),
		createLabel: t('jsonld-templates.create_title', 'Create Template'),
		createPermission: 'content.jsonld.manage',
		pageSize: 25,
		columns: [
			{ key: 'content_type', label: t('jsonld-templates.content_type', 'Content Type'), sortable: true, componentSnippet: ContentTypeCell },
			{ key: 'template', label: t('jsonld-templates.schema_type', 'Schema Type'), componentSnippet: SchemaTypeCell },
			{ key: 'is_active', label: t('jsonld-templates.status', 'Status'), sortable: true, componentSnippet: StatusCell },
			{
				key: 'updated_at',
				label: t('jsonld-templates.last_updated', 'Last Updated'),
				sortable: true,
				render: (v: string) => new Date(v).toLocaleDateString()
			}
		],
		filters: [
			{
				key: 'is_active',
				label: t('jsonld-templates.status', 'Status'),
				options: [
					{ value: 'true', label: t('jsonld-templates.status_active', 'Active') },
					{ value: 'false', label: t('jsonld-templates.status_inactive', 'Inactive') }
				]
			}
		],
		bulkActions: [
			{
				label: t('common.deleteSelected', 'Delete Selected'),
				icon: Trash2,
				variant: 'destructive',
				onclick: bulkDelete
			}
		],
		fields,
		load: async ({ page, pageSize, q, filter }) => {
			const result = await api<JsonLdTemplate[]>('/content/jsonld-templates?page_size=1000');
			if (!result.ok) {
				throw new Error(result.error ?? t('common.error', 'Failed to load templates'));
			}
			let data = (result.data ?? []) as JsonLdTemplate[];
			// The list endpoint returns the full set, so filter client-side.
			if (filter?.is_active) {
				data = data.filter((x) => String(x.is_active) === filter.is_active);
			}
			const lc = (q ?? '').toLowerCase();
			if (lc) {
				data = data.filter((x) =>
					[x.content_type, schemaTypeOf(x)].some((v) =>
						String(v ?? '').toLowerCase().includes(lc)
					)
				);
			}
			const mapped = data.map(mapTemplate);
			const size = pageSize || 25;
			const offset = (page || 0) * size;
			const total = mapped.length;
			const totalPages = Math.max(1, Math.ceil(total / size));
			return {
				data: mapped.slice(offset, offset + size),
				total,
				totalPages
			};
		},
		create: async (body) => {
			const parsed = toUpsertBody(body);
			if (!parsed.ok) return parsed;
			const res = await jsonLdApi.upsertTemplate(parsed.body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (_id, body) => {
			// Upsert is keyed by content_type (immutable in the form when editing),
			// so update reuses the same path as create.
			const parsed = toUpsertBody(body);
			if (!parsed.ok) return parsed;
			const res = await jsonLdApi.upsertTemplate(parsed.body);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await jsonLdApi.deleteTemplate(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		viewSnippet: TemplateView,
		formSnippet: TemplateForm
	};
</script>

<svelte:head>
	<title>{t('jsonld.title', 'JSON-LD Templates')} — {siteName()} Admin</title>
</svelte:head>

<!-- Page-local export/import header actions (rendered above the scaffold) -->
<div class="mb-4 flex items-center gap-2">
	<Button variant="outline" size="sm" onclick={exportTemplates}>
		<Download class="me-1 h-3.5 w-3.5" />
		{t('jsonld-templates.export', 'Export')}
	</Button>
	{#if canManageTemplates}
		<Button variant="outline" size="sm" onclick={handleImport} disabled={importing}>
			<Upload class="me-1 h-3.5 w-3.5" />
			{importing ? t('common.loading', 'Loading...') : t('jsonld-templates.import', 'Import')}
		</Button>
	{/if}
</div>

{#if pageError}
	<div
		class="mb-4 rounded-lg p-3 text-sm"
		style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)"
	>
		{pageError}
	</div>
{/if}

<EntityPage {config} bind:controller={entityController} />

<!-- ── Snippets ─────────────────────────────────────────────────────────── -->

{#snippet ContentTypeCell({ row }: { value: any; row: Template })}
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-lg" style="background: color-mix(in srgb, var(--color-warning) 15%, transparent)">
			<svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24" style="color: var(--color-warning)">
				<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"></path>
			</svg>
		</div>
		<p class="font-medium" style="color: var(--color-foreground)">{row.content_type}</p>
	</div>
{/snippet}

{#snippet SchemaTypeCell({ row }: { value: any; row: Template })}
	<span class="font-mono text-sm" style="color: var(--color-foreground)">{schemaTypeOf(row)}</span>
{/snippet}

{#snippet StatusCell({ value }: { value: boolean; row: Template })}
	<span
		class="inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold {value
			? 'bg-success/15 text-success border-success'
			: 'bg-surface text-foreground border'}"
	>
		{value ? t('jsonld-templates.status_active', 'Active') : t('jsonld-templates.status_inactive', 'Inactive')}
	</span>
{/snippet}

<!-- View slide-over body (mirrors the legacy view modal) -->
{#snippet TemplateView({ row }: { row: Template })}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('jsonld-templates.content_type', 'Content Type')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.content_type}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('jsonld-templates.schema_type', 'Schema Type')}
				</p>
				<p class="mt-1 text-sm font-mono" style="color: var(--color-foreground)">{schemaTypeOf(row)}</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('jsonld-templates.status', 'Status')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{row.is_active ? t('jsonld-templates.status_active', 'Active') : t('jsonld-templates.status_inactive', 'Inactive')}
				</p>
			</div>
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
					{t('jsonld-templates.last_updated', 'Last Updated')}
				</p>
				<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
					{new Date(row.updated_at).toLocaleDateString()}
				</p>
			</div>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">
				{t('jsonld-templates.content', 'JSON-LD Content')}
			</p>
			<pre class="mt-1 overflow-x-auto rounded-lg border p-3 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"><code>{JSON.stringify(row.template ?? {}, null, 2)}</code></pre>
		</div>
	</div>
{/snippet}

<!-- Create/Edit form body (structured JSON editor with validation feedback) -->
{#snippet TemplateForm({ value, editing }: { value: Record<string, any>; editing: any | null })}
	<!-- Seed "content" once: edit from the row's template, create from the default skeleton. -->
	{@const _seed = value.content ??= editing ? JSON.stringify(editing?.template ?? {}, null, 2) : DEFAULT_TEMPLATE}
	{@const jsonError = validateJson(value.content)}
	<div class="space-y-4">
		<div class="grid grid-cols-2 gap-4">
			<div>
				<label for="template-content-type" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground-alt)">
					{t('jsonld-templates.content_type', 'Content Type')}
				</label>
				<AutocompleteSelect
					id="template-content-type"
					bind:value={value.content_type}
					disabled={!!editing}
					placeholder={t('jsonld-templates.select_content_type', 'Select a content type')}
					options={contentTypeOptions}
					class="w-full"
				/>
			</div>
			<div>
				<label for="template-is-active" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground-alt)">
					{t('jsonld-templates.status', 'Status')}
				</label>
				<label class="inline-flex items-center gap-2 pt-2">
					<input id="template-is-active" type="checkbox" class="h-4 w-4 rounded border" bind:checked={value.is_active} />
					<span class="text-sm" style="color: var(--color-foreground-alt)">
						{value.is_active ? t('jsonld-templates.status_active', 'Active') : t('jsonld-templates.status_inactive', 'Inactive')}
					</span>
				</label>
			</div>
		</div>
		<div>
			<label for="template-content" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground-alt)">
				{t('jsonld-templates.content', 'JSON-LD Content')}
				{#if jsonError}
					<span class="ms-2" style="color: var(--color-danger)">
						({t('jsonld-templates.invalid_json', 'Invalid JSON')}: {jsonError})
					</span>
				{/if}
			</label>
			<JsonValueEditor
				id="template-content"
				bind:value={value.content}
				placeholder={t('jsonld-templates.placeholder', '{"@context": "https://schema.org", "@type": "Article", ...}')}
			/>
		</div>
	</div>
{/snippet}
