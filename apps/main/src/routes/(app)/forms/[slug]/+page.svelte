<script lang="ts">
	// Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
	// Generic public form renderer for {SITE_URL}/forms/{slug}. Renders the
	// fields from GET /forms/{slug}/embed (no hardcoded fields — Rule 16) and
	// submits to POST /forms/{id}/submissions. Multi-step forms render one
	// step at a time; each step's fields are submitted together.
	import { t, siteName } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import AutocompleteSelect from '$lib/components/ui/AutocompleteSelect.svelte';
	import { FileText, Send, ChevronLeft, ChevronRight } from 'lucide-svelte';
	import type { FormField } from '$lib/types';

	let { data } = $props();
	const form = $derived(data.form);

	// Field values keyed by field key when present, else label — the backend
	// accepts both and the machine key survives label edits.
	let values = $state<Record<string, string>>({});
	let loading = $state(false);
	let submitted = $state(false);
	let stepIndex = $state(0);
	let errorMsg = $state('');

	type Step = { id: string; title: string; position: number; fields: FormField[] };

	const isMultiStep = $derived(!!form?.is_multi_step && (form?.steps?.length ?? 0) > 0);
	const steps = $derived<Step[]>(
		isMultiStep ? [...(form!.steps ?? [])].sort((a, b) => a.position - b.position) : []
	);
	const sortedFields = $derived<FormField[]>(
		isMultiStep
			? [...(steps[stepIndex]?.fields ?? [])].sort((a, b) => a.sort_order - b.sort_order)
			: form
				? [...form.fields].sort((a, b) => a.sort_order - b.sort_order)
				: []
	);

	function fieldKey(field: FormField): string {
		return field.key || field.label;
	}

	function inputClass(): string {
		return 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
	}

	function validateStep(): boolean {
		for (const f of sortedFields) {
			if (f.required && !values[fieldKey(f)]?.trim()) {
				errorMsg = t('feedback.errors.field_required');
				return false;
			}
		}
		return true;
	}

	function nextStep() {
		errorMsg = '';
		if (!validateStep()) return;
		stepIndex += 1;
	}

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		if (!form) return;
		if (!validateStep()) return;
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
				errorMsg = t('feedback.errors.submit_failed');
			}
		} catch {
			errorMsg = t('feedback.errors.submit_failed');
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>{form?.name ?? t('feedback.title')} — {siteName()}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<section class="mx-auto max-w-3xl space-y-6 px-4 py-8">
	<header class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-primary)]/10 text-[var(--color-primary)]">
			<FileText class="h-5 w-5" />
		</div>
		<div>
			<h1 class="text-2xl font-semibold text-[var(--color-text)]">{form?.name ?? t('feedback.title')}</h1>
			{#if form?.description}<p class="text-sm text-[var(--color-muted)]">{form.description}</p>{/if}
		</div>
	</header>

	{#if !form}
		<EmptyState icon={FileText} title={t('feedback.unavailable')} body={t('feedback.unavailable_body')} />
	{:else if submitted}
		<div class="rounded-[12px] border border-green-500/30 bg-green-500/10 p-6 text-center">
			<p class="text-lg font-medium text-green-600">{form.success_message ?? t('feedback.success')}</p>
		</div>
	{:else}
		{#if isMultiStep}
			<ol class="flex items-center gap-2 text-sm text-[var(--color-muted)]">
				{#each steps as s, i (s.id)}
					<li class={i === stepIndex ? 'font-medium text-[var(--color-text)]' : ''}>
						{s.title}{#if i < steps.length - 1}<span class="mx-2">→</span>{/if}
					</li>
				{/each}
			</ol>
		{/if}

		<form onsubmit={submit} class="space-y-4 rounded-[12px] border border-[var(--color-border)] bg-[var(--color-surface)] p-6" novalidate>
			{#each sortedFields as field (field.id)}
				<div class="flex flex-col gap-1.5">
					<label for="field-{field.id}" class="text-sm font-medium text-[var(--color-text)]">
						{field.label}
						{#if field.required}<span class="text-red-500" aria-hidden="true">*</span>{/if}
					</label>
					{#if field.type === 'textarea' || field.type === 'long_text'}
						<textarea id="field-{field.id}" rows={5} placeholder={field.placeholder ?? ''}
							value={values[fieldKey(field)] ?? ''}
							oninput={(e) => (values = { ...values, [fieldKey(field)]: e.currentTarget.value })}
							class={inputClass()}></textarea>
					{:else if field.type === 'select' && field.options && field.options.length > 0}
						<AutocompleteSelect
							id="field-{field.id}"
							value={values[fieldKey(field)] ?? ''}
							options={[
								{ value: '', label: t('feedback.select_placeholder') },
								...field.options.map((opt) => ({ value: opt, label: opt }))
							]}
							onValueChange={(v) => (values = { ...values, [fieldKey(field)]: v })}
						/>
					{:else if field.type === 'multiselect' && field.options && field.options.length > 0}
						<div class="flex flex-col gap-1.5" role="group" aria-labelledby="field-{field.id}-label">
							{#each field.options as opt}
								<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
									<input type="checkbox" class="h-4 w-4 rounded border-[var(--color-border)]"
										checked={(values[fieldKey(field)] ?? '').split(',').includes(opt)}
										onchange={(e) => {
											const cur = (values[fieldKey(field)] ?? '').split(',').filter(Boolean);
											const next = e.currentTarget.checked ? [...cur, opt] : cur.filter((v) => v !== opt);
											values = { ...values, [fieldKey(field)]: next.join(',') };
										}} />
									{opt}
								</label>
							{/each}
						</div>
					{:else if field.type === 'boolean' || field.type === 'checkbox'}
						<label class="flex items-center gap-2 text-sm text-[var(--color-text)]">
							<input type="checkbox" id="field-{field.id}"
								checked={values[fieldKey(field)] === 'true'}
								onchange={(e) => (values = { ...values, [fieldKey(field)]: e.currentTarget.checked ? 'true' : '' })}
								class="h-4 w-4 rounded border-[var(--color-border)]" />
							{field.placeholder ?? field.label}
						</label>
					{:else}
						<!-- Media/image/file types render as a URL input on the
							 anonymous embed page — the real upload flow needs auth
							 and lives on the bespoke pages. -->
						<input id="field-{field.id}" placeholder={field.placeholder ?? ''}
							type={
								field.type === 'email' ? 'email'
								: field.type === 'number' || field.type === 'currency' || field.type === 'percentage' || field.type === 'rating' ? 'number'
								: field.type === 'url' || field.type === 'media' || field.type === 'image' || field.type === 'image_single' || field.type === 'file' || field.type === 'document' ? 'url'
								: field.type === 'phone' ? 'tel'
								: field.type === 'date' ? 'date'
								: field.type === 'datetime' ? 'datetime-local'
								: field.type === 'time' ? 'time'
								: 'text'
							}
							value={values[fieldKey(field)] ?? field.default_value ?? ''}
							oninput={(e) => (values = { ...values, [fieldKey(field)]: e.currentTarget.value })}
							class={inputClass()} />
						{#if ['media', 'image', 'image_single', 'file', 'document'].includes(field.type)}
							<p class="text-xs text-[var(--color-muted)]">{t('forms.media_url_hint', 'Paste a link to the file')}</p>
						{/if}
					{/if}
				</div>
			{/each}

			{#if errorMsg}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">{errorMsg}</p>
			{/if}

			<div class="flex justify-between pt-2">
				{#if isMultiStep && stepIndex > 0}
					<button type="button" onclick={() => (stepIndex -= 1)}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]">
						<ChevronLeft class="h-4 w-4" />{t('common.back')}
					</button>
				{:else}
					<span></span>
				{/if}
				{#if isMultiStep && stepIndex < steps.length - 1}
					<button type="button" onclick={nextStep}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90">
						{t('common.next')}<ChevronRight class="h-4 w-4" />
					</button>
				{:else}
					<button type="submit" disabled={loading}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50">
						<Send class="h-4 w-4 rtl:-scale-x-100" />
						{loading ? '...' : (form.submit_label || t('feedback.submit'))}
					</button>
				{/if}
			</div>
		</form>
	{/if}
</section>
