<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Track = {
		id: string;
		title: string;
		slug: string;
		description: string;
		status: string;
		module_count: number;
		enrollment_count: number;
		created_at: string;
	};

	type Module = {
		id: string;
		track_id: string;
		title: string;
		slug: string;
		lesson_count: number;
		sort_order: number;
	};

	type Lesson = {
		id: string;
		module_id: string;
		title: string;
		slug: string;
		duration_minutes: number;
		sort_order: number;
	};

	let activeTab = $state<'tracks' | 'modules' | 'lessons'>('tracks');
	let tracks = $state<Track[]>([]);
	let modules = $state<Module[]>([]);
	let lessons = $state<Lesson[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Create forms
	let showCreateModal = $state(false);
	let creating = $state(false);
	let newTrack = $state({ title: '', description: '' });
	let newModule = $state({ track_id: '', title: '' });
	let newLesson = $state({ module_id: '', title: '', body: '' });

	// Edit
	let editingItem = $state<Track | Module | Lesson | null>(null);
	let editModalOpen = $state(false);

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			if (activeTab === 'tracks') {
				const result = await api<Track[]>('/admin/learn/tracks');
				tracks = result.ok ? (result.data ?? []) : [];
			} else if (activeTab === 'modules') {
				const result = await api<Module[]>('/admin/learn/modules');
				modules = result.ok ? (result.data ?? []) : [];
			} else {
				const result = await api<Lesson[]>('/admin/learn/lessons');
				lessons = result.ok ? (result.data ?? []) : [];
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function createTrack() {
		if (!newTrack.title.trim() || creating) return;
		creating = true;
		try {
			const result = await api('/admin/learn/tracks', {
				method: 'POST',
				body: newTrack
			});
			if (result.ok) {
				showCreateModal = false;
				newTrack = { title: '', description: '' };
				await load();
			}
		} catch {
			error = t('common.error');
		} finally {
			creating = false;
		}
	}

	async function createModule() {
		if (!newModule.title.trim() || !newModule.track_id || creating) return;
		creating = true;
		try {
			await api(`/admin/learn/tracks/${newModule.track_id}/modules`, {
				method: 'POST',
				body: { title: newModule.title }
			});
			showCreateModal = false;
			newModule = { track_id: '', title: '' };
			await load();
		} catch {
			error = t('common.error');
		} finally {
			creating = false;
		}
	}

	async function createLesson() {
		if (!newLesson.title.trim() || !newLesson.module_id || creating) return;
		creating = true;
		try {
			await api(`/admin/learn/modules/${newLesson.module_id}/lessons`, {
				method: 'POST',
				body: { title: newLesson.title, body: newLesson.body }
			});
			showCreateModal = false;
			newLesson = { module_id: '', title: '', body: '' };
			await load();
		} catch {
			error = t('common.error');
		} finally {
			creating = false;
		}
	}

	async function publishTrack(id: string) {
		try {
			await api(`/admin/learn/tracks/${id}/publish`, { method: 'POST' });
			await load();
		} catch {
			error = t('common.error');
		}
	}

	async function deleteItem(type: string, id: string) {
		if (!confirm('Delete this item?')) return;
		try {
			if (type === 'track') await api(`/admin/learn/tracks/${id}`, { method: 'DELETE' });
			else if (type === 'module') await api(`/admin/learn/modules/${id}`, { method: 'DELETE' });
			else await api(`/admin/learn/lessons/${id}`, { method: 'DELETE' });
			await load();
		} catch {
			error = t('common.error');
		}
	}
</script>

<div class="p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Learning Management</h1>
		<button
			onclick={() => (showCreateModal = true)}
			class="rounded-lg px-4 py-2 text-sm font-medium text-white"
			style="background: var(--color-primary)"
		>
			Create {activeTab === 'tracks' ? 'Track' : activeTab === 'modules' ? 'Module' : 'Lesson'}
		</button>
	</div>

	<!-- Tabs -->
	<div class="mb-6 flex gap-1 rounded-lg border p-1" style="border-color: var(--color-border)">
		{#each ['tracks', 'modules', 'lessons'] as tab}
			<button
				onclick={() => { activeTab = tab as typeof activeTab; load(); }}
				class="rounded-md px-4 py-2 text-sm font-medium transition-colors"
				style="background: {activeTab === tab ? 'var(--color-primary)' : 'transparent'}; color: {activeTab === tab ? 'white' : 'var(--color-muted)'}"
			>
				{tab.charAt(0).toUpperCase() + tab.slice(1)}
			</button>
		{/each}
	</div>

	{#if error}
		<div class="mb-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700">{error}</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			{#each Array(5) as _}
				<div class="h-16 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
			{/each}
		</div>
	{:else if activeTab === 'tracks'}
		{#if tracks.length === 0}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">No tracks yet</p>
		{:else}
			<div class="space-y-3">
				{#each tracks as track (track.id)}
					<div class="flex items-center justify-between rounded-lg border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
						<div>
							<h3 class="font-medium" style="color: var(--color-foreground)">{track.title}</h3>
							<p class="text-xs" style="color: var(--color-muted)">{track.module_count} modules &middot; {track.enrollment_count} enrollments &middot; {track.status}</p>
						</div>
						<div class="flex gap-2">
							{#if track.status === 'draft'}
								<button onclick={() => publishTrack(track.id)} class="rounded px-3 py-1 text-xs font-medium text-white" style="background: var(--color-primary)">Publish</button>
							{/if}
							<button onclick={() => deleteItem('track', track.id)} class="rounded px-3 py-1 text-xs font-medium" style="color: #ef4444">Delete</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{:else if activeTab === 'modules'}
		{#if modules.length === 0}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">No modules yet</p>
		{:else}
			<div class="space-y-3">
				{#each modules as mod (mod.id)}
					<div class="flex items-center justify-between rounded-lg border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
						<div>
							<h3 class="font-medium" style="color: var(--color-foreground)">{mod.title}</h3>
							<p class="text-xs" style="color: var(--color-muted)">{mod.lesson_count} lessons &middot; Order: {mod.sort_order}</p>
						</div>
						<button onclick={() => deleteItem('module', mod.id)} class="rounded px-3 py-1 text-xs font-medium" style="color: #ef4444">Delete</button>
					</div>
				{/each}
			</div>
		{/if}
	{:else}
		{#if lessons.length === 0}
			<p class="py-8 text-center text-sm" style="color: var(--color-muted)">No lessons yet</p>
		{:else}
			<div class="space-y-3">
				{#each lessons as lesson (lesson.id)}
					<div class="flex items-center justify-between rounded-lg border p-4" style="border-color: var(--color-border); background: var(--color-surface)">
						<div>
							<h3 class="font-medium" style="color: var(--color-foreground)">{lesson.title}</h3>
							<p class="text-xs" style="color: var(--color-muted)">{lesson.duration_minutes} min &middot; Order: {lesson.sort_order}</p>
						</div>
						<button onclick={() => deleteItem('lesson', lesson.id)} class="rounded px-3 py-1 text-xs font-medium" style="color: #ef4444">Delete</button>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</div>

<!-- Create Modal -->
<Modal
	bind:open={showCreateModal}
	title="Create {activeTab === 'tracks' ? 'Track' : activeTab === 'modules' ? 'Module' : 'Lesson'}"
	onConfirm={activeTab === 'tracks' ? createTrack : activeTab === 'modules' ? createModule : createLesson}
	confirmLabel={creating ? 'Creating...' : 'Create'}
>
	{#if activeTab === 'tracks'}
		<div class="space-y-4">
			<div>
				<label for="track-title" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Title</label>
				<input id="track-title" type="text" bind:value={newTrack.title} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)" />
			</div>
			<div>
				<label for="track-description" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Description</label>
				<textarea id="track-description" bind:value={newTrack.description} rows="3" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)"></textarea>
			</div>
		</div>
	{:else if activeTab === 'modules'}
		<div class="space-y-4">
			<div>
				<label for="module-track-id" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Track ID</label>
				<input id="module-track-id" type="text" bind:value={newModule.track_id} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)" />
			</div>
			<div>
				<label for="module-title" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Title</label>
				<input id="module-title" type="text" bind:value={newModule.title} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)" />
			</div>
		</div>
	{:else}
		<div class="space-y-4">
			<div>
				<label for="lesson-module-id" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Module ID</label>
				<input id="lesson-module-id" type="text" bind:value={newLesson.module_id} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)" />
			</div>
			<div>
				<label for="lesson-title" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Title</label>
				<input id="lesson-title" type="text" bind:value={newLesson.title} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)" />
			</div>
			<div>
				<label for="lesson-body" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Body (Markdown)</label>
				<textarea id="lesson-body" bind:value={newLesson.body} rows="6" class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)"></textarea>
			</div>
		</div>
	{/if}
</Modal>
