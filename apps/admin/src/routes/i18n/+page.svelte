<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import Modal from '$lib/components/Modal.svelte';

	type Language = {
		code: string;
		name: string;
		is_default: boolean;
	};

	type TranslationString = {
		id: string;
		key: string;
		value: string;
		language_code: string;
	};

	let languages = $state<Language[]>([]);
	let strings = $state<TranslationString[]>([]);
	let loading = $state(true);
	let error = $state('');
	let selectedLang = $state('en');
	let searchQuery = $state('');

	let showCreateModal = $state(false);
	let creating = $state(false);
	let newString = $state({ key: '', value: '', language_code: 'en' });

	let showTranslateModal = $state(false);
	let translating = $state(false);
	let translateTarget = $state('');
	let translateResult = $state('');

	onMount(load);

	async function load() {
		loading = true;
		error = '';
		try {
			const [langResult, strResult] = await Promise.all([
				api<Language[]>('/admin/i18n/languages'),
				api<TranslationString[]>(`/admin/i18n/strings?language_code=${selectedLang}`)
			]);
			languages = langResult.ok ? (langResult.data ?? []) : [];
			strings = strResult.ok ? (strResult.data ?? []) : [];
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function createString() {
		if (!newString.key.trim() || creating) return;
		creating = true;
		try {
			await api('/admin/i18n/strings', {
				method: 'POST',
				body: newString
			});
			showCreateModal = false;
			newString = { key: '', value: '', language_code: 'en' };
			await load();
		} catch {
			error = t('common.error');
		} finally {
			creating = false;
		}
	}

	async function deleteString(id: string) {
		if (!confirm('Delete this translation string?')) return;
		try {
			await api(`/admin/i18n/strings/${id}`, { method: 'DELETE' });
			await load();
		} catch {
			error = t('common.error');
		}
	}

	async function aiTranslate() {
		if (!translateTarget || translating) return;
		translating = true;
		translateResult = '';
		try {
			const result = await api('/admin/i18n/strings/ai-translate-batch', {
				method: 'POST',
				body: { target_language: translateTarget }
			});
			translateResult = result.ok ? 'Translation completed' : 'Translation failed';
			await load();
		} catch {
			translateResult = 'Translation failed';
		} finally {
			translating = false;
		}
	}

	let filteredStrings = $derived(strings.filter(s =>
		!searchQuery || s.key.toLowerCase().includes(searchQuery.toLowerCase()) || s.value.toLowerCase().includes(searchQuery.toLowerCase())
	));
</script>

<div class="p-6">
	<div class="mb-6 flex items-center justify-between">
		<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">Translations</h1>
		<div class="flex gap-2">
			<button
				onclick={() => { showTranslateModal = true; }}
				class="rounded-lg border px-4 py-2 text-sm font-medium"
				style="border-color: var(--color-border); color: var(--color-foreground)"
			>
				AI Translate
			</button>
			<button
				onclick={() => (showCreateModal = true)}
				class="rounded-lg px-4 py-2 text-sm font-medium text-white"
				style="background: var(--color-primary)"
			>
				Add String
			</button>
		</div>
	</div>

	<!-- Filters -->
	<div class="mb-4 flex items-center gap-3">
		<select
			bind:value={selectedLang}
			onchange={() => load()}
			class="rounded-lg border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			{#each languages as lang (lang.code)}
				<option value={lang.code}>{lang.name} ({lang.code})</option>
			{/each}
		</select>
		<input
			type="text"
			bind:value={searchQuery}
			placeholder="Search keys or values..."
			class="flex-1 rounded-lg border px-3 py-2 text-sm"
			style="border-color: var(--color-border); background: var(--color-surface)"
		/>
	</div>

	{#if error}
		<div class="mb-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700">{error}</div>
	{/if}

	{#if loading}
		<div class="space-y-2">
			{#each Array(10) as _}
				<div class="h-12 animate-pulse rounded-lg" style="background: var(--color-surface)"></div>
			{/each}
		</div>
	{:else if filteredStrings.length === 0}
		<p class="py-8 text-center text-sm" style="color: var(--color-muted)">No translation strings found</p>
	{:else}
		<div class="rounded-lg border overflow-hidden" style="border-color: var(--color-border)">
			<table class="w-full text-sm">
				<thead>
					<tr style="background: var(--color-surface)">
						<th class="px-4 py-2 text-left font-medium" style="color: var(--color-muted)">Key</th>
						<th class="px-4 py-2 text-left font-medium" style="color: var(--color-muted)">Value</th>
						<th class="px-4 py-2 text-right font-medium" style="color: var(--color-muted)">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each filteredStrings as s (s.id)}
						<tr style="border-top: 1px solid var(--color-border)">
							<td class="px-4 py-2 font-mono text-xs" style="color: var(--color-foreground)">{s.key}</td>
							<td class="px-4 py-2 text-sm" style="color: var(--color-foreground)">{s.value}</td>
							<td class="px-4 py-2 text-right">
								<button onclick={() => deleteString(s.id)} class="text-xs" style="color: #ef4444">Delete</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<!-- Create Modal -->
<Modal
	bind:open={showCreateModal}
	title="Add Translation String"
	onConfirm={createString}
	confirmLabel={creating ? 'Creating...' : 'Create'}
>
	<div class="space-y-4">
		<div>
			<label for="i18n-key" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Key</label>
			<input id="i18n-key" type="text" bind:value={newString.key} placeholder="e.g., blog.title" class="w-full rounded-lg border px-3 py-2 text-sm font-mono" style="border-color: var(--color-border); background: var(--color-surface)" />
		</div>
		<div>
			<label for="i18n-value" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Value</label>
			<input id="i18n-value" type="text" bind:value={newString.value} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)" />
		</div>
		<div>
			<label for="i18n-lang" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Language</label>
			<select id="i18n-lang" bind:value={newString.language_code} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)">
				{#each languages as lang (lang.code)}
					<option value={lang.code}>{lang.name}</option>
				{/each}
			</select>
		</div>
	</div>
</Modal>

<!-- AI Translate Modal -->
<Modal
	bind:open={showTranslateModal}
	title="AI Translate Batch"
	onConfirm={aiTranslate}
	confirmLabel={translating ? 'Translating...' : 'Translate'}
>
	<div class="space-y-4">
		<p class="text-sm" style="color: var(--color-muted)">Automatically translate all strings to the target language using AI.</p>
		<div>
			<label for="translate-target" class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">Target Language</label>
			<select id="translate-target" bind:value={translateTarget} class="w-full rounded-lg border px-3 py-2 text-sm" style="border-color: var(--color-border); background: var(--color-surface)">
				{#each languages as lang (lang.code)}
					<option value={lang.code}>{lang.name}</option>
				{/each}
			</select>
		</div>
		{#if translateResult}
			<p class="text-sm font-medium" style="color: {translateResult.includes('completed') ? 'var(--color-success, #16a34a)' : '#ef4444'}">{translateResult}</p>
		{/if}
	</div>
</Modal>
