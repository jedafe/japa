<script lang="ts">
	import { aiApi, personaVoicesAdminApi, type GenerateResult, type PersonaVoice } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Loader2, Copy, Check, Sparkles, RotateCcw, Send, ChevronDown, ChevronUp, Clock, Zap, Trash2, AlertTriangle } from 'lucide-svelte';
	import { hasPermission } from '$lib/stores/permissions.svelte';
	import { Select } from '$lib/components/ui/select';
	import Autocomplete, { type AutocompleteItem } from '$lib/components/ui/Autocomplete.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const canGenerate = $derived(hasPermission('ai.generate') && data?.enabled !== false);

	const isFeatureEnabled = $derived(data?.enabled !== false);

	// ─── Form State ───────────────────────────────────────────────
	let prompt = $state('');
	let category = $state('blog_post');
	let model = $state('');
	let voiceID = $state('');
	let generating = $state(false);
	let result = $state<GenerateResult | null>(null);
	let error = $state('');
	let copied = $state(false);
	let showAdvanced = $state(false);
	let sentToReview = $state(false);
	let reviewMessage = $state('');

	// ─── Humanization ─────────────────────────────────────────────
	// Per-request override. The backend merges this with the global
	// ai.humanize.* system setting (gated by the ai.humanize.manage RBAC node).
	let humanize = $state(false);
	let humanizeLevel = $state<'low' | 'medium' | 'high'>('medium');
	const humanizeLevelOptions = [
		{ value: 'low', labelKey: 'ai.humanize.level.low' },
		{ value: 'medium', labelKey: 'ai.humanize.level.medium' },
		{ value: 'high', labelKey: 'ai.humanize.level.high' }
	];

	// ─── History ──────────────────────────────────────────────────
	type HistoryEntry = {
		id: string;
		prompt: string;
		category: string;
		result: GenerateResult;
		createdAt: string;
	};
	let history = $state<HistoryEntry[]>([]);

	// Rule 16: categories come from the backend (/api/v1/ai/categories).
	// No hardcoded fallback — if the API fails the selector shows a
	// "failed to load" message instead of invented options.
	let categories = $state<{ value: string; labelKey: string; desc: string }[]>([]);
	let categoriesError = $state(false);

	let models = $state<{ value: string; label: string }[]>([
		{ value: '', label: 'Default' }
	]);

	// Rule 16: persona voices come from the backend. No hardcoded options.
	let voices = $state<{ value: string; label: string }[]>([
		{ value: '', label: t('ai.voice_default', 'Default (no voice override)') }
	]);

	$effect(() => {
		(async () => {
			// Rule 16: models come from the backend (/admin/ai/models), which
			// derives them from the providers actually registered with the
			// dispatcher. No hardcoded model fallback — if the API fails the
			// single "Default" entry (set at init) remains.
			try {
				const res = await aiApi.listModels();
				if (res.ok && res.data.length > 0) {
					models = res.data.map((m) => ({ value: m.id, label: m.label }));
				}
			} catch {
				// keep the initial "Default"-only list; do not invent models.
			}
			try {
				const voicesRes = await personaVoicesAdminApi.list();
				if (voicesRes.ok && voicesRes.data && Array.isArray(voicesRes.data.data)) {
					voices = [
						{ value: '', label: t('ai.voice_default', 'Default (no voice override)') },
						...voicesRes.data.data.map((v: PersonaVoice) => ({ value: v.id, label: v.name }))
					];
				}
			} catch {
				// keep the default-only list.
			}
			try {
				const catsRes = await aiApi.listCategories();
				if (catsRes.ok && Array.isArray(catsRes.data) && catsRes.data.length > 0) {
					categories = catsRes.data.map((c: { value: string; label: string }) => ({
						value: c.value,
						labelKey: `ai.category.${c.value}`,
						desc: c.label
					}));
					categoriesError = false;
				} else {
					categoriesError = true;
				}
			} catch {
				categoriesError = true;
			}
		})();
	});

	// ─── Autocomplete fetchers (client-side filter over cached lists) ───
	// The /ai/categories and /admin/ai/models endpoints return full lists
	// without server-side search, so we fetch once (above) and filter
	// client-side in the fetcher — the same pattern SpaceAutocomplete uses.
	function categoryFetcher(q: string): Promise<AutocompleteItem[]> {
		const ql = q.trim().toLowerCase();
		const items = categories.map((c) => ({
			id: c.value,
			label: t(c.labelKey),
			sublabel: c.desc
		}));
		const filtered = ql
			? items.filter((i) => i.label.toLowerCase().includes(ql) || i.id.toLowerCase().includes(ql))
			: items;
		return Promise.resolve(filtered.slice(0, 50));
	}

	function categoryInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(
			categories.map((c) => ({ id: c.value, label: t(c.labelKey), sublabel: c.desc })).slice(0, 50)
		);
	}

	function modelFetcher(q: string): Promise<AutocompleteItem[]> {
		const ql = q.trim().toLowerCase();
		const items = models.map((m) => ({ id: m.value, label: m.label }));
		const filtered = ql
			? items.filter((i) => i.label.toLowerCase().includes(ql) || i.id.toLowerCase().includes(ql))
			: items;
		return Promise.resolve(filtered.slice(0, 50));
	}

	function modelInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(models.map((m) => ({ id: m.value, label: m.label })).slice(0, 50));
	}

	function voiceFetcher(q: string): Promise<AutocompleteItem[]> {
		const ql = q.trim().toLowerCase();
		const items = voices.map((v) => ({ id: v.value, label: v.label }));
		const filtered = ql
			? items.filter((i) => i.label.toLowerCase().includes(ql) || i.id.toLowerCase().includes(ql))
			: items;
		return Promise.resolve(filtered.slice(0, 50));
	}

	function voiceInitialFetcher(): Promise<AutocompleteItem[]> {
		return Promise.resolve(voices.map((v) => ({ id: v.value, label: v.label })).slice(0, 50));
	}

	// Selected labels for the Autocomplete chip display.
	let categoryLabel = $derived(
		categories.find((c) => c.value === category) ? t(categories.find((c) => c.value === category)!.labelKey) : category
	);
	let modelLabel = $derived(models.find((m) => m.value === model)?.label ?? '');
	let voiceLabel = $derived(voices.find((v) => v.value === voiceID)?.label ?? '');

	let charCount = $derived(prompt.length);

	async function handleGenerate() {
		if (!prompt.trim()) return;
		generating = true;
		error = '';
		result = null;
		sentToReview = false;
		reviewMessage = '';
		try {
			const res = await aiApi.generate({
				prompt: prompt.trim(),
				category,
				model: model || undefined,
				voice_id: voiceID || undefined,
				humanize,
				humanize_level: humanize ? humanizeLevel : undefined
			});
			if (res.ok) {
				result = res.data;
				history = [
					{
						id: crypto.randomUUID(),
						prompt: prompt.trim(),
						category,
						result: res.data,
						createdAt: new Date().toISOString()
					},
					...history.slice(0, 19)
				];
			} else {
				error = res.error;
			}
		} catch {
			error = t('ai.generate_error');
		} finally {
			generating = false;
		}
	}

	async function copyResult() {
		if (!result) return;
		await navigator.clipboard.writeText(result.generated_text);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	async function sendToReview() {
		if (!result) return;
		reviewMessage = '';
		try {
			const res = await aiApi.createHitlReview({
				content: result.generated_text,
				content_type: category,
				source: 'admin_generator'
			});
			if (res.ok) {
				sentToReview = true;
				reviewMessage = t('ai.sent_to_review');
			} else {
				reviewMessage = res.error;
			}
		} catch {
			reviewMessage = t('ai.review_error');
		}
	}

	function clearHistory() {
		history = [];
	}

	function loadFromHistory(entry: HistoryEntry) {
		prompt = entry.prompt;
		category = entry.category;
		result = entry.result;
	}
</script>

<svelte:head>
	<title>AI Generator — JapaConnect Admin</title>
</svelte:head>

<div class="mb-6">
	<h1 class="mb-1 text-2xl font-bold tracking-tight">{t('ai.generator')}</h1>
	<p class="text-sm" style="color: var(--color-muted)">{t('ai.generator_description')}</p>
</div>

{#if !isFeatureEnabled}
	<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="flex items-center gap-3 text-sm" style="color: var(--color-muted)">
			<AlertTriangle class="h-5 w-5" />
			{t('ai.feature_disabled', 'AI content generation is currently disabled for this tenant.')}
		</div>
	</div>
{:else if !canGenerate}
	<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
		<div class="flex items-center gap-3 text-sm" style="color: var(--color-muted)">
			<AlertTriangle class="h-5 w-5" />
			You don't have permission to use AI generation.
		</div>
	</div>
{:else}

<div class="grid gap-6 lg:grid-cols-3">
	<!-- ═══ Main Generator ═══ -->
	<div class="lg:col-span-2">
		<div class="rounded-lg border p-6" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="space-y-4">
				<!-- Prompt -->
				<div>
					<div class="mb-1 flex items-center justify-between">
						<label for="gen-prompt" class="text-sm font-medium">{t('ai.prompt')}</label>
						<span class="text-xs" style="color: var(--color-muted)">{charCount} chars</span>
					</div>
					<textarea
						id="gen-prompt"
						class="w-full rounded-md border px-3 py-2 text-sm"
						style="border-color: var(--color-border); background-color: var(--color-background)"
						rows="6"
						placeholder={t('ai.prompt_placeholder')}
						bind:value={prompt}
					></textarea>
				</div>

				<!-- Category, Model & Voice -->
				<div class="grid gap-4 sm:grid-cols-3">
					<div>
						<label for="gen-category" class="mb-1 block text-sm font-medium">{t('ai.category')}</label>
						{#if categoriesError}
							<p class="text-sm" style="color: var(--color-danger)">
								{t('ai.categories_load_failed', 'Failed to load categories. Please retry.')}
							</p>
						{:else}
							<Autocomplete
								id="gen-category"
								placeholder={t('ai.category_search', 'Search category...')}
								emptyMessage={t('ai.no_categories', 'No categories found')}
								fetcher={categoryFetcher}
								initialFetcher={categoryInitialFetcher}
								selectedId={category}
								selectedLabel={categoryLabel}
								onSelect={(item) => (category = item.id)}
								onClear={() => (category = 'blog_post')}
							/>
							<p class="mt-1 text-xs" style="color: var(--color-muted)">
								{categories.find(c => c.value === category)?.desc}
							</p>
						{/if}
					</div>
					<div>
						<label for="gen-model" class="mb-1 block text-sm font-medium">{t('ai.model_optional')}</label>
						<Autocomplete
							id="gen-model"
							placeholder={t('ai.model_search', 'Search model...')}
							emptyMessage={t('ai.no_models', 'No models found')}
							fetcher={modelFetcher}
							initialFetcher={modelInitialFetcher}
							selectedId={model}
							selectedLabel={modelLabel}
							onSelect={(item) => (model = item.id)}
							onClear={() => (model = '')}
							/>
						</div>
					<div>
						<label for="gen-voice" class="mb-1 block text-sm font-medium">{t('ai.voice_optional', 'Voice / Tone')}</label>
						<Autocomplete
							id="gen-voice"
							placeholder={t('ai.voice_search', 'Search voice...')}
							emptyMessage={t('ai.no_voices', 'No voices found')}
							fetcher={voiceFetcher}
							initialFetcher={voiceInitialFetcher}
							selectedId={voiceID}
							selectedLabel={voiceLabel}
							onSelect={(item) => (voiceID = item.id)}
							onClear={() => (voiceID = '')}
						/>
					</div>
				</div>

				<!-- Advanced Options -->
				<div>
					<button
						class="flex items-center gap-1.5 text-xs font-medium transition-colors hover:opacity-80"
						style="color: var(--color-muted)"
						onclick={() => (showAdvanced = !showAdvanced)}
					>
						{#if showAdvanced}
							<ChevronUp class="h-3 w-3" />
						{:else}
							<ChevronDown class="h-3 w-3" />
						{/if}
						{t('ai.advanced_options')}
					</button>
					{#if showAdvanced}
						<div class="mt-3 rounded-md border p-3" style="border-color: var(--color-border); background-color: var(--color-background)">
							<!-- Humanization toggle (per-request override; merged with global ai.humanize.* setting) -->
							<div class="flex items-center gap-2">
								<input type="checkbox" id="gen-humanize" bind:checked={humanize} />
								<label for="gen-humanize" class="text-sm font-medium">{t('ai.humanize.label')}</label>
							</div>
							<p class="ml-6 mt-1 text-xs" style="color: var(--color-muted)">{t('ai.humanize.description')}</p>
							{#if humanize}
								<div class="ml-6 mt-3">
									<label for="gen-humanize-level" class="mb-1 block text-xs font-medium">{t('ai.humanize.level.label')}</label>
									<Select
										options={humanizeLevelOptions.map(o => ({ value: o.value, label: t(o.labelKey) }))}
										bind:value={humanizeLevel}
									/>
								</div>
							{/if}
						</div>
					{/if}
				</div>

				<!-- Actions -->
				<div class="flex items-center gap-3 pt-2">
					<button
						class="btn btn-primary disabled:opacity-50"
						disabled={generating || !prompt.trim()}
						onclick={handleGenerate}
					>
						{#if generating}
							<Loader2 class="h-4 w-4 animate-spin" />
							{t('ai.generating')}
						{:else}
							<Sparkles class="h-4 w-4" />
							{t('ai.generate')}
						{/if}
					</button>
					{#if result}
						<button
							class="btn btn-secondary"
							onclick={handleGenerate}
						>
							<RotateCcw class="h-4 w-4" />
							{t('ai.regenerate')}
						</button>
					{/if}
				</div>

				{#if error}
					<div class="rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
						{error}
					</div>
				{/if}

				<!-- Result Card -->
				{#if result}
					<div class="animate-scale-in rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-background)">
						<div class="mb-3 flex items-center justify-between">
							<h3 class="text-sm font-semibold">{t('ai.result')}</h3>
							<button
								class="flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
								style="border-color: var(--color-border)"
								onclick={copyResult}
							>
								{#if copied}
									<Check class="h-3 w-3" style="color: var(--color-success)" />
									{t('ai.copied')}
								{:else}
									<Copy class="h-3 w-3" />
									{t('ai.copy')}
								{/if}
							</button>
						</div>

						<div class="mb-4 whitespace-pre-wrap rounded-md p-4 text-sm leading-relaxed" style="background-color: var(--color-surface)">
							{result.generated_text}
						</div>

						<div class="mb-4 flex flex-wrap gap-4 text-xs" style="color: var(--color-muted)">
							<span class="flex items-center gap-1">
								<Zap class="h-3 w-3" />
								{t('ai.model_used')}: <strong>{result.model}</strong>
							</span>
							<span class="flex items-center gap-1">
								<Clock class="h-3 w-3" />
								{t('ai.tokens_used')}: <strong>{result.tokens_used}</strong>
							</span>
						</div>

						<div class="flex items-center gap-3 border-t pt-3" style="border-color: var(--color-border)">
							<button
								class="flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium text-white transition-colors disabled:opacity-50"
								style="background-color: var(--color-primary)"
								disabled={sentToReview}
								onclick={sendToReview}
							>
								<Send class="h-3 w-3" />
								{sentToReview ? t('ai.sent_to_review') : t('ai.send_to_review')}
							</button>
							{#if reviewMessage}
								<span class="text-xs" style="color: {sentToReview ? 'var(--color-success)' : 'var(--color-danger)'}">{reviewMessage}</span>
							{/if}
						</div>
					</div>
				{/if}
			</div>
		</div>
	</div>

	<!-- ═══ History Sidebar ═══ -->
	<div>
		<div class="rounded-lg border p-4" style="border-color: var(--color-border); background-color: var(--color-surface)">
			<div class="mb-3 flex items-center justify-between">
				<h3 class="text-sm font-semibold">{t('ai.history')}</h3>
				{#if history.length > 0}
					<button
						class="flex items-center gap-1 text-xs transition-colors hover:opacity-80"
						style="color: var(--color-muted)"
						onclick={clearHistory}
					>
						<Trash2 class="h-3 w-3" />
						{t('ai.clear_history')}
					</button>
				{/if}
			</div>

			{#if history.length === 0}
				<p class="py-6 text-center text-xs" style="color: var(--color-muted)">{t('ai.history_empty')}</p>
			{:else}
				<div class="space-y-2 max-h-[600px] overflow-y-auto">
					{#each history as entry (entry.id)}
						<button
							class="w-full rounded-md border p-3 text-left transition-colors hover:bg-black/5 dark:hover:bg-surface/10"
							style="border-color: var(--color-border); background-color: var(--color-background)"
							onclick={() => loadFromHistory(entry)}
						>
							<p class="mb-1 truncate text-xs font-medium">{entry.prompt}</p>
							<div class="flex items-center justify-between text-xs" style="color: var(--color-muted)">
								<span class="rounded-full bg-purple-500/15 px-1.5 py-0.5 text-purple-700 dark:text-purple-400">
									{entry.category}
								</span>
								<span>{entry.result.tokens_used} tokens</span>
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</div>
{/if}
