<script lang="ts">
	import { onMount } from 'svelte';
	import { Code, Save, Loader2, AlertTriangle, CheckCircle2 } from 'lucide-svelte';
	import { jsonLdApi } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';

	let templatesJson = $state('');
	let loading = $state(true);
	let saving = $state(false);
	let errorMessage = $state('');
	let successMessage = $state('');
	let parseError = $state('');

	onMount(async () => {
		await loadTemplates();
	});

	async function loadTemplates() {
		loading = true;
		errorMessage = '';
		const result = await jsonLdApi.getTemplates();
		if (result.ok) {
			templatesJson = JSON.stringify(result.data, null, 2);
		} else {
			errorMessage = result.error;
		}
		loading = false;
	}

	function validateJson(): Record<string, unknown> | null {
		parseError = '';
		if (!templatesJson.trim()) {
			parseError = 'JSON cannot be empty.';
			return null;
		}
		try {
			const parsed = JSON.parse(templatesJson);
			if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
				parseError = 'JSON must be an object (not an array or primitive).';
				return null;
			}
			return parsed;
		} catch (e) {
			parseError = e instanceof Error ? e.message : 'Invalid JSON';
			return null;
		}
	}

	async function saveTemplates() {
		const parsed = validateJson();
		if (!parsed) return;

		saving = true;
		successMessage = '';
		errorMessage = '';
		const result = await jsonLdApi.updateTemplates(parsed);
		if (result.ok) {
			successMessage = 'Templates saved successfully.';
			setTimeout(() => (successMessage = ''), 3000);
		} else {
			errorMessage = result.error;
		}
		saving = false;
	}

	let placeholderText = '{"Article": { ... }}';

	function handleInput() {
		successMessage = '';
		if (templatesJson.trim()) {
			validateJson();
		} else {
			parseError = '';
		}
	}
</script>

<svelte:head>
	<title>JSON-LD Templates | JapaAdmin</title>
</svelte:head>

<div class="space-y-6">
	<header>
		<div class="flex items-center gap-3">
			<div
				class="flex h-10 w-10 items-center justify-center rounded-xl"
				style="background: color-mix(in srgb, var(--color-primary) 12%, transparent)"
			>
				<Code class="h-5 w-5" style="color: var(--color-primary)" />
			</div>
			<div>
				<h1 class="text-2xl font-bold" style="color: var(--color-foreground)">
					{t('jsonld.title') || 'JSON-LD Templates'}
				</h1>
				<p class="text-sm" style="color: var(--color-muted)">
					{t('jsonld.description') || 'Manage structured data templates for content SEO.'}
				</p>
			</div>
		</div>
	</header>

	{#if errorMessage}
		<div
			class="flex items-center gap-2 rounded-lg border p-3 text-sm"
			style="border-color: color-mix(in srgb, var(--color-danger) 30%, transparent); background: color-mix(in srgb, var(--color-danger) 6%, transparent); color: var(--color-danger)"
		>
			<AlertTriangle class="h-4 w-4 flex-shrink-0" />
			{errorMessage}
		</div>
	{/if}

	{#if successMessage}
		<div
			class="flex items-center gap-2 rounded-lg border p-3 text-sm"
			style="border-color: color-mix(in srgb, #22c55e 30%, transparent); background: color-mix(in srgb, #22c55e 6%, transparent); color: #16a34a"
		>
			<CheckCircle2 class="h-4 w-4 flex-shrink-0" />
			{successMessage}
		</div>
	{/if}

	{#if loading}
		<div
			class="h-96 animate-pulse rounded-xl"
			style="background-color: var(--color-surface)"
		></div>
	{:else}
		<div
			class="rounded-xl border p-4"
			style="border-color: var(--color-border); background: var(--color-surface)"
		>
			<textarea
				bind:value={templatesJson}
				oninput={handleInput}
				class="h-96 w-full resize-y rounded-lg border px-3 py-2 font-mono text-sm outline-none transition-colors focus:ring-2"
				style="border-color: {parseError ? 'var(--color-danger)' : 'var(--color-border)'}; background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary)"
				spellcheck="false"
				placeholder={placeholderText}
			></textarea>

			{#if parseError}
				<p class="mt-2 text-xs" style="color: var(--color-danger)">
					{parseError}
				</p>
			{/if}

			<div class="mt-4 flex items-center justify-end">
				<button
					class="flex items-center gap-2 rounded-lg px-4 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
					style="background-color: var(--color-primary)"
					disabled={saving || !!parseError}
					onclick={saveTemplates}
				>
					{#if saving}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						<Save class="h-4 w-4" />
					{/if}
					{saving ? (t('common.saving') || 'Saving...') : (t('common.save') || 'Save')}
				</button>
			</div>
		</div>
	{/if}
</div>
