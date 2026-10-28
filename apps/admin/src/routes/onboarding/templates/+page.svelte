<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	// /onboarding/templates — Onboarding template CRUD (Section 09).
	// Templates are configurable onboarding step sequences stored as jsonb.
	// The backend exposes GET/POST/PATCH/DELETE under /admin/onboarding/templates
	// (RBAC: admin.settings for mutations, admin.settings.view for reads).
	// This page uses the EntityPage scaffold with a formSnippet for the steps
	// jsonb editor (a textarea accepting a JSON array), since EntityForm does
	// not have a native JSON field type.
	import { EntityPage, type EntityPageConfig, type EntityPageController, type FieldDef } from '$lib/components/entity';
	import { api, onboardingAdminApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Trash2 } from 'lucide-svelte';

	type OnboardingTemplate = {
		id: string;
		name: string;
		description: string | null;
		steps: unknown[] | string;
		is_active: boolean;
		is_system: boolean;
		created_at?: string;
		updated_at?: string;
	};

	const canManage = $derived(hasPermission('admin.settings'));

	// `fields` drives EntityForm seeding for the edit form. The `steps` field
	// is rendered by `formSnippet` below (a JSON textarea), but it MUST appear
	// here so EntityPage seeds value.steps from the selected row.
	const fields: FieldDef[] = [
		{ key: 'name', label: t('onboarding.templates.name', 'Name'), type: 'text', help: t('onboarding.templates.name_hint', 'The name shown in admin lists and on the site where public.'), required: true, placeholder: 'Default 7-step flow' },
		{ key: 'description', label: t('onboarding.templates.description', 'Description'), type: 'textarea', help: t('onboarding.templates.description_hint', 'Shown in detail views — keep it concise.'), placeholder: 'Describe what this template configures…' },
		{ key: 'steps', label: t('onboarding.templates.steps', 'Steps (JSON)'), type: 'textarea', help: t('onboarding.templates.steps_hint', 'The ordered steps — one per line.') },
		{ key: 'is_active', label: t('onboarding.templates.active', 'Active'), type: 'checkbox', help: t('onboarding.templates.is_active_hint', 'Inactive records are hidden or disabled on the site.') }
	];

	// ── Bulk delete ───── EntityPage auto-reloads + clears selection after a
	// bulk action handler completes.
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
			await Promise.all(idArr.map((id) => onboardingAdminApi.deleteTemplate(id)));
		} catch (e) {
			pageError = e instanceof Error ? e.message : t('onboarding.templates.delete_error', 'Failed to delete templates');
		} finally {
			bulkDeleting = false;
		}
	}

	// Serialize steps (array | string | null) into a pretty-printed JSON string
	// for the textarea. Falls back to '[]' when empty.
	function stepsToText(v: unknown): string {
		if (v == null) return '[]';
		if (typeof v === 'string') {
			// Already a string — try to pretty-print if it parses, else return as-is.
			try {
				return JSON.stringify(JSON.parse(v), null, 2);
			} catch {
				return v;
			}
		}
		try {
			return JSON.stringify(v, null, 2);
		} catch {
			return '[]';
		}
	}

	// Parse the textarea content back into a JSON array for the API.
	function parseSteps(text: string): unknown {
		try {
			const parsed = JSON.parse(text);
			return Array.isArray(parsed) ? parsed : parsed;
		} catch {
			// If the user typed invalid JSON, send the raw string — the backend
			// will marshal it again and surface a clear error if it's invalid.
			return text;
		}
	}

	const baseConfig: EntityPageConfig = {
		title: t('onboarding.templates.title', 'Onboarding Templates'),
		description: t('onboarding.templates.description_long', 'Configure onboarding step sequences. Templates are a future enhancement (§21) — the CRUD surface lets admins pre-configure them ahead of consumer-side wiring.'),
		createLabel: t('onboarding.templates.create', 'New Template'),
		createPermission: 'admin.settings',
		editPermission: 'admin.settings',
		deletePermission: 'admin.settings',
		pageSize: 25,
		columns: [
			{ key: 'name', label: t('onboarding.templates.name', 'Name'), sortable: true },
			{ key: 'description', label: t('onboarding.templates.description', 'Description') },
			{ key: 'is_active', label: t('onboarding.templates.active', 'Active'), sortable: true, componentSnippet: ActiveCell },
			{ key: 'is_system', label: t('onboarding.templates.system', 'System'), sortable: true, componentSnippet: SystemCell }
		],
		filters: [
			{
				key: 'is_active',
				label: t('onboarding.templates.status', 'Status'),
				options: [
					{ value: 'true', label: t('onboarding.templates.active', 'Active') },
					{ value: 'false', label: t('onboarding.templates.inactive', 'Inactive') }
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
			const result = await api<OnboardingTemplate[]>('/admin/onboarding/templates?page_size=1000');
			if (result.ok) {
				let data = (result.data as OnboardingTemplate[]) ?? [];
				const query = (q ?? '').toLowerCase();
				if (query) {
					data = data.filter((row) =>
						[row.name, row.description].some((v) =>
							String(v ?? '').toLowerCase().includes(query)
						)
					);
				}
				if (filter?.is_active) {
					data = data.filter((row) => String(row.is_active) === filter.is_active);
				}
				const size = pageSize || 25;
				const offset = (page || 0) * size;
				const total = data.length;
				const totalPages = Math.max(1, Math.ceil(total / size));
				return {
					data: data.slice(offset, offset + size),
					total,
					totalPages
				};
			}
			throw new Error(result.error ?? t('onboarding.templates.load_error', 'Failed to load templates'));
		},
		create: async (body) => {
			const res = await onboardingAdminApi.createTemplate({
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim() || '',
				steps: parseSteps(String(body.steps ?? '[]')),
				is_active: body.is_active === undefined ? true : !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		update: async (id, body) => {
			const res = await onboardingAdminApi.updateTemplate(id, {
				name: String(body.name ?? '').trim(),
				description: String(body.description ?? '').trim() || '',
				steps: parseSteps(String(body.steps ?? '[]')),
				is_active: !!body.is_active
			});
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		remove: async (id) => {
			const res = await onboardingAdminApi.deleteTemplate(id);
			return res.ok ? { ok: true } : { ok: false, error: res.error };
		},
		// System templates cannot be deleted (backend enforces this; mirror in UI).
		isDeleted: () => false,
		viewSnippet: TemplateView,
		formSnippet: TemplateForm
	};

	// Hide delete for system templates via a custom actions array.
	const config: EntityPageConfig = {
		...baseConfig,
		actions: [
			{
				label: t('common.view', 'View'),
				onclick: (row, ctx) => ctx?.openView(row)
			},
			{
				label: t('common.edit', 'Edit'),
				onclick: (row, ctx) => ctx?.openEdit(row),
				show: (row) => !row.is_system
			},
			{
				label: t('common.delete', 'Delete'),
				variant: 'destructive',
				onclick: (row, ctx) => ctx?.openDelete(row),
				show: (row) => !row.is_system && canManage
			}
		]
	};
</script>

<svelte:head>
	<title>{t('onboarding.templates.title', 'Onboarding Templates')} — {siteName()} Admin</title>
</svelte:head>

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

{#snippet ActiveCell({ value }: { value: boolean; row: OnboardingTemplate })}
	<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium {value ? 'bg-success/15 text-success' : 'bg-muted/15 text-muted'}">
		{value ? t('onboarding.templates.active', 'Active') : t('onboarding.templates.inactive', 'Inactive')}
	</span>
{/snippet}

{#snippet SystemCell({ value }: { value: boolean; row: OnboardingTemplate })}
	{#if value}
		<span class="inline-block rounded-full bg-primary/15 px-2 py-0.5 text-xs font-medium text-primary">
			{t('onboarding.templates.system', 'System')}
		</span>
	{:else}
		<span class="text-xs" style="color: var(--color-muted)">—</span>
	{/if}
{/snippet}

<!-- Create/edit form: name + description as standard inputs, steps as a
     JSON textarea (EntityForm has no native JSON field type). -->
{#snippet TemplateForm({ value, editing }: { value: Record<string, any>; editing: boolean })}
	<div class="space-y-4">
		<div>
			<label for="tpl-name" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('onboarding.templates.name', 'Name')} <span style="color: var(--color-danger)">*</span>
			</label>
			<span class="mt-1 block text-xs" style="color: var(--color-muted)">{t('onboarding.templates.name_hint', 'The name shown in admin lists and on the site where public.')}</span>
			<input
				id="tpl-name"
				type="text"
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.name}
				placeholder={t('onboarding.templates.name_ph', 'Default 7-step flow')}
				required
			/>
		</div>
		<div>
			<label for="tpl-desc" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('onboarding.templates.description', 'Description')}
			</label>
			<span class="mt-1 block text-xs" style="color: var(--color-muted)">{t('onboarding.templates.desc_hint', 'Shown in detail views — keep it concise.')}</span>
			<textarea
				id="tpl-desc"
				rows="2"
				class="w-full rounded-lg border px-3 py-2 text-sm"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				bind:value={value.description}
				placeholder={t('onboarding.templates.description_ph', 'Describe what this template configures…')}
			></textarea>
		</div>
		<div>
			<label for="tpl-steps" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
				{t('onboarding.templates.steps', 'Steps (JSON)')}
			</label>
			<span class="mt-1 block text-xs" style="color: var(--color-muted)">{t('onboarding.templates.steps_hint', 'The ordered steps — one per line.')}</span>
			<textarea
				id="tpl-steps"
				rows="10"
				class="w-full rounded-lg border px-3 py-2 font-mono text-xs"
				style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)"
				value={stepsToText(value.steps)}
				oninput={(e) => (value.steps = e.currentTarget.value)}
				placeholder={'[{"step":1,"title":"Welcome"},{"step":2,"title":"Destination"}]'}
			></textarea>
			<p class="mt-1 text-xs" style="color: var(--color-muted)">
				{t('onboarding.templates.steps_help', 'A JSON array describing the ordered onboarding steps. Leave as [] for the default 5-step flow.')}
			</p>
		</div>
		<div class="flex items-center gap-2">
			<input
				id="tpl-active"
				type="checkbox"
				class="rounded"
				bind:checked={value.is_active}
			/>
			<label for="tpl-active" class="text-sm" style="color: var(--color-foreground)">
				{t('onboarding.templates.active', 'Active')}
			</label>
			<span class="mt-1 block text-xs" style="color: var(--color-muted)">{t('onboarding.templates.active_hint', 'The active for this record.')}</span>
		</div>
	</div>
{/snippet}

<!-- View slide-over body -->
{#snippet TemplateView({ row }: { row: OnboardingTemplate })}
	<div class="space-y-4">
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('onboarding.templates.name', 'Name')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">{row.name}</p>
		</div>
		{#if row.description}
			<div>
				<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('onboarding.templates.description', 'Description')}</p>
				<p class="mt-1 text-sm" style="color: var(--color-foreground)">{row.description}</p>
			</div>
		{/if}
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('onboarding.templates.status', 'Status')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_active ? t('onboarding.templates.active', 'Active') : t('onboarding.templates.inactive', 'Inactive')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('onboarding.templates.system', 'System')}</p>
			<p class="mt-1 text-sm font-medium" style="color: var(--color-foreground)">
				{row.is_system ? t('common.yes', 'Yes') : t('common.no', 'No')}
			</p>
		</div>
		<div>
			<p class="text-xs font-medium uppercase tracking-wider" style="color: var(--color-muted)">{t('onboarding.templates.steps', 'Steps (JSON)')}</p>
			<pre class="mt-1 max-h-64 overflow-auto rounded-lg border p-3 text-xs" style="border-color: var(--color-border); background: var(--color-surface); color: var(--color-foreground)">{stepsToText(row.steps)}</pre>
		</div>
	</div>
{/snippet}
