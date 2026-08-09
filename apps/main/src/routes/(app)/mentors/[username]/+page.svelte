// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { ArrowLeft, GraduationCap, Send, CheckCircle2 } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import type { Mentor } from '$lib/types';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const mentor = $derived<Mentor | null>(data.mentor);

	// Request form state.
	let goals = $state('');
	let destinationSpaceId = $state('');
	let submitting = $state(false);
	let submitted = $state(false);
	let errorMsg = $state('');

	const countries = $derived(data.countries ?? []);

	const title = $derived(
		mentor
			? t('mentors.profile_title', '{name} — Mentor').replace('{name}', mentor.username)
			: t('mentors.list_title', 'Find a Mentor — JapaConnect')
	);

	function capacityPct(m: Mentor): number {
		const cap = m.mentor_capacity ?? 0;
		const active = m.active_mentee_count ?? 0;
		if (cap <= 0) return 0;
		return Math.min(100, Math.round((active / cap) * 100));
	}

	function hasCapacity(m: Mentor): boolean {
		const cap = m.mentor_capacity ?? 0;
		const active = m.active_mentee_count ?? 0;
		return cap > 0 && active < cap;
	}

	async function submitRequest(e: SubmitEvent) {
		e.preventDefault();
		errorMsg = '';
		if (!mentor) return;
		if (!goals.trim()) {
			errorMsg = t('mentors.errors.goals_required', 'Please describe your goals.');
			return;
		}
		submitting = true;
		try {
			const res = await api.post<{ id: string; status: string }>('/mentorship/request', {
				mentor_id: mentor.user_id,
				goals: goals.trim(),
				destination_space_id: destinationSpaceId.trim() || null
			});
			if (res.data) {
				submitted = true;
				goals = '';
			} else {
				errorMsg = res.error?.message ?? t('mentors.errors.request_failed', 'Request failed. Please try again.');
			}
		} catch {
			errorMsg = t('mentors.errors.request_failed', 'Request failed. Please try again.');
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="description" content={mentor?.mentor_bio ?? t('mentors.list_description', 'Discover experienced diaspora mentors to guide your journey.')} />
	<meta property="og:title" content={title} />
	<meta property="og:type" content="profile" />
</svelte:head>

<div class="mx-auto w-full max-w-full">
	<!-- Header -->
	<div class="border-b border-[var(--color-border)] px-4 py-3">
		<a
			href="/mentors"
			class="mb-1 inline-flex items-center gap-1.5 text-sm text-[var(--color-text-subtle)] transition-colors hover:text-[var(--color-text)]"
		>
			<ArrowLeft size={15} /> {t('mentors.back', 'Back to mentors')}
		</a>
	</div>

	<div class="p-4">
		{#if data.loadError || !mentor}
			<EmptyState
				icon={GraduationCap}
				title={t('mentors.not_found', 'Mentor not found')}
				body={t('mentors.not_found_body', 'This mentor profile may have been removed.')}
				action={{ label: t('mentors.back', 'Back to mentors'), href: '/mentors' }}
			/>
		{:else}
			<div class="grid gap-6 lg:grid-cols-[1fr_22rem]">
				<!-- Profile -->
				<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
					<div class="flex items-start gap-4">
						{#if mentor.avatar_url}
							<img
								src={mentor.avatar_url}
								alt={mentor.username}
								class="h-16 w-16 rounded-full object-cover"
							/>
						{:else}
							<div class="grid h-16 w-16 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
								<GraduationCap size={28} />
							</div>
						{/if}
						<div class="min-w-0">
							<h1 class="text-xl font-semibold text-[var(--color-text)]">{mentor.username}</h1>
							<div class="mt-1 flex flex-wrap items-center gap-2 text-sm text-[var(--color-muted)]">
								{#if hasCapacity(mentor)}
									<span class="inline-flex items-center rounded-full bg-green-500/10 px-2 py-0.5 text-[10px] font-medium text-green-600">
										{t('mentors.spots_open', 'Spots open')}
									</span>
								{/if}
							</div>
						</div>
					</div>

					{#if mentor.mentor_bio}
						<div class="mt-4">
							<h2 class="text-sm font-medium text-[var(--color-text)]">{t('mentors.bio', 'About')}</h2>
							<p class="mt-1 text-sm text-[var(--color-text-subtle)]">{mentor.mentor_bio}</p>
						</div>
					{/if}

					{#if mentor.mentor_areas && mentor.mentor_areas.length > 0}
						<div class="mt-4">
							<h2 class="text-sm font-medium text-[var(--color-text)]">{t('mentors.areas', 'Mentorship areas')}</h2>
							<div class="mt-1.5 flex flex-wrap gap-1.5">
								{#each mentor.mentor_areas as area (area)}
									<span class="inline-flex items-center rounded-full border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2.5 py-0.5 text-xs font-medium text-[var(--color-text-subtle)]">
										{area}
									</span>
								{/each}
							</div>
						</div>
					{/if}

					<!-- Capacity -->
					<div class="mt-4">
						<h2 class="text-sm font-medium text-[var(--color-text)]">{t('mentors.capacity', 'Mentees')}</h2>
						<div class="mt-1.5 flex items-center gap-2">
							<div class="h-2 w-40 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
								<div
									class="h-full rounded-full bg-[var(--color-primary)]"
									style="width: {capacityPct(mentor)}%"
								></div>
							</div>
							<span class="text-xs text-[var(--color-muted)]">
								{mentor.active_mentee_count ?? 0}/{mentor.mentor_capacity ?? 0}
							</span>
						</div>
					</div>
				</section>

				<!-- Request form -->
				<section class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-6">
					<h2 class="text-base font-semibold text-[var(--color-text)]">
						{t('mentors.request_heading', 'Request Mentorship')}
					</h2>
					<p class="mt-1 text-sm text-[var(--color-text-subtle)]">
						{t('mentors.request_body', 'Share your goals so your mentor can help you plan.')}
					</p>

					{#if !data.authed}
						<a
							href="/login?redirect=/mentors/{mentor.username}"
							class="mt-4 inline-flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)]"
						>
							{t('common.login', 'Log in to request')}
						</a>
					{:else if submitted}
						<div class="mt-4 rounded-[var(--radius)] border border-green-500/30 bg-green-500/10 p-4 text-center">
							<CheckCircle2 class="mx-auto h-6 w-6 text-green-600" />
							<p class="mt-2 text-sm font-medium text-green-600">
								{t('mentors.request_sent', 'Request sent!')}
							</p>
							<button
								type="button"
								onclick={() => (submitted = false)}
								class="mt-3 rounded-[var(--radius)] border border-[var(--color-border)] px-3 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]"
							>
								{t('mentors.request_another', 'Send another')}
							</button>
						</div>
					{:else}
						<form onsubmit={submitRequest} class="mt-4 space-y-3" novalidate>
							<div class="flex flex-col gap-1.5">
								<label for="goals" class="text-sm font-medium text-[var(--color-text)]">
									{t('mentors.field_goals', 'Your goals')}
								</label>
								<textarea
									id="goals"
									rows={4}
									bind:value={goals}
									placeholder={t('mentors.field_goals_placeholder', 'What do you hope to achieve?')}
									class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
								></textarea>
							</div>

							<div class="flex flex-col gap-1.5">
								<label for="dest" class="text-sm font-medium text-[var(--color-text)]">
									{t('mentors.field_destination', 'Destination country')}
								</label>
								<select
									id="dest"
									bind:value={destinationSpaceId}
									class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20"
								>
									<option value="">{t('common.select', 'Select...')}</option>
									{#each countries as country (country.id)}
										<option value={country.id}>{country.name}</option>
									{/each}
								</select>
							</div>

							{#if errorMsg}
								<p class="rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
									{errorMsg}
								</p>
							{/if}

							<button
								type="submit"
								disabled={submitting}
								class="inline-flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
							>
								<Send size={16} />
								{submitting ? '...' : t('mentors.send_request', 'Send request')}
							</button>
						</form>
					{/if}
				</section>
			</div>
		{/if}
	</div>
</div>
