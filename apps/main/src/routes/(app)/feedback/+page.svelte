<script lang="ts">
	// Ref: japaconnect-engineering/sections/06-custom-fields-forms.md (public form embed)
	// Public feedback form. Renders the fields from GET /forms/feedback/embed
	// (no hardcoded fields — Rule 16) and submits to POST /forms/{id}/submissions
	// via the browser API client. Auth is optional: an authed user's ID is
	// recorded automatically by the backend.
	import { t, siteName} from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { MessageSquare, Send } from 'lucide-svelte';
	import type { FormField } from '$lib/types';

	let { data } = $props();
	const form = $derived(data.form);

	// Field values keyed by field label (the backend stores field_values as a
	// map keyed by the field label, matching the GetForm handler's contract).
	let values = $state<Record<string, string>>({});
	let loading = $state(false);
	let submitted = $state(false);
	let errorKey = $state('');

	const sortedFields = $derived(
		form ? [...form.fields].sort((a, b) => a.sort_order - b.sort_order) : []
	);

	function inputClass(field: FormField): string {
		const base =
			'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
		return base;
	}

	function validate(): boolean {
		if (!form) return false;
		for (const f of sortedFields) {
			if (f.required && !values[f.label]?.trim()) {
				errorKey = 'feedback.errors.field_required';
				return false;
			}
		}
		return true;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorKey = '';
		if (!form) return;
		if (!validate()) return;
		loading = true;
		try {
			const res = await api.post<{ id: string; status: string }>(
				`/forms/${form.id}/submissions`,
				{ field_values: values }
			);
			if (res.data) {
				submitted = true;
				values = {};
			} else {
				errorKey = 'feedback.errors.submit_failed';
			}
		} catch {
			errorKey = 'feedback.errors.submit_failed';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{t('feedback.title')} — {siteName()}</title>
	<meta name="description" content={t('feedback.subtitle')} />
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<section class="mx-auto max-w-3xl space-y-6 px-4 py-8">
	<header class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
			<MessageSquare class="h-5 w-5" />
		</div>
		<div>
			<h1 class="text-2xl font-semibold text-[var(--color-text)]">
				{form?.name ?? t('feedback.title')}
			</h1>
			<p class="text-sm text-[var(--color-muted)]">
				{form?.description ?? t('feedback.subtitle')}
			</p>
		</div>
	</header>

	{#if !form}
		<EmptyState icon={MessageSquare} title={t('feedback.unavailable')} body={t('feedback.unavailable_body')} />
	{:else if submitted}
		<div class="rounded-[12px] border border-green-500/30 bg-green-500/10 p-6 text-center">
			<p class="text-lg font-medium text-green-600">
				{form.success_message ?? t('feedback.success')}
			</p>
			<button
				type="button"
				onclick={() => (submitted = false)}
				class="mt-4 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
			>
				{t('feedback.submit_another')}
			</button>
		</div>
	{:else}
		<form onsubmit={submit} class="space-y-4 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6" novalidate>
			{#each sortedFields as field (field.id)}
				<div class="flex flex-col gap-1.5">
					<label for="field-{field.id}" class="text-sm font-medium text-[var(--color-text)]">
						{field.label}
						{#if field.required}<span class="text-red-500" aria-hidden="true">*</span>{/if}
					</label>
					{#if field.type === 'textarea' || field.type === 'long_text'}
						<textarea
							id="field-{field.id}"
							rows={5}
							value={values[field.label] ?? ''}
							oninput={(e) => (values = { ...values, [field.label]: e.currentTarget.value })}
							class={inputClass(field)}
						></textarea>
					{:else if field.type === 'select' && field.options && field.options.length > 0}
						<AutocompleteSelect
							id="field-{field.id}"
							value={values[field.label] ?? ''}
							options={[
								{ value: '', label: t('feedback.select_placeholder') },
								...field.options.map((opt) => ({ value: opt, label: opt }))
							]}
							onValueChange={(v) => (values = { ...values, [field.label]: v })}
						/>
					{:else if field.type === 'checkbox'}
						<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
							<input
								type="checkbox"
								id="field-{field.id}"
								checked={values[field.label] === 'true'}
								onchange={(e) => (values = { ...values, [field.label]: e.currentTarget.checked ? 'true' : '' })}
								class="h-4 w-4 rounded border-[var(--color-border)]"
							/>
							{field.label}
						</label>
					{:else}
						<input
							id="field-{field.id}"
							type={field.type === 'email' ? 'email' : field.type === 'number' ? 'number' : 'text'}
							value={values[field.label] ?? ''}
							oninput={(e) => (values = { ...values, [field.label]: e.currentTarget.value })}
							class={inputClass(field)}
						/>
					{/if}
				</div>
			{/each}

			{#if errorKey}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{t(errorKey)}
				</p>
			{/if}

			<div class="flex justify-end pt-2">
				<button
					type="submit"
					disabled={loading}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
				>
					<Send class="h-4 w-4" />
					{loading ? '...' : (form.submit_label || t('feedback.submit'))}
				</button>
			</div>
		</form>
	{/if}
</section>
