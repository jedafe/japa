<script lang="ts">
	import { aiApi, type GenerateResult } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { Loader2, Copy, Check, Sparkles, RotateCcw, Send, ChevronDown, ChevronUp, Clock, Zap, Trash2 } from 'lucide-svelte';

	// ─── Form State ───────────────────────────────────────────────
	let prompt = $state('');
	let category = $state('blog_post');
	let model = $state('');
	let generating = $state(false);
	let result = $state<GenerateResult | null>(null);
	let error = $state('');
	let copied = $state(false);
	let showAdvanced = $state(false);
	let sentToReview = $state(false);
	let reviewMessage = $state('');

	// ─── History ──────────────────────────────────────────────────
	type HistoryEntry = {
		id: string;
		prompt: string;
		category: string;
		result: GenerateResult;
		createdAt: string;
	};
	let history = $state<HistoryEntry[]>([]);

	const categories = [
		{ value: 'blog_post', labelKey: 'ai.category.blog_post', desc: 'Long-form articles and blog posts' },
		{ value: 'forum_reply', labelKey: 'ai.category.forum_reply', desc: 'Contextual replies for forum discussions' },
		{ value: 'social_media', labelKey: 'ai.category.social_media', desc: 'Short posts for social platforms' },
		{ value: 'email', labelKey: 'ai.category.email', desc: 'Professional and transactional emails' },
		{ value: 'summary', labelKey: 'ai.category.summary', desc: 'Condensed summaries of longer content' },
		{ value: 'translation', labelKey: 'ai.category.translation', desc: 'Translate content between languages' }
	];

	const models: { value: string; label: string }[] = [
		{ value: '', label: 'Default' },
		{ value: 'gpt-4', label: t('ai.model.gpt4') },
		{ value: 'gpt-3.5-turbo', label: t('ai.model.gpt35') },
		{ value: 'claude-3', label: t('ai.model.claude3') }
	];

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
				model: model || undefined
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

				<!-- Category & Model -->
				<div class="grid gap-4 sm:grid-cols-2">
					<div>
						<label for="gen-category" class="mb-1 block text-sm font-medium">{t('ai.category')}</label>
						<select
							id="gen-category"
							class="w-full rounded-md border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background-color: var(--color-background)"
							bind:value={category}
						>
							{#each categories as cat}
								<option value={cat.value}>{t(cat.labelKey)}</option>
							{/each}
						</select>
						<p class="mt-1 text-xs" style="color: var(--color-muted)">
							{categories.find(c => c.value === category)?.desc}
						</p>
					</div>
					<div>
						<label for="gen-model" class="mb-1 block text-sm font-medium">{t('ai.model_optional')}</label>
						<select
							id="gen-model"
							class="w-full rounded-md border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background-color: var(--color-background)"
							bind:value={model}
						>
							{#each models as m}
								<option value={m.value}>{m.label}</option>
							{/each}
						</select>
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
							<p class="text-xs" style="color: var(--color-muted)">Advanced parameters (temperature, max tokens) will be available when the backend supports them.</p>
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
								class="flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium transition-colors hover:bg-black/5 dark:hover:bg-white/10"
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
							class="w-full rounded-md border p-3 text-left transition-colors hover:bg-black/5 dark:hover:bg-white/10"
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
