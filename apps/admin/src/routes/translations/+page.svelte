<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Plus, Trash2, Search, Languages } from 'lucide-svelte';

	interface Translation {
		id: string;
		locale: string;
		key: string;
		value: string;
		context: string;
		created_at: string;
		updated_at: string;
	}

	interface LocaleCount {
		locale: string;
		count: number;
	}

	let locales: LocaleCount[] = $state([]);
	let selectedLocale = $state('en');
	let translations: Translation[] = $state([]);
	let loading = $state(true);
	let searchQuery = $state('');
	let showAddModal = $state(false);
	let editingId = $state<string | null>(null);
	let editValue = $state('');
	let deleteConfirmId = $state<string | null>(null);

	let newKey = $state('');
	let newValue = $state('');
	let newContext = $state('label');
	let adding = $state(false);

	const contextOptions = [
		'error',
		'label',
		'heading',
		'button',
		'description',
		'placeholder',
		'title',
		'nav'
	];

	let filteredTranslations = $derived(
		searchQuery
			? translations.filter(
					(t) =>
						t.key.toLowerCase().includes(searchQuery.toLowerCase()) ||
						t.value.toLowerCase().includes(searchQuery.toLowerCase())
			  )
			: translations
	);

	async function loadLocales() {
		const result = await api<LocaleCount[]>('/admin/translations/locales');
		if (result.ok) {
			locales = result.data;
			if (locales.length > 0 && !locales.find((l) => l.locale === selectedLocale)) {
				selectedLocale = locales[0].locale;
			}
		}
	}

	async function loadTranslations() {
		loading = true;
		const result = await api<Translation[]>(`/admin/translations?locale=${selectedLocale}`);
		if (result.ok) {
			translations = result.data;
		}
		loading = false;
	}

	async function addTranslation() {
		if (!newKey.trim() || !newValue.trim()) return;
		adding = true;
		const result = await api<Translation>('/admin/translations', {
			method: 'POST',
			body: {
				locale: selectedLocale,
				key: newKey.trim(),
				value: newValue.trim(),
				context: newContext
			}
		});
		if (result.ok) {
			translations = [...translations, result.data];
			newKey = '';
			newValue = '';
			newContext = 'label';
			showAddModal = false;
			await loadLocales();
		}
		adding = false;
	}

	async function saveEdit(id: string) {
		const t = translations.find((t) => t.id === id);
		if (!t) return;
		const result = await api<Translation>('/admin/translations', {
			method: 'POST',
			body: { locale: t.locale, key: t.key, value: editValue, context: t.context }
		});
		if (result.ok) {
			translations = translations.map((t) => (t.id === id ? { ...t, value: editValue } : t));
			editingId = null;
		}
	}

	async function deleteTranslation(id: string) {
		const result = await api<{ deleted: boolean }>(`/admin/translations/${id}`, {
			method: 'DELETE'
		});
		if (result.ok) {
			translations = translations.filter((t) => t.id !== id);
			deleteConfirmId = null;
			await loadLocales();
		}
	}

	onMount(async () => {
		await loadLocales();
		await loadTranslations();
	});

	$effect(() => {
		selectedLocale;
		loadTranslations();
	});
</script>

<svelte:head>
	<title>Translations — JapaAdmin</title>
</svelte:head>

