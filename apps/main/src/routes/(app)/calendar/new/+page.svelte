<script lang="ts">
	// Ref: japaconnect-engineering/sections/86-community-calendar.md
	import { t } from '$lib/i18n';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { ArrowLeft, Send, Loader2, Lock } from 'lucide-svelte';
	import EmptyState from '$lib/components/ui/EmptyState.svelte';
	import type { CalendarCategory } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Auth is required to submit. The app layout exposes `user` in page.data.
	const authed = $derived(!!page.data.user);

	// Schema-defined category enum (Rule 16 — no hardcoded data arrays).
	const categories: { value: CalendarCategory; label: string }[] = [
		{ value: 'visa_window', label: t('calendar.cat_visa_window', 'Visa window') },
		{ value: 'embassy_closure', label: t('calendar.cat_embassy_closure', 'Embassy closure') },
		{ value: 'application_deadline', label: t('calendar.cat_application_deadline', 'Application deadline') },
		{ value: 'exam_window', label: t('calendar.cat_exam_window', 'Exam window') },
		{ value: 'immigration_office', label: t('calendar.cat_immigration_office', 'Immigration office') },
		{ value: 'other', label: t('calendar.cat_other', 'Other') }
	];

	let title = $state('');
	let description = $state('');
	let category = $state<CalendarCategory>('visa_window');
	let destinationSpaceId = $state('');
	let originSpaceId = $state('');
	let occursAt = $state('');
	let endsAt = $state('');
	let isRecurring = $state(false);
	let recurrencePattern = $state('');
	let sourceUrl = $state('');
	let submitting = $state(false);
	let errorMsg = $state('');

	const inputClass =
		'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';

	function toIso(value: string): string | null {
		if (!value) return null;
		// datetime-local values are already ISO-ish (no timezone); send as-is.
		const d = new Date(value);
		return isNaN(d.getTime()) ? null : d.toISOString();
	}

	const canSubmit = $derived(
		title.trim().length > 0 &&
			!!sourceUrl.trim() &&
			!!occursAt &&
			destinationSpaceId.length > 0 &&
			!submitting
	);

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		if (!authed) {
			goto('/login?redirect=/calendar/new');
			return;
		}
		if (!canSubmit) {
			errorMsg = t('calendar.new.error_required', 'Title, start time, and a verifiable source URL are required.');
			return;
		}
		// source_url is REQUIRED and must be a verifiable official source.
		try {
			new URL(sourceUrl.trim());
		} catch {
			errorMsg = t('calendar.new.error_invalid_url', 'Please enter a valid source URL.');
			return;
		}

		submitting = true;
		try {
			const body = {
				title: title.trim(),
				description: description.trim() || undefined,
				category,
				destination_space_id: destinationSpaceId,
				origin_space_id: originSpaceId || undefined,
				occurs_at: toIso(occursAt),
				ends_at: endsAt ? toIso(endsAt) : undefined,
				is_recurring: isRecurring,
				recurrence_pattern: isRecurring ? recurrencePattern.trim() || undefined : undefined,
				source_url: sourceUrl.trim()
			};
			const res = await api.post<{ id: string }>('/calendar', body);
			if (res.data && res.data.id) {
				await goto(`/calendar/${res.data.id}`);
			} else {
				errorMsg = res.error?.message ?? t('calendar.new.error_failed', 'Failed to submit event. Please try again.');
			}
		} catch {
			errorMsg = t('calendar.new.error_failed', 'Failed to submit event. Please try again.');
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>{t('calendar.new_title', 'Submit Event — JapaConnect')}</title>
	<meta name="description" content={t('calendar.new_description', 'Submit a community calendar event with a verifiable official source.')} />
	<meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-2xl">
	<div class="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/calendar"
			class="grid h-8 w-8 place-items-center rounded-[var(--radius)] text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('calendar.back', 'Back to calendar')}
		>
			<ArrowLeft size={18} />
		</a>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">{t('calendar.new_heading', 'Submit Event')}</h1>
	</div>

	{#if !authed}
		<div class="p-4">
			<EmptyState
				icon={Lock}
				title={t('calendar.new.login_required', 'Login required')}
				body={t('calendar.new.login_required_body', 'You must be signed in to submit a calendar event.')}
				action={{ label: t('auth.sign_in', 'Sign in'), href: '/login?redirect=/calendar/new' }}
			/>
		</div>
	{:else}
		<form onsubmit={submit} class="space-y-5 p-4" novalidate>
			{#if errorMsg}
				<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
					{errorMsg}
				</p>
			{/if}

			<!-- Title -->
			<div>
				<label for="title" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_title', 'Title')} <span class="text-red-500" aria-hidden="true">*</span>
				</label>
				<input
					id="title"
					type="text"
					bind:value={title}
					maxlength="160"
					placeholder={t('calendar.new.placeholder_title', 'e.g. UK visa application window opens')}
					class={inputClass}
				/>
			</div>

			<!-- Category -->
			<div>
				<label for="category" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_category', 'Category')}
				</label>
				<select id="category" bind:value={category} class={inputClass}>
					{#each categories as cat (cat.value)}
						<option value={cat.value}>{cat.label}</option>
					{/each}
				</select>
			</div>

			<!-- Description -->
			<div>
				<label for="description" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_description', 'Description')}
				</label>
				<textarea
					id="description"
					bind:value={description}
					rows="5"
					placeholder={t('calendar.new.placeholder_description', 'Add details about this event…')}
					class={inputClass}
				></textarea>
			</div>

			<!-- Destination / Origin -->
			<div class="grid gap-4 sm:grid-cols-2">
				<div>
					<label for="destination_country" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_destination', 'Destination country')} <span class="text-red-500" aria-hidden="true">*</span>
					</label>
					<select id="destination_country" bind:value={destinationSpaceId} class={inputClass}>
						<option value="">{t('calendar.new.placeholder_destination', 'Select a country')}</option>
						{#each data.countries as country (country.id)}
							<option value={country.id}>{country.name}</option>
						{/each}
					</select>
				</div>
				<div>
					<label for="origin_country" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_origin', 'Origin country (optional)')}
					</label>
					<select id="origin_country" bind:value={originSpaceId} class={inputClass}>
						<option value="">{t('calendar.new.placeholder_origin', 'Leave blank for all origins')}</option>
						{#each data.countries as country (country.id)}
							<option value={country.id}>{country.name}</option>
						{/each}
					</select>
				</div>
			</div>

			<!-- Dates -->
			<div class="grid gap-4 sm:grid-cols-2">
				<div>
					<label for="occurs_at" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_occurs_at', 'Starts')} <span class="text-red-500" aria-hidden="true">*</span>
					</label>
					<input id="occurs_at" type="datetime-local" bind:value={occursAt} class={inputClass} />
				</div>
				<div>
					<label for="ends_at" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
						{t('calendar.new.label_ends_at', 'Ends (optional)')}
					</label>
					<input id="ends_at" type="datetime-local" bind:value={endsAt} class={inputClass} />
				</div>
			</div>

			<!-- Recurrence -->
			<div class="space-y-2">
				<label class="flex cursor-pointer items-center gap-2 text-sm text-[var(--color-text)]">
					<input type="checkbox" bind:checked={isRecurring} class="h-4 w-4 accent-[var(--color-primary)]" />
					{t('calendar.new.label_recurring', 'Recurring event')}
				</label>
				{#if isRecurring}
					<input
						type="text"
						bind:value={recurrencePattern}
						placeholder={t('calendar.new.placeholder_recurrence', 'e.g. annual')}
						class={inputClass}
					/>
				{/if}
			</div>

			<!-- Source URL (REQUIRED) -->
			<div>
				<label for="source_url" class="mb-1.5 block text-sm font-medium text-[var(--color-text)]">
					{t('calendar.new.label_source', 'Official source URL')} <span class="text-red-500" aria-hidden="true">*</span>
				</label>
				<input
					id="source_url"
					type="url"
					bind:value={sourceUrl}
					placeholder={t('calendar.new.placeholder_source', 'https://official-source.example/…')}
					class={inputClass}
				/>
				<p class="mt-1 text-xs text-[var(--color-muted)]">
					{t('calendar.new.source_help', 'Must link to a verifiable official source.')}
				</p>
			</div>

			<!-- Submit -->
			<div class="flex items-center justify-end gap-2 border-t border-[var(--color-border)] pt-4">
				<a
					href="/calendar"
					class="rounded-[var(--radius)] border border-[var(--color-border)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)]"
				>
					{t('common.cancel', 'Cancel')}
				</a>
				<button
					type="submit"
					disabled={!canSubmit}
					class="inline-flex items-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{#if submitting}
						<Loader2 size={16} class="animate-spin" />
					{:else}
						<Send size={16} />
					{/if}
					{t('calendar.new.submit', 'Submit event')}
				</button>
			</div>
		</form>
	{/if}
</div>
