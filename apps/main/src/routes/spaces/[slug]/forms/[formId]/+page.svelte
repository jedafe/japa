<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { t } from '$lib/stores/i18n.svelte';
	import { ArrowLeft, Send, CheckCircle } from 'lucide-svelte';

	type FormField = {
		id: string;
		key: string;
		label: string;
		field_type: string;
		required: boolean;
		options?: string[];
		placeholder?: string;
		description?: string;
	};

	type FormData = {
		id: string;
		title: string;
		slug: string;
		fields: FormField[];
	};

	const slug = $derived(page.params.slug);
	const formId = $derived((page.params as Record<string, string>).formId);

	let form = $state<FormData | null>(null);
	let loading = $state(true);
	let errorMessage = $state('');
	let submitting = $state(false);
	let submitted = $state(false);
	let values = $state<Record<string, unknown>>({});

	async function load() {
		loading = true;
		errorMessage = '';
		try {
			form = await api.getForm(formId);
			// Initialize values
			if (form?.fields) {
				for (const field of form.fields) {
					values[field.key] = '';
				}
			}
		} catch {
			errorMessage = t('common.error');
		} finally {
			loading = false;
		}
	}

	async function submitForm(e: Event) {
		e.preventDefault();
		if (!form || submitting) return;

		// Validate required fields
		for (const field of form.fields) {
			if (field.required && !values[field.key]) {
				errorMessage = `"${field.label}" is required.`;
				return;
			}
		}

		submitting = true;
		errorMessage = '';
		try {
			await api.submitForm(form.id, { values });
			submitted = true;
		} catch (err) {
			errorMessage = err instanceof Error ? err.message : t('common.error');
		} finally {
			submitting = false;
		}
	}

	onMount(load);
</script>

<svelte:head>
	<title>{form?.title ?? 'Form'} — {slug} | JapaConnect</title>
</svelte:head>

<main class="mx-auto max-w-2xl px-4 py-8">
	<div class="mb-6">
		<a href="/spaces/{slug}/forms" class="flex items-center gap-1 text-sm" style="color: var(--color-primary)">
			<ArrowLeft class="h-4 w-4" />
			Back to forms
		</a>
	</div>

	{#if errorMessage}
		<div class="mb-4 rounded-md border px-4 py-3 text-sm" style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 8%, transparent); color: var(--color-danger)">
			{errorMessage}
		</div>
	{/if}

	{#if loading}
		<div class="space-y-4">
			{#each Array(4) as _}
				<div class="h-16 animate-pulse rounded-lg" style="background-color: var(--color-surface)"></div>
			{/each}
		</div>
	{:else if submitted}
		<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border)">
			<CheckCircle class="mx-auto mb-3 h-10 w-10 text-green-500" />
			<h2 class="text-lg font-semibold" style="color: var(--color-foreground)">{t('spaces.slug.forms.formId.submission_received')}</h2>
			<p class="mt-2 text-sm" style="color: var(--color-muted)">{t('spaces.slug.forms.formId.thank_you_for_your_submission')}</p>
			<div class="mt-4 flex justify-center gap-3">
				<a href="/spaces/{slug}/forms" class="rounded-lg border px-4 py-2 text-sm font-medium" style="border-color: var(--color-border); color: var(--color-foreground)">
					Back to forms
				</a>
				<button onclick={() => { submitted = false; values = {}; }} class="rounded-lg px-4 py-2 text-sm font-medium text-white" style="background: var(--color-primary)">
					Submit another
				</button>
			</div>
		</div>
	{:else if form}
		<h1 class="mb-6 text-2xl font-bold" style="color: var(--color-foreground)">{form.title}</h1>

		<form onsubmit={submitForm} class="space-y-5">
			{#each form.fields as field (field.id)}
				<div>
					<label for={field.key} class="mb-1 block text-sm font-medium" style="color: var(--color-foreground)">
						{field.label}
						{#if field.required}<span class="text-red-500">*</span>{/if}
					</label>
					{#if field.description}
						<p class="mb-1 text-xs" style="color: var(--color-muted)">{field.description}</p>
					{/if}

					{#if field.field_type === 'text' || field.field_type === 'email' || field.field_type === 'number' || field.field_type === 'url'}
						<input
							id={field.key}
							type={field.field_type === 'text' ? 'text' : field.field_type}
							placeholder={field.placeholder}
							bind:value={values[field.key]}
							required={field.required}
							class="w-full rounded-lg border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
						/>
					{:else if field.field_type === 'textarea'}
						<textarea
							id={field.key}
							placeholder={field.placeholder}
							bind:value={values[field.key]}
							required={field.required}
							rows="4"
							class="w-full rounded-lg border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
						></textarea>
					{:else if field.field_type === 'select' && field.options}
						<select
							id={field.key}
							bind:value={values[field.key]}
							required={field.required}
							class="w-full rounded-lg border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
						>
							<option value="">{t('spaces.slug.forms.formId.select')}</option>
							{#each field.options as option}
								<option value={option}>{option}</option>
							{/each}
						</select>
					{:else if field.field_type === 'checkbox'}
						<label class="flex items-center gap-2">
							<input type="checkbox" checked={!!values[field.key]} onchange={(e) => { values[field.key] = (e.target as HTMLInputElement).checked; }} class="rounded" />
							<span class="text-sm" style="color: var(--color-foreground)">{field.placeholder ?? 'Yes'}</span>
						</label>
					{:else}
						<input
							id={field.key}
							type="text"
							placeholder={field.placeholder}
							bind:value={values[field.key]}
							required={field.required}
							class="w-full rounded-lg border px-3 py-2 text-sm"
							style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground)"
						/>
					{/if}
				</div>
			{/each}

			<button
				type="submit"
				disabled={submitting}
				class="flex items-center gap-2 rounded-lg px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
				style="background: var(--color-primary)"
			>
				<Send class="h-4 w-4" />
				{submitting ? 'Submitting...' : 'Submit'}
			</button>
		</form>
	{/if}
</main>
