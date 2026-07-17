<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { FileText, ArrowRight } from 'lucide-svelte';

	type Form = {
		id: string;
		title: string;
		slug: string;
		space_id?: string;
		submission_count: number;
	};

	let forms = $state<Form[]>([]);
	let loading = $state(true);
	let errorMessage = $state('');

	const slug = $derived(page.params.slug);

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			const allForms = await api.listForms();
			// Filter to forms belonging to this space
			forms = allForms.filter((f: Form) => f.space_id);
			// If space_id filtering isn't available server-side, load all
			if (forms.length === 0) {
				forms = allForms;
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{t('spaces.forms.title') ?? 'Forms'} — {slug} | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<div class="mb-6">
		<a href="/spaces/{slug}" class="text-sm" style="color: var(--color-primary)">&larr; Back to {slug}</a>
		<h1 class="mt-2 text-2xl font-bold" style="color: var(--color-foreground)">
			{t('spaces.forms.title') ?? 'Forms'}
		</h1>
		<p class="mt-1 text-sm" style="color: var(--color-muted)">
			Browse and submit forms in this space.
		</p>
	</div>

	{#if errorMessage}
		<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{errorMessage}
		</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			{#each Array(3) as _}
				<div class="h-20 animate-pulse rounded-lg" style="background-color: var(--color-surface)"></div>
			{/each}
		</div>
	{:else if forms.length === 0}
		<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border)">
			<FileText class="mx-auto mb-3 h-8 w-8" style="color: var(--color-muted)" />
			<p class="text-sm" style="color: var(--color-muted)">{t('spaces.slug.forms.no_forms_available_in_this_space')}</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each forms as form (form.id)}
				<a
					href="/spaces/{slug}/forms/{form.id}"
					class="flex items-center justify-between rounded-lg border p-4 transition-colors hover:bg-black/5 dark:hover:bg-white/5"
					style="border-color: var(--color-border)"
				>
					<div class="flex items-center gap-3">
						<div
							class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg"
							style="background-color: color-mix(in srgb, var(--color-primary) 12%, transparent)"
						>
							<FileText class="h-5 w-5" style="color: var(--color-primary)" />
						</div>
						<div>
							<h3 class="font-medium text-sm" style="color: var(--color-foreground)">{form.title}</h3>
							<p class="text-xs" style="color: var(--color-muted)">
								{form.submission_count} submission{form.submission_count !== 1 ? 's' : ''}
							</p>
						</div>
					</div>
					<ArrowRight class="h-4 w-4" style="color: var(--color-muted)" />
				</a>
			{/each}
		</div>
	{/if}
</main>