<div class="space-y-6">
	<!-- Header -->
	<div class="flex items-center justify-between">
		<div class="flex items-center gap-3">
			<Languages class="h-6 w-6" style="color: var(--color-primary)" />
			<h1 class="text-2xl font-bold tracking-tight">{t('translations.title')}</h1>
		</div>
		<button
			class="flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
			style="background-color: var(--color-primary)"
			onclick={() => (showAddModal = true)}
		>
			<Plus class="h-4 w-4" />
			{t('translations.add')}
		</button>
	</div>

	<!-- Locale selector + search -->
	<div class="flex items-center gap-4">
		<div class="flex items-center gap-2">
			<label for="locale-select" class="text-sm font-medium">{t('translations.locale')}</label>
			<select
				id="locale-select"
				class="rounded-md border px-3 py-1.5 text-sm"
				style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-foreground)"
				bind:value={selectedLocale}
			>
				{#each locales as loc}
					<option value={loc.locale}>{loc.locale.toUpperCase()} ({loc.count})</option>
				{/each}
				{#if locales.length === 0}
					<option value="en">EN</option>
				{/if}
			</select>
		</div>
		<div class="relative flex-1 max-w-md">
			<Search
				class="absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2"
				style="color: var(--color-muted)"
			/>
			<input
				type="text"
				placeholder={t('translations.search')}
				class="w-full rounded-md border py-1.5 pl-9 pr-3 text-sm outline-none"
				style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-foreground)"
				bind:value={searchQuery}
			/>
		</div>
		<span class="text-sm" style="color: var(--color-muted)">
			{filteredTranslations.length} translation{filteredTranslations.length !== 1 ? 's' : ''}
		</span>
	</div>

	<!-- Translations table -->
	<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
		<table class="w-full text-sm">
			<thead>
				<tr style="border-color: var(--color-border)">
					<th
						class="px-4 py-3 text-left font-medium"
						style="border-color: var(--color-border); background-color: var(--color-surface)"
					>
						{t('translations.key')}
					</th>
					<th
						class="px-4 py-3 text-left font-medium"
						style="border-color: var(--color-border); background-color: var(--color-surface)"
					>
						{t('translations.value')}
					</th>
					<th
						class="px-4 py-3 text-left font-medium"
						style="border-color: var(--color-border); background-color: var(--color-surface)"
					>
						{t('translations.context')}
					</th>
					<th
						class="px-4 py-3 text-right font-medium"
						style="border-color: var(--color-border); background-color: var(--color-surface)"
					>
						{t('translations.actions')}
					</th>
				</tr>
			</thead>
			<tbody>
				{#if loading}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('translations.loading')}
						</td>
					</tr>
				{:else if filteredTranslations.length === 0}
					<tr>
						<td colspan="4" class="px-4 py-8 text-center" style="color: var(--color-muted)">
							{t('translations.empty')}
						</td>
					</tr>
				{:else}
					{#each filteredTranslations as t (t.id)}
						<tr
							class="border-t"
							style="border-color: var(--color-border)"
						>
							<td
								class="px-4 py-3 font-mono text-xs"
								style="border-color: var(--color-border)"
							>
								{t.key}
							</td>
							<td class="px-4 py-3" style="border-color: var(--color-border)">
								{#if editingId === t.id}
									<input
										class="w-full rounded border px-2 py-1 text-sm"
										style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-foreground)"
										bind:value={editValue}
										onkeydown={(e) => {
											if (e.key === 'Enter') saveEdit(t.id);
											if (e.key === 'Escape') editingId = null;
										}}
									/>
								{:else}
									<button
										class="text-left hover:underline"
										onclick={() => {
											editingId = t.id;
											editValue = t.value;
										}}
									>
										{t.value}
									</button>
								{/if}
							</td>
							<td class="px-4 py-3" style="border-color: var(--color-border)">
								<span
									class="inline-block rounded px-2 py-0.5 text-xs font-medium"
									style="background-color: var(--color-surface); color: var(--color-muted)"
								>
									{t.context || '\u2014'}
								</span>
							</td>
							<td class="px-4 py-3 text-right" style="border-color: var(--color-border)">
								{#if editingId === t.id}
									<button
										class="text-sm font-medium hover:underline"
										style="color: var(--color-primary)"
										onclick={() => saveEdit(t.id)}
									>
									{t('common.save')}
								</button>
								<button
									class="ml-2 text-sm hover:underline"
									style="color: var(--color-muted)"
									onclick={() => (editingId = null)}
								>
									{t('common.cancel')}
									</button>
								{:else if deleteConfirmId === t.id}
									<span class="text-sm" style="color: var(--color-danger)">{t('translations.delete_confirm')}</span>
									<button
										class="ml-2 text-sm font-medium hover:underline"
										style="color: var(--color-danger)"
										onclick={() => deleteTranslation(t.id)}
									>
										Yes
									</button>
									<button
										class="ml-2 text-sm hover:underline"
										style="color: var(--color-muted)"
										onclick={() => (deleteConfirmId = null)}
									>
										No
									</button>
								{:else}
									<button
										class="p-1 hover:opacity-75"
										onclick={() => (deleteConfirmId = t.id)}
										aria-label={t('translations.delete_aria')}
									>
										<Trash2 class="h-4 w-4" style="color: var(--color-danger)" />
									</button>
								{/if}
							</td>
						</tr>
					{/each}
				{/if}
			</tbody>
		</table>
	</div>
</div>

<!-- Add Translation Modal -->
{#if showAddModal}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
		<div
			class="w-full max-w-lg rounded-lg border p-6 shadow-xl"
			style="background-color: var(--color-background); border-color: var(--color-border)"
		>
			<h2 class="mb-4 text-lg font-semibold">{t('translations.add_title')}</h2>
			<form
				onsubmit={(e) => {
					e.preventDefault();
					addTranslation();
				}}
				class="space-y-4"
			>
				<div>
					<label for="add-locale" class="mb-1 block text-sm font-medium">Locale</label>
					<input
						id="add-locale"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-foreground)"
						value={selectedLocale}
						disabled
					/>
				</div>
				<div>
					<label for="add-key" class="mb-1 block text-sm font-medium">{t('translations.key')}</label>
					<input
						id="add-key"
						class="w-full rounded-md border px-3 py-2 text-sm font-mono"
						style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-foreground)"
						placeholder="e.g. nav.home"
						bind:value={newKey}
						required
					/>
				</div>
				<div>
					<label for="add-value" class="mb-1 block text-sm font-medium">{t('translations.value')}</label>
					<input
						id="add-value"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-foreground)"
						placeholder="e.g. Home"
						bind:value={newValue}
						required
					/>
				</div>
				<div>
					<label for="add-context" class="mb-1 block text-sm font-medium">{t('translations.context')}</label>
					<select
						id="add-context"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-foreground)"
						bind:value={newContext}
					>
						{#each contextOptions as ctx}
							<option value={ctx}>{ctx}</option>
						{/each}
					</select>
				</div>
				<div class="flex justify-end gap-3 pt-2">
					<button
						type="button"
						class="rounded-md border px-4 py-2 text-sm font-medium"
						style="border-color: var(--color-border); color: var(--color-foreground)"
						onclick={() => (showAddModal = false)}
					>
						{t('common.cancel')}
					</button>
					<button
						type="submit"
						class="rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:opacity-50"
						style="background-color: var(--color-primary)"
						disabled={adding || !newKey.trim() || !newValue.trim()}
					>
						{adding ? t('translations.adding') : t('translations.add_btn')}
					</button>
				</div>
			</form>
		</div>
	</div>
{/if}
