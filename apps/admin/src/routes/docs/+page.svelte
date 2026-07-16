<script lang="ts">
	import Modal from '$lib/components/Modal.svelte';
	import FilterBar from '$lib/components/FilterBar.svelte';
	import { Select } from '$lib/components/ui/select';
	import { docsApi } from '$lib/api';

	type DocPage = {
		id: string;
		title: string;
		slug: string;
		audience: string;
		status: 'draft' | 'published';
		body: string;
		updated_at: string;
	};

	let docs = $state<DocPage[]>([]);
	let page = $state(0);
	let totalPages = $state(1);
	let loading = $state(true);
	let errorMessage = $state('');
	let filterAudience = $state('');

	let showModal = $state(false);
	let editingId = $state<string | null>(null);
	let saving = $state(false);
	let showDeleteConfirm = $state(false);
	let deleteTargetId = $state<string | null>(null);

	let form = $state({
		title: '',
		slug: '',
		audience: 'public',
		body: ''
	});

	const audiences = ['developer', 'user', 'admin', 'public'];

	const filterDefs = [
		{ key: 'audience', label: 'Audience', type: 'select' as const, options: audiences }
	];

	function slugify(text: string): string {
		return text
			.toLowerCase()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-|-$/g, '');
	}

	async function load() {
		loading = true;
		errorMessage = '';
		const params = new URLSearchParams({ page: String(page) });
		if (filterAudience) params.set('audience', filterAudience);
		const result = await docsApi.list(params.toString());
		if (result.ok) {
			const data = result.data as { items: DocPage[]; total_pages: number };
			docs = data.items ?? [];
			totalPages = data.total_pages ?? 1;
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function handleFilter(values: Record<string, string>) {
		filterAudience = values.audience ?? '';
		page = 0;
		load();
	}

	function openCreate() {
		editingId = null;
		form = { title: '', slug: '', audience: 'public', body: '' };
		showModal = true;
	}

	function openEdit(item: DocPage) {
		editingId = item.id;
		form = {
			title: item.title,
			slug: item.slug,
			audience: item.audience,
			body: item.body ?? ''
		};
		showModal = true;
	}

	async function submit() {
		if (!form.title.trim()) return;
		saving = true;
		errorMessage = '';

		const slug = form.slug.trim() || slugify(form.title);
		const body = {
			title: form.title.trim(),
			slug,
			audience: form.audience,
			body: form.body
		};

		const result = editingId
			? await docsApi.update(editingId, body)
			: await docsApi.create(body);

		if (result.ok) {
			showModal = false;
			load();
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	async function togglePublish(item: DocPage) {
		errorMessage = '';
		const newStatus = item.status === 'published' ? 'draft' : 'published';
		const result = await docsApi.update(item.id, { status: newStatus });
		if (result.ok) {
			load();
		} else {
			errorMessage = result.error;
		}
	}

	function confirmDelete(id: string) {
		deleteTargetId = id;
		showDeleteConfirm = true;
	}

	async function executeDelete() {
		if (!deleteTargetId) return;
		errorMessage = '';
		const result = await docsApi.delete(deleteTargetId);
		if (result.ok) {
			showDeleteConfirm = false;
			deleteTargetId = null;
			load();
		} else {
			errorMessage = result.error;
		}
	}

	function handlePageChange(p: number) {
		page = p;
		load();
	}

	$effect(() => {
		load();
	});
</script>

<svelte:head>
	<title>Documentation — JapaConnect Admin</title>
</svelte:head>

<h1 class="mb-1 text-2xl font-bold tracking-tight">Documentation</h1>
<p class="mb-4 text-sm" style="color: var(--color-muted)">Manage documentation pages for developers, users, admins, and the public</p>

{#if errorMessage}
	<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
		{errorMessage}
	</div>
{/if}

<FilterBar filters={filterDefs} onApply={handleFilter} />

<div class="mb-4 flex justify-end">
	<button
		class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors"
		style="background-color: var(--color-primary)"
		onclick={openCreate}
	>
		New Doc Page
	</button>
</div>

{#if loading}
	<p class="py-8 text-center text-sm" style="color: var(--color-muted)">Loading...</p>
{:else}
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Title</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Slug</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Audience</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Status</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">Updated</th>
					<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)"></th>
				</tr>
			</thead>
			<tbody>
				{#each docs as doc}
					<tr class="border-t hover:bg-black/[0.02] dark:hover:bg-white/[0.02]" style="border-color: var(--color-border)">
						<td class="px-4 py-3 font-medium">{doc.title}</td>
						<td class="px-4 py-3 font-mono text-xs" style="color: var(--color-muted)">{doc.slug}</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10">{doc.audience}</span>
						</td>
						<td class="px-4 py-3">
							<span class="inline-block rounded-full px-2 py-0.5 text-xs {doc.status === 'published' ? 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-400' : 'bg-amber-500/15 text-amber-700 dark:text-amber-400'}">
								{doc.status}
							</span>
						</td>
						<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{doc.updated_at}</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-2">
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-info)"
									onclick={() => openEdit(doc)}
								>
									Edit
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: {doc.status === 'published' ? 'var(--color-warning)' : 'var(--color-success)'}"
									onclick={() => togglePublish(doc)}
								>
									{doc.status === 'published' ? 'Unpublish' : 'Publish'}
								</button>
								<button
									class="rounded px-2 py-1 text-xs hover:bg-black/5 dark:hover:bg-white/10"
									style="color: var(--color-danger)"
									onclick={() => confirmDelete(doc.id)}
								>
									Delete
								</button>
							</div>
						</td>
					</tr>
				{:else}
					<tr>
						<td colspan="6" class="px-4 py-8 text-center" style="color: var(--color-muted)">No documentation pages found</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div class="mt-3 flex items-center justify-between">
		<p class="text-xs" style="color: var(--color-muted)">Page {page + 1} of {totalPages}</p>
		<div class="flex gap-1">
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page === 0} onclick={() => handlePageChange(page - 1)}>&larr;</button>
			<button class="rounded p-1 hover:bg-black/5 dark:hover:bg-white/10 disabled:opacity-40" disabled={page >= totalPages - 1} onclick={() => handlePageChange(page + 1)}>&rarr;</button>
		</div>
	</div>
{/if}

<!-- Create/Edit Modal (wide for body textarea) -->
{#if showModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
		onclick={(e) => { if (e.target === e.currentTarget) showModal = false; }}
		onkeydown={(e) => { if (e.key === 'Escape') showModal = false; }}
	>
		<div class="w-full max-w-2xl rounded-lg border p-6 shadow-lg" style="border-color: var(--color-border); background-color: var(--color-background)">
			<h2 class="text-lg font-semibold">{editingId ? 'Edit Doc Page' : 'New Doc Page'}</h2>
			<div class="mt-4 space-y-3">
				<div class="grid gap-3 sm:grid-cols-2">
					<div>
						<label for="doc-title" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Title</label>
						<input id="doc-title" type="text" class="w-full rounded-md border px-3 py-2 text-sm" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.title} oninput={() => { if (!editingId) form.slug = slugify(form.title); }} />
					</div>
					<div>
						<label for="doc-slug" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Slug</label>
						<input id="doc-slug" type="text" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" bind:value={form.slug} />
					</div>
				</div>
				<div>
					<label for="doc-audience" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Audience</label>
					<Select
						options={audiences.map(a => ({ value: a, label: a }))}
						bind:value={form.audience}
					/>
				</div>
				<div>
					<label for="doc-body" class="mb-1 block text-xs font-medium" style="color: var(--color-muted)">Body (Markdown)</label>
					<textarea id="doc-body" class="w-full rounded-md border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background-color: var(--color-surface)" rows="12" bind:value={form.body}></textarea>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button
					class="rounded-md border px-3 py-1.5 text-sm font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
					style="border-color: var(--color-border)"
					onclick={() => (showModal = false)}
				>
					Cancel
				</button>
				<button
					class="rounded-md px-3 py-1.5 text-sm font-medium text-white transition-colors disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving}
					onclick={submit}
				>
					{saving ? 'Saving...' : (editingId ? 'Update' : 'Create')}
				</button>
			</div>
		</div>
	</div>
{/if}

<!-- Delete Confirmation -->
<Modal bind:open={showDeleteConfirm} title="Confirm Delete" onConfirm={executeDelete} confirmLabel="Delete">
	<p class="text-sm" style="color: var(--color-muted)">Are you sure you want to delete this documentation page? This action cannot be undone.</p>
</Modal>
