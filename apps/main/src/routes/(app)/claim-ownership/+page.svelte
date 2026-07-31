<script lang="ts">
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { enhance } from '$app/forms';
	import { ShieldQuestion, Loader2, Send } from 'lucide-svelte';
	import type { ActionData, PageData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();

	const org = $derived(data.org);

	let evidenceUrl = $state('');
	let notes = $state('');
	let submitting = $state(false);

	const pageTitle = $derived(t('claim_ownership.title', 'Claim ownership — JapaConnect'));
	const pageDescription = $derived(
		t('claim_ownership.description', 'Submit evidence to claim this organization.')
	);

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]';
</script>

<svelte:head>
	<title>{pageTitle}</title>
	<meta name="description" content={pageDescription} />
	<meta property="og:title" content={pageTitle} />
	<meta property="og:description" content={pageDescription} />
	<meta property="og:type" content="website" />
	<meta property="og:url" content={page.url.href} />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<h1 class="flex items-center gap-2 text-lg font-semibold text-[var(--color-text)]">
			<ShieldQuestion size={20} />
			{t('claim_ownership.heading', 'Claim ownership')}
		</h1>
		<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
			{t('claim_ownership.subtitle', 'Submit evidence to claim this organization.')}
		</p>
	</div>

	<div class="p-3">
		{#if form?.message}
			<div
				class="mb-4 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-3 py-2 text-sm text-[var(--color-text-subtle)]"
				role="alert"
			>
				{form.message}
			</div>
		{/if}

		<!-- Org context card -->
		<div
			class="mb-4 flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<span
				class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)]"
			>
				<ShieldQuestion size={18} />
			</span>
			<div class="min-w-0">
				<p class="truncate text-sm font-medium text-[var(--color-text)]">{org.name}</p>
				<p class="truncate text-xs text-[var(--color-muted)]">/{org.slug}</p>
			</div>
		</div>

		<form
			method="POST"
			use:enhance={() => {
				submitting = true;
				return async ({ update }) => {
					await update();
					submitting = false;
				};
			}}
			class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4"
		>
			<input type="hidden" name="organization_id" value={org.id} />
			<input type="hidden" name="org_slug" value={org.slug} />

			<div class="space-y-4">
				<div>
					<label
						for="evidence_url"
						class="mb-1 block text-sm font-medium text-[var(--color-text)]"
					>
						{t('claim_ownership.field_evidence', 'Evidence URL')}
					</label>
					<input
						id="evidence_url"
						name="evidence_url"
						type="url"
						bind:value={evidenceUrl}
						placeholder="https://example.com/proof"
						class={inputClass}
					/>
					<p class="mt-1 text-xs text-[var(--color-muted)]">
						{t('claim_ownership.evidence_hint', 'A link to documentation proving your authority over this organization.')}
					</p>
				</div>

				<div>
					<label
						for="notes"
						class="mb-1 block text-sm font-medium text-[var(--color-text)]"
					>
						{t('claim_ownership.field_notes', 'Notes')}
					</label>
					<textarea
						id="notes"
						name="notes"
						rows="4"
						bind:value={notes}
						placeholder={t('claim_ownership.notes_placeholder', 'Explain your relationship to this organization…')}
						class={inputClass}
					></textarea>
				</div>

				<div class="flex items-center justify-between gap-3">
					<a
						href="/org/{org.slug}"
						class="text-sm text-[var(--color-muted)] hover:underline"
					>
						← {t('claim_ownership.back', 'Back to organization')}
					</a>
					<button
						type="submit"
						disabled={submitting}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
					>
						{#if submitting}
							<Loader2 size={16} class="animate-spin" />
						{:else}
							<Send size={16} />
						{/if}
						{t('claim_ownership.submit', 'Submit claim')}
					</button>
				</div>
			</div>
		</form>
	</div>
</div>
