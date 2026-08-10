<script lang="ts">
	// Creator application — Japa University instructor application form (ADR-063).
	// CSR ONLY: POST /learn/university/instructor/apply.
	// Payload matches backend ApplyInput (internal/university/store.go):
	//   display_name (required), headline, bio, application_note.
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { goto } from '$app/navigation';
	import { GraduationCap, Send } from 'lucide-svelte';

	let displayName = $state('');
	let headline = $state('');
	let bio = $state('');
	let applicationNote = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		if (submitting) return;
		submitting = true;
		error = null;
		const r = await api.post('/learn/university/instructor/apply', {
			display_name: displayName,
			headline: headline || undefined,
			bio: bio || undefined,
			application_note: applicationNote || undefined
		});
		submitting = false;
		if (r.error) {
			error = r.error.message;
		} else {
			goto('/dashboard/instructor');
		}
	}
</script>

<svelte:head>
	<title>{t('university.apply')} · {t('app.name')}</title>
</svelte:head>

<div class="flex justify-center px-4 py-6 md:px-6 lg:px-8">
	<form onsubmit={submit} class="w-full max-w-2xl space-y-5">
		<header class="mb-2">
			<h1 class="flex items-center gap-2 text-2xl font-bold text-[var(--color-text)]">
				<GraduationCap size={24} class="text-[var(--color-primary)]" />
				{t('university.apply')}
			</h1>
			<p class="mt-1 text-sm text-[var(--color-muted)]">
				{t('university.apply_description')}
			</p>
		</header>

		<!-- Display Name (required) -->
		<div>
			<label for="display_name" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.display_name_label')}
			</label>
			<input
				id="display_name"
				type="text"
				bind:value={displayName}
				required
				placeholder={t('university.display_name_placeholder', 'Your public instructor name')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		<!-- Headline -->
		<div>
			<label for="headline" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.headline_label')}
			</label>
			<input
				id="headline"
				type="text"
				bind:value={headline}
				placeholder={t('university.headline_placeholder', 'e.g. Senior Go Engineer & Course Creator')}
				class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			/>
		</div>

		<!-- Bio -->
		<div>
			<label for="bio" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.bio_label')}
			</label>
			<textarea
				id="bio"
				bind:value={bio}
				rows="4"
				placeholder={t('university.bio_apply_placeholder', 'Tell us about your teaching experience and expertise…')}
				class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			></textarea>
		</div>

		<!-- Application Note (portfolio URL, expertise, samples) -->
		<div>
			<label for="application_note" class="mb-1 block text-sm font-medium text-[var(--color-text)]">
				{t('university.application_note_label')}
			</label>
			<textarea
				id="application_note"
				bind:value={applicationNote}
				rows="3"
				placeholder={t('university.application_note_placeholder', 'Portfolio URL, areas of expertise, sample content links…')}
				class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] p-3 text-sm text-[var(--color-text)] placeholder:text-[var(--color-muted)] focus:border-[var(--color-primary)] focus:outline-none"
			></textarea>
		</div>

		{#if error}
			<p class="text-sm text-[var(--color-destructive)]">{error}</p>
		{/if}

		<button
			type="submit"
			disabled={submitting}
			class="inline-flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] hover:bg-[var(--color-primary-hover)] disabled:opacity-60"
		>
			<Send size={16} />
			{submitting ? t('common.loading') : t('university.submit_application')}
		</button>
	</form>
</div>
