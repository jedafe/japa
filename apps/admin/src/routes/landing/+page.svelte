<script lang="ts">
	import type { PageData } from './$types';
	import { goto } from '$app/navigation';
	import { DataTable } from '$lib/components/ui/data-table';
	import { Dialog } from 'bits-ui';
	import { api } from '$lib/api';

	interface LandingPage {
		id: string;
		title: string;
		slug: string;
		page_type: string;
		status: string;
		updated_at: string;
	}

	let { data }: { data: PageData } = $props();

	let pages = $derived<LandingPage[]>(data.pages ?? []);
	let error = $state<string | null>(null);
	let showCreate = $state(false);
	let newSlug = $state('');
	let newTitle = $state('');
	let creating = $state(false);

	// Sync error from server load data when it changes (e.g. navigation/reload).
	$effect(() => {
		error = data.error ?? null;
	});

	function slugify(value: string): string {
		return value.toLowerCase().trim()
			.replace(/[^a-z0-9]+/g, '-')
			.replace(/^-+|-+$/g, '');
	}

	const columns = [
		{ key: 'title', label: 'Title', sortable: true },
		{ key: 'slug', label: 'Slug', sortable: true },
		{
			key: 'status',
			label: 'Status',
			sortable: true,
			componentSnippet: StatusBadge,
		},
		{
			key: 'updated_at',
			label: 'Last Updated',
			sortable: true,
			render: (v: string) => v ? new Date(v).toLocaleDateString() : '—',
		},
		{
			key: 'actions',
			label: 'Actions',
			componentSnippet: ActionsCell,
		},
	];

	async function createLanding() {
		if (!newSlug.trim()) return;
		creating = true;
		error = null;
		try {
			const slug = newSlug.trim() || slugify(newTitle);
			const result = await api<{ id: string; slug: string }>('/admin/pages', {
				method: 'POST',
				body: { title: newTitle || slug, slug, page_type: 'landing' },
			});
			if (!result.ok) {
				error = result.error;
			} else {
				showCreate = false;
				newSlug = '';
				newTitle = '';
				goto('/landing/' + result.data.slug);
			}
		} catch {
			error = 'Failed to create landing page';
		} finally {
			creating = false;
		}
	}
</script>

<svelte:head>
	<title>Landing Pages — JapaConnect Admin</title>
</svelte:head>

<div class="p-6">
	<div class="mb-6 flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold tracking-tight" style="color: var(--foreground)">Landing Pages</h1>
			<p class="text-sm" style="color: var(--muted)">Manage standalone landing pages.</p>
		</div>
		<button
			type="button"
			onclick={() => (showCreate = true)}
			class="rounded-md px-4 py-2 text-sm font-semibold text-white transition-colors hover:opacity-90"
			style="background-color: var(--primary)"
		>
			Create Landing Page
		</button>
	</div>

	{#if error}
		<div
			class="mb-4 rounded-md border p-3 text-sm"
			style="border-color: var(--danger); background-color: color-mix(in srgb, var(--danger) 10%, transparent); color: var(--danger)"
		>
			{error}
		</div>
	{/if}

	<DataTable {columns} data={pages} pageSize={25} searchable searchPlaceholder="Search landing pages..." />
</div>

{#snippet StatusBadge({ value }: { value: string; row: LandingPage })}
	<span
		class="rounded-full px-2 py-0.5 text-xs font-medium"
		style={value === 'published'
			? 'background: color-mix(in srgb, var(--success) 15%, transparent); color: var(--success)'
			: value === 'archived'
				? 'background: color-mix(in srgb, var(--muted) 15%, transparent); color: var(--muted)'
				: 'background: color-mix(in srgb, var(--warning) 15%, transparent); color: var(--warning)'}
	>
		{value}
	</span>
{/snippet}

{#snippet ActionsCell({ row }: { value: any; row: LandingPage })}
	<button
		type="button"
		onclick={() => goto('/landing/' + row.slug)}
		class="text-sm font-medium hover:underline"
		style="color: var(--primary)"
	>
		Edit
	</button>
{/snippet}

<Dialog.Root bind:open={showCreate}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-50 bg-black/50" />
		<Dialog.Content
			class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border p-6 shadow-xl"
			style="border-color: var(--border); background-color: var(--background)"
		>
			<Dialog.Title class="mb-4 text-lg font-semibold" style="color: var(--foreground)">
				Create Landing Page
			</Dialog.Title>
			<div class="space-y-3">
				<div>
					<label for="landing-title" class="mb-1 block text-sm font-medium" style="color: var(--foreground-alt)">
						Title
					</label>
					<input
						id="landing-title"
						type="text"
						bind:value={newTitle}
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--border-input); background-color: var(--background); color: var(--foreground)"
					/>
				</div>
				<div>
					<label for="landing-slug" class="mb-1 block text-sm font-medium" style="color: var(--foreground-alt)">
						Slug
					</label>
					<input
						id="landing-slug"
						type="text"
						bind:value={newSlug}
						placeholder="homepage"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--border-input); background-color: var(--background); color: var(--foreground)"
					/>
				</div>
			</div>
			<div class="mt-6 flex justify-end gap-2">
				<button
					type="button"
					onclick={() => (showCreate = false)}
					class="rounded-md px-4 py-2 text-sm font-medium"
					style="border: 1px solid var(--border); color: var(--foreground)"
				>
					Cancel
				</button>
				<button
					type="button"
					onclick={createLanding}
					disabled={creating || !newSlug.trim()}
					class="rounded-md px-4 py-2 text-sm font-semibold text-white transition-colors disabled:opacity-50"
					style="background-color: var(--primary)"
				>
					{creating ? 'Creating...' : 'Create'}
				</button>
			</div>
			<Dialog.Close
				class="absolute right-4 top-4 text-sm hover:opacity-70"
				style="color: var(--muted)"
				aria-label="Close"
			>
				&#10005;
			</Dialog.Close>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
