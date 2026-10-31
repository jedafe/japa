<script lang="ts">
	import { siteName } from '$lib/stores/site.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Modal } from '$lib/components/ui/modal';
	import { Button } from '$lib/components/ui/button';
	import { api, apiRaw, formsAdminApi, adminCatalogApi, type CatalogOption } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import type { FilterDef } from '$lib/components/ui/data-table';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';

	// Seeded form nodes are forms.edit/forms.delete/forms.submissions.view —
	// "space.forms.manage" does not exist.
	const canEdit = $derived(hasPermission('forms.edit'));
	const canDelete = $derived(hasPermission('forms.delete'));
	const canViewSubmissions = $derived(hasPermission('forms.submissions.view'));

	// GET /forms/{id} returns a flat form object — fields carry
	// {id,label,type,required,sort_order,options}.
	type FieldDef = {
		id: string;
		label: string;
		type: string;
		required: boolean;
		sort_order: number;
		options: string[];
	};

	type FormDef = {
		id: string;
		name: string;
		description?: string;
		fields: FieldDef[];
		created_at: string;
	};

	// ListFormSubmissionsByFormRow — there is no `data`/`submitter_id`;
	// answers live in the JSONB `field_values` map, keyed by field label
	// (or field key for API callers).
	type Submission = {
		id: string;
		user_id?: string;
		ip_address?: string;
		field_values: Record<string, unknown>;
		status: string;
		created_at: string;
	};

	const spaceId = $derived(page.params.spaceId ?? '');
	const formId = $derived(page.params.formId ?? '');

	let form = $state<FormDef | null>(null);
	let loading = $state(true);
	const FORM_TABS = ['edit', 'submissions'] as const;
	let activeTab = $state<'edit' | 'submissions'>(initTabParam('edit', { valid: FORM_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: FORM_TABS, preserve: [] });
	let errorMessage = $state('');
	let saveMsg = $state('');

	let editName = $state('');
	let editDescription = $state('');
	let saving = $state(false);

	let showDelete = $state(false);
	let deleting = $state(false);

	// ── Server-paginated submissions ─────────────────────────────────────
	let submissions = $state<Submission[]>([]);
	let subLoading = $state(false);
	let subPage = $state(0); // DataTable pages are 0-based
	let subPageSize = $state(25);
	let subTotal = $state(0);
	let subFilter = $state<Record<string, string>>({});
	const subTotalPages = $derived(Math.max(1, Math.ceil(subTotal / subPageSize)));

	let statusOptions = $state<CatalogOption[]>([
		{ value: 'new', label: 'New' },
		{ value: 'reviewed', label: 'Reviewed' },
		{ value: 'pending_payment', label: 'Pending Payment' }
	]);

	async function loadStatusOptions() {
		try {
			const res = await adminCatalogApi.get('forms.submission_status');
			if (res.ok && res.data?.length) statusOptions = res.data;
		} catch {
			// fallback stays
		}
	}

	// Option filters render through FilterPanel's shared Autocomplete.
	const subFilters: FilterDef[] = $derived([
		{ key: 'status', label: t('space_forms.status', 'Status'), options: statusOptions }
	]);

	// Field values are keyed by label or by the generated key
	// (lowercase label, spaces → underscores).
	function fieldValue(sub: Submission, field: FieldDef): unknown {
		const fv = sub.field_values ?? {};
		if (field.label in fv) return fv[field.label];
		const key = field.label.toLowerCase().replace(/\s+/g, '_');
		if (key in fv) return fv[key];
		return undefined;
	}

	const submissionColumns = $derived([
		{ key: 'created_at', label: t('space_forms.submitted'), sortable: false, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v ? new Date(v).toLocaleString() : '—'}</span>` },
		{ key: 'status', label: t('space_forms.status', 'Status'), sortable: false, class: 'text-xs', render: (v: string) => `<span class="inline-block rounded-full px-2 py-0.5 text-xs font-medium bg-black/5 dark:bg-surface/10">${v || '—'}</span>` },
		...(form?.fields ?? []).map((field: FieldDef) => ({
			key: `field_${field.id}`,
			label: field.label,
			sortable: false,
			class: 'text-xs',
			render: (_v: unknown, row: Record<string, unknown>) => formatFieldValue(row[`field_${field.id}`])
		}))
	]);

	function getSubmissionRow(sub: Submission): Record<string, unknown> {
		const row: Record<string, unknown> = { id: sub.id, created_at: sub.created_at, status: sub.status };
		for (const field of form?.fields ?? []) {
			row[`field_${field.id}`] = fieldValue(sub, field);
		}
		return row;
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<FormDef>(`/forms/${formId}`);
			if (result.ok) {
				form = result.data;
				editName = form.name;
				editDescription = form.description ?? '';
			} else {
				errorMessage = result.error ?? t('space_forms.load_error');
			}
		} catch {
			errorMessage = t('space_forms.load_error');
		} finally {
			loading = false;
		}
	}

	async function loadSubmissions() {
		subLoading = true;
		try {
			const params = new URLSearchParams({
				page_size: String(subPageSize),
				page_offset: String(subPage * subPageSize)
			});
			if (subFilter.status) params.set('status', subFilter.status);
			// apiRaw keeps the top-level `total` the handler emits next to `data`.
			const result = await apiRaw<{ data: Submission[]; total: number }>(`/forms/${formId}/submissions?${params}`);
			if (result.ok) {
				submissions = Array.isArray(result.data?.data) ? result.data.data : [];
				subTotal = result.data?.total ?? 0;
			} else {
				errorMessage = result.error ?? t('space_forms.load_error');
			}
		} catch {
			errorMessage = t('space_forms.load_error');
		} finally {
			subLoading = false;
		}
	}

	async function saveMetadata() {
		if (!editName.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			const result = await api(`/forms/${formId}`, {
				method: 'PATCH',
				body: { name: editName.trim(), description: editDescription.trim() }
			});
			if (!result.ok) {
				errorMessage = result.error ?? t('space_forms.save_error');
				return;
			}
			saveMsg = t('common.saved');
			setTimeout(() => (saveMsg = ''), 3000);
			load();
		} catch {
			errorMessage = t('space_forms.save_error');
		} finally {
			saving = false;
		}
	}

	async function deleteForm() {
		if (deleting) return;
		deleting = true;
		try {
			const result = await formsAdminApi.delete(formId);
			if (!result.ok) {
				errorMessage = result.error ?? t('space_forms.delete_error');
				return;
			}
			await goto(`/spaces/${spaceId}/forms`);
		} catch {
			errorMessage = t('space_forms.delete_error');
		} finally {
			deleting = false;
		}
	}

	function formatFieldValue(value: unknown): string {
		if (value === null || value === undefined) return '—';
		if (typeof value === 'boolean') return value ? 'Yes' : 'No';
		if (typeof value === 'object') return JSON.stringify(value);
		return String(value);
	}

	$effect(() => {
		load();
		loadSubmissions();
		loadStatusOptions();
	});
</script>

<svelte:head>
	<title>{form?.name ?? t('space_forms.detail_title')} — {siteName()} Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between gap-2 flex-wrap">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{form?.name ?? t('space_forms.detail_title')}</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">{t('space_forms.detail_description')}</p>
	</div>
	<div class="flex items-center gap-2">
		<a
			href="/spaces/{spaceId}/forms"
			class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
			style="border-color: var(--color-border)"
		>
			&larr; {t('common.back')}
		</a>
		<a
			href="/forms/{formId}"
			class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
			style="border-color: var(--color-border); color: var(--color-primary)"
		>
			{t('space_forms.open_full_editor', 'Open Full Editor')}
		</a>
		{#if canDelete}
			<button
				class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
				style="border-color: var(--color-border); color: var(--color-danger)"
				onclick={() => (showDelete = true)}
			>
				{t('common.delete')}
			</button>
		{/if}
	</div>
</div>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<!-- Tabs -->
<div class="mb-6 flex gap-1 border-b" style="border-color: var(--color-border)">
	<button
		class="rounded-t-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'edit' ? 'color: var(--color-primary); border-bottom: 2px solid var(--color-primary)' : 'color: var(--color-muted)'}
		onclick={() => (activeTab = 'edit')}
	>
		{t('space_forms.tab_edit')}
	</button>
	<button
		class="rounded-t-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'submissions' ? 'color: var(--color-primary); border-bottom: 2px solid var(--color-primary)' : 'color: var(--color-muted)'}
		onclick={() => (activeTab = 'submissions')}
	>
		{t('space_forms.tab_submissions').replace('{count}', String(subTotal))}
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
{:else if activeTab === 'edit'}
	<div class="space-y-6">
		<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('space_forms.sections.details')}</h2>
			<div class="space-y-4">
				<div>
					<label for="edit-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space_forms.field_label')}</label>
					<input
						id="edit-name"
						type="text"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						bind:value={editName}
						disabled={!canEdit}
					/>
				</div>
				<div>
					<label for="edit-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">{t('space_forms.description_label')}</label>
					<textarea
						id="edit-desc"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						rows="2"
						bind:value={editDescription}
						disabled={!canEdit}
					></textarea>
				</div>
			</div>
			{#if canEdit}
				<div class="mt-4 flex items-center gap-3">
					<button
						class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
						style="background-color: var(--color-primary)"
						disabled={saving || !editName.trim()}
						onclick={saveMetadata}
					>
						{saving ? t('common.saving') : t('common.save')}
					</button>
					{#if saveMsg}
						<span class="text-xs" style="color: var(--color-success)">{saveMsg}</span>
					{/if}
				</div>
			{/if}
		</div>

		<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="mb-4 flex items-center justify-between gap-2 flex-wrap">
				<h2 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">{t('space_forms.sections.fields')}</h2>
				<a
					href="/forms/{formId}"
					class="text-xs font-medium transition-colors hover:underline"
					style="color: var(--color-primary)"
				>
					{t('space_forms.manage_fields', 'Manage fields in full editor →')}
				</a>
			</div>
			{#if form?.fields && form.fields.length > 0}
				<div class="space-y-3">
					{#each form.fields as field (field.id)}
						<div class="flex items-center justify-between gap-2 flex-wrap rounded-md border px-4 py-3" style="border-color: var(--color-border); background-color: var(--color-background)">
							<div>
								<span class="text-sm font-medium">{field.label}</span>
								<span class="ms-2 inline-block rounded-full px-2 py-0.5 text-xs bg-black/5 dark:bg-surface/10">{field.type}</span>
								{#if field.required}
									<span class="ms-1 text-xs" style="color: var(--color-danger)">{t('space_forms.required_indicator')}</span>
								{/if}
							</div>
							{#if field.options && field.options.length > 0}
								<span class="text-xs" style="color: var(--color-muted)">{t('space_forms.options_count').replace('{count}', String(field.options.length))}</span>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<p class="text-sm" style="color: var(--color-muted)">{t('space_forms.no_fields')}</p>
			{/if}
		</div>
	</div>
{:else}
	{#if !canViewSubmissions}
		<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">{t('common.noPermission', 'You do not have permission to view submissions')}</p>
		</div>
	{:else}
		<DataTable
			columns={submissionColumns}
			data={submissions.map(getSubmissionRow)}
			serverSide
			loading={subLoading}
			totalRows={subTotal}
			totalPages={subTotalPages}
			bind:page={subPage}
			bind:pageSize={subPageSize}
			onpagechange={(p) => { subPage = p; loadSubmissions(); }}
			onpagesizechange={(s) => { subPageSize = s; subPage = 0; loadSubmissions(); }}
			filters={subFilters}
			bind:filterValue={subFilter}
			onfilterapply={() => { subPage = 0; loadSubmissions(); }}
			onfilterclear={() => { subPage = 0; loadSubmissions(); }}
			emptyMessage={t('space_forms.no_submissions')}
		/>
	{/if}
{/if}

<!-- Delete Confirmation -->
<Modal bind:open={showDelete} title={t('space_forms.delete_title')} size="sm">
	<p class="text-sm">{t('space_forms.delete_text').replace('{name}', form?.name ?? '')}</p>
	<div class="mt-6 flex justify-end gap-2">
		<Button variant="outline" onclick={() => { showDelete = false; }}>{t('common.cancel', 'Cancel')}</Button>
		<Button variant="destructive" onclick={deleteForm} disabled={deleting}>{deleting ? t('common.deleting', 'Deleting...') : t('common.delete', 'Delete')}</Button>
	</div>
</Modal>
