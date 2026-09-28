<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { DataTable } from '$lib/components/ui/data-table';
	import { initTabParam, bindTabParam } from '$lib/utils/url-sync.svelte';

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
		description: string;
		fields: FieldDef[];
		created_at: string;
	};

	type Submission = {
		id: string;
		submitter_id: string;
		data: Record<string, unknown>;
		created_at: string;
	};

	const spaceId = $derived(page.params.spaceId);
	const formId = $derived(page.params.formId);

	let form = $state<FormDef | null>(null);
	let submissions = $state<Submission[]>([]);
	let loading = $state(true);
	const FORM_TABS = ['edit', 'submissions'] as const;
	let activeTab = $state<'edit' | 'submissions'>(initTabParam('edit', { valid: FORM_TABS }));
	bindTabParam(() => activeTab, (v) => (activeTab = v), { valid: FORM_TABS, preserve: [] });
	let errorMessage = $state('');
	let saveMsg = $state('');

	let searchQuery = $state('');

	let editName = $state('');
	let editDescription = $state('');
	let saving = $state(false);

	let showDelete = $state(false);

	const submissionColumns = $derived([
		{ key: 'created_at', label: t('space_forms.submitted'), sortable: true, class: 'text-xs', render: (v: string) => `<span style="color: var(--color-muted)">${v}</span>` },
		...(form?.fields ?? []).map((field: FieldDef) => ({
			key: `field_${field.label}`,
			label: field.label,
			sortable: false,
			class: 'text-xs',
			render: (_v: unknown, row: Submission) => formatFieldValue(row.data[field.label])
		}))
	]);

	function getSubmissionData(sub: Submission): Record<string, unknown> {
		const row: Record<string, unknown> = { id: sub.id, created_at: sub.created_at, submitter_id: sub.submitter_id };
		for (const field of (form?.fields ?? [])) {
			row[`field_${field.label}`] = sub.data[field.label];
		}
		return row;
	}

	function filteredSubmissions(): Record<string, unknown>[] {
		const q = searchQuery.toLowerCase().trim();
		const rows = submissions.map(getSubmissionData);
		if (!q) return rows;
		return rows.filter(r => {
			return Object.values(r).some(v => {
				if (v === null || v === undefined) return false;
				return String(v).toLowerCase().includes(q);
			});
		});
	}

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const result = await api<FormDef>(`/forms/${formId}`);
			if (result.ok) {
				form = result.data;
				editName = form.name;
				editDescription = form.description;
			}
		} catch {
			errorMessage = t('space_forms.load_error');
		} finally {
			loading = false;
		}
	}

	async function loadSubmissions() {
		try {
			const result = await api<Submission[]>(`/forms/${formId}/submissions`);
			if (result.ok) {
				submissions = result.data;
			}
		} catch {
		}
	}

	async function saveMetadata() {
		if (!editName.trim()) return;
		saving = true;
		errorMessage = '';
		try {
			await api(`/forms/${formId}`, {
				method: 'PATCH',
				body: { name: editName.trim(), description: editDescription.trim() }
			});
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
		try {
			await api(`/forms/${formId}`, { method: 'DELETE' });
			await goto(`/spaces/${spaceId}/forms`);
		} catch {
			errorMessage = t('space_forms.delete_error');
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
	});
</script>

<svelte:head>
	<title>Form Detail — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between">
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
		<button
			class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
			style="border-color: var(--color-border); color: var(--color-danger)"
			onclick={() => (showDelete = true)}
		>
			{t('common.delete')}
		</button>
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
		{t('space_forms.tab_submissions').replace('{count}', String(submissions.length))}
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
					></textarea>
				</div>
			</div>
			<div class="mt-4 flex items-center gap-3">
				<button
					class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving}
					onclick={saveMetadata}
				>
					{saving ? t('common.saving') : t('common.save')}
				</button>
				{#if saveMsg}
					<span class="text-xs" style="color: var(--color-success)">{saveMsg}</span>
				{/if}
			</div>
		</div>

		<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="mb-4 flex items-center justify-between">
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
					{#each form.fields as field (field)}
						<div class="flex items-center justify-between rounded-md border px-4 py-3" style="border-color: var(--color-border); background-color: var(--color-background)">
							<div>
								<span class="text-sm font-medium">{field.label}</span>
								<span class="ml-2 inline-block rounded-full px-2 py-0.5 text-xs bg-black/5 dark:bg-surface/10">{field.type}</span>
								{#if field.required}
									<span class="ml-1 text-xs" style="color: var(--color-danger)">{t('space_forms.required_indicator')}</span>
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
	{#if submissions.length === 0}
		<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">{t('space_forms.no_submissions')}</p>
		</div>
	{:else}
		<DataTable
			columns={submissionColumns}
			data={filteredSubmissions()}
			pageSize={25}
			emptyMessage={t('space_forms.no_submissions')}
			searchable
			searchPlaceholder={t('common.search')}
			onsearch={(q) => { searchQuery = q; }}
		/>
	{/if}
{/if}

<!-- Delete Confirmation -->
<Modal bind:open={showDelete} title={t('space_forms.delete_title')} onConfirm={deleteForm} confirmLabel={t('common.delete')}>
	<p class="text-sm">{t('space_forms.delete_text').replace('{name}', form?.name ?? '')}</p>
</Modal>
