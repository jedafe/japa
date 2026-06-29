<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import Modal from '$lib/components/Modal.svelte';
	import { api } from '$lib/api';

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

	const spaceId = $derived($page.params.spaceId);
	const formId = $derived($page.params.formId);

	let form = $state<FormDef | null>(null);
	let submissions = $state<Submission[]>([]);
	let loading = $state(true);
	let activeTab = $state<'edit' | 'submissions'>('edit');
	let errorMessage = $state('');
	let saveMsg = $state('');

	// Edit form metadata
	let editName = $state('');
	let editDescription = $state('');
	let saving = $state(false);

	// Delete
	let showDelete = $state(false);

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
			errorMessage = 'Failed to load form.';
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
			// API not available
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
			saveMsg = 'Saved';
			setTimeout(() => (saveMsg = ''), 3000);
			load();
		} catch {
			errorMessage = 'Failed to save.';
		} finally {
			saving = false;
		}
	}

	async function deleteForm() {
		try {
			await api(`/forms/${formId}`, { method: 'DELETE' });
			window.location.href = `/spaces/${spaceId}/forms`;
		} catch {
			errorMessage = 'Failed to delete form.';
		}
	}

	function formatFieldValue(value: unknown): string {
		if (value === null || value === undefined) return '—';
		if (typeof value === 'boolean') return value ? 'Yes' : 'No';
		if (typeof value === 'object') return JSON.stringify(value);
		return String(value);
	}

	onMount(() => {
		load();
		loadSubmissions();
	});
</script>

<svelte:head>
	<title>Form Detail — JapaConnect Admin</title>
</svelte:head>

<div class="mb-4 flex items-center justify-between">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">{form?.name ?? 'Form Detail'}</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">Manage form definition and view submissions</p>
	</div>
	<div class="flex items-center gap-2">
		<a
			href="/spaces/{spaceId}/forms"
			class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="border-color: var(--color-border)"
		>
			&larr; Back
		</a>
		<button
			class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
			style="border-color: var(--color-border); color: var(--color-danger)"
			onclick={() => (showDelete = true)}
		>
			Delete
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
		Edit Form
	</button>
	<button
		class="rounded-t-md px-4 py-2 text-sm font-medium transition-colors"
		style={activeTab === 'submissions' ? 'color: var(--color-primary); border-bottom: 2px solid var(--color-primary)' : 'color: var(--color-muted)'}
		onclick={() => (activeTab = 'submissions')}
	>
		Submissions ({submissions.length})
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else if activeTab === 'edit'}
	<!-- Edit Form Definition -->
	<div class="space-y-6">
		<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">Form Details</h2>
			<div class="space-y-4">
				<div>
					<label for="edit-name" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Name</label>
					<input
						id="edit-name"
						type="text"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						bind:value={editName}
					/>
				</div>
				<div>
					<label for="edit-desc" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Description</label>
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
					{saving ? 'Saving...' : 'Save Changes'}
				</button>
				{#if saveMsg}
					<span class="text-xs" style="color: var(--color-success)">{saveMsg}</span>
				{/if}
			</div>
		</div>

		<!-- Field list (read-only display) -->
		<div class="rounded-lg border p-5" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<h2 class="mb-4 text-sm font-semibold uppercase tracking-wider" style="color: var(--color-muted)">Fields</h2>
			{#if form?.fields && form.fields.length > 0}
				<div class="space-y-3">
					{#each form.fields as field}
						<div class="flex items-center justify-between rounded-md border px-4 py-3" style="border-color: var(--color-border); background-color: var(--color-background)">
							<div>
								<span class="text-sm font-medium">{field.label}</span>
								<span class="ml-2 inline-block rounded-full px-2 py-0.5 text-xs bg-black/5 dark:bg-white/10">{field.type}</span>
								{#if field.required}
									<span class="ml-1 text-xs" style="color: var(--color-danger)">*required</span>
								{/if}
							</div>
							{#if field.options && field.options.length > 0}
								<span class="text-xs" style="color: var(--color-muted)">{field.options.length} options</span>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<p class="text-sm" style="color: var(--color-muted)">No fields defined.</p>
			{/if}
		</div>
	</div>
{:else}
	<!-- Submissions -->
	{#if submissions.length === 0}
		<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">No submissions yet.</p>
		</div>
	{:else}
		<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr style="border-color: var(--color-border)">
						<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Submitted</th>
						{#each form?.fields ?? [] as field}
							<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{field.label}</th>
						{/each}
					</tr>
				</thead>
				<tbody>
					{#each submissions as sub}
						<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
							<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{sub.created_at}</td>
							{#each form?.fields ?? [] as field}
								<td class="px-4 py-3 text-xs">{formatFieldValue(sub.data[field.label])}</td>
							{/each}
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
{/if}

<!-- Delete Confirmation -->
<Modal bind:open={showDelete} title="Delete Form" onConfirm={deleteForm} confirmLabel="Delete">
	<p class="text-sm">Are you sure you want to delete <strong>{form?.name}</strong>? All submissions will also be removed. This cannot be undone.</p>
</Modal>
