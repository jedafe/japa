<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api/client';
	import { t } from '$lib/i18n';
	import { Check, ChevronRight, Sparkles, MapPin } from 'lucide-svelte';
	import type { PageData } from './$types';
	import type { OnboardingRecommendation, TagSuggestion } from '$lib/types';

	let { data }: { data: PageData } = $props();

	// Start at the backend's current step (or step 1).
	let currentStep = $state(untrack(() => data.status?.current_step ?? 1));
	let saving = $state(false);
	let error = $state<string | null>(null);

	// Onboarding minimums from system_settings (onboarding.min_tags_follow etc.)
	// exposed via GET /site/settings/public. Ref: conventions/system-settings-catalog.md
	const minTags = $derived(Number(data.siteSettings?.['onboarding.min_tags_follow'] ?? 3));
	const minUsers = $derived(Number(data.siteSettings?.['onboarding.min_users_follow'] ?? 2));
	const minSpaces = $derived(Number(data.siteSettings?.['onboarding.min_spaces_join'] ?? 2));

	// Step 1: Core Identity — pre-fill location from GeoIP (ADR-037).
	let name = $state('');
	let japaStatus = $state('');
	let currentLocation = $state(
		untrack(() => {
			const g = data.geoContext;
			if (g?.is_valid && g.city) return `${g.city}, ${g.country_name}`;
			if (g?.is_valid && g.country_name) return g.country_name;
			return '';
		})
	);
	// Country selector sourced from /geo/countries (ADR-005). Empty = no
	// selection (user can still free-text the location field).
	let selectedCountry = $state(untrack(() => data.geoContext?.country_code ?? ''));

	// Step 2: Destination & Goals
	let destination = $state('');
	let primaryGoal = $state('');

	// Step 3: Interests/Tags
	let selectedTags = $state<string[]>([]);

	// Step 4: Recommended Spaces & Connections
	let selectedRecs = $state<string[]>([]);

	const totalSteps = 5;
	const stepLabels = $derived([
		t('onboarding.step1', 'Welcome'),
		t('onboarding.step2', 'Destination'),
		t('onboarding.step3', 'Interests'),
		t('onboarding.step4', 'Connect'),
		t('onboarding.step5', 'Get Started')
	]);

	function toggleTag(tagSlug: string) {
		selectedTags = selectedTags.includes(tagSlug)
			? selectedTags.filter((s) => s !== tagSlug)
			: [...selectedTags, tagSlug];
	}

	function toggleRec(id: string) {
		selectedRecs = selectedRecs.includes(id)
			? selectedRecs.filter((s) => s !== id)
			: [...selectedRecs, id];
	}

	async function saveStep() {
		saving = true;
		error = null;

		const stepData: Record<string, unknown> = {};
		if (currentStep === 1) {
			stepData.name = name;
			stepData.japa_status = japaStatus;
			stepData.current_location = currentLocation;
		} else if (currentStep === 2) {
			stepData.destination = destination;
			stepData.primary_goal = primaryGoal;
		} else if (currentStep === 3) {
			// Send both 'tags' and 'interests' so the backend recommendation
			// engine (which reads 'tags' with 'interests' fallback) works.
			stepData.tags = selectedTags;
			stepData.interests = selectedTags;
		} else if (currentStep === 4) {
			stepData.follows = selectedRecs;
		}

		try {
			if (currentStep < totalSteps) {
				const res = await api.put('/onboarding/step', { step: currentStep, step_data: stepData });
				if (res.error) {
					error = res.error.message;
					saving = false;
					return;
				}
				currentStep++;
			} else {
				// Final step — complete onboarding.
				const res = await api.post('/onboarding/complete', {});
				if (res.error) {
					error = res.error.message;
					saving = false;
					return;
				}
				await goto('/');
			}
		} catch {
			error = t('onboarding.error', 'Something went wrong. Please try again.');
		}
		saving = false;
	}

	async function skipStep() {
		saving = true;
		error = null;
		try {
			if (currentStep < totalSteps) {
				const res = await api.put('/onboarding/step', { step: currentStep, step_data: {} });
				if (res.error) {
					error = res.error.message;
					saving = false;
					return;
				}
				currentStep++;
			} else {
				await goto('/');
			}
		} catch {
			error = t('onboarding.error', 'Something went wrong. Please try again.');
		}
		saving = false;
	}
</script>

<svelte:head>
	<title>{t('onboarding.title', 'Welcome to JapaConnect')} — {t('app.name')}</title>
</svelte:head>

<div class="min-h-screen bg-[var(--color-bg)]">
	<div class="mx-auto max-w-2xl px-6 py-12">
		<!-- Step indicator -->
		<div class="mb-10 flex items-center justify-center gap-2">
			{#each Array(totalSteps) as _, i (i)}
				<div
					class="h-2 rounded-full transition-all {i + 1 === currentStep
						? 'w-8 bg-[var(--color-primary)]'
						: i + 1 < currentStep
							? 'w-2 bg-[var(--color-primary)]'
							: 'w-2 bg-[var(--color-border)]'}"
				></div>
			{/each}
		</div>

		<div class="rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-8">
			<!-- Step 1: Welcome & Core Identity -->
			{#if currentStep === 1}
				<div class="mb-6 flex items-center gap-3">
					<Sparkles class="text-[var(--color-primary)]" size={24} />
					<h1 class="text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step1_title', 'Welcome! Tell us about yourself')}</h1>
				</div>
				<div class="space-y-4">
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-name">{t('onboarding.field_name', 'Your name')}</label>
						<input id="ob-name" bind:value={name} type="text" class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none" />
					</div>
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-status">{t('onboarding.field_japa_status', 'Where are you in your japa journey?')}</label>
						<select id="ob-status" bind:value={japaStatus} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none">
							<option value="">{t('onboarding.select_option', 'Select...')}</option>
							<option value="newly_japa">{t('onboarding.status_newly_japa', 'Newly Japa (< 1 year)')}</option>
							<option value="settled">{t('onboarding.status_settled', 'Settled Abroad')}</option>
							<option value="about_to_japa">{t('onboarding.status_about_to_japa', 'About to Japa')}</option>
							<option value="planning">{t('onboarding.status_planning', 'Planning to Relocate')}</option>
							<option value="looking_jobs">{t('onboarding.status_looking_jobs', 'Looking for Jobs Abroad')}</option>
							<option value="looking_study">{t('onboarding.status_looking_study', 'Looking to Study Abroad')}</option>
							<option value="returning">{t('onboarding.status_returning', 'Considering Returning Home')}</option>
							<option value="remote_worker">{t('onboarding.status_remote_worker', 'Remote Worker (Nigeria-based)')}</option>
						</select>
					</div>
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-country">{t('onboarding.field_country', 'Country')}</label>
						<select id="ob-country" bind:value={selectedCountry} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none">
							<option value="">{t('onboarding.select_option', 'Select...')}</option>
							{#each data.countries as c (c.id)}
								<option value={c.geo_code}>{c.name}</option>
							{/each}
						</select>
						{#if data.geoContext?.is_valid}
							<p class="mt-1 flex items-center gap-1 text-xs text-[var(--color-muted)]">
								<MapPin size={11} /> {t('onboarding.geo_detected', 'Detected: {country}').replace('{country}', data.geoContext.country_name)}
							</p>
						{/if}
					</div>
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-location">{t('onboarding.field_location', 'Current location')}</label>
						<input id="ob-location" bind:value={currentLocation} type="text" placeholder="e.g. Lagos, Nigeria" class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none" />
					</div>
				</div>
			{/if}

			<!-- Step 2: Destination & Goals -->
			{#if currentStep === 2}
				<h1 class="mb-6 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step2_title', 'Where are you headed?')}</h1>
				<div class="space-y-4">
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-dest">{t('onboarding.field_destination', 'Destination country')}</label>
						<input id="ob-dest" bind:value={destination} type="text" placeholder="e.g. United Kingdom" class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none" />
					</div>
					<div>
						<label class="mb-1 block text-sm font-medium text-[var(--color-text)]" for="ob-goal">{t('onboarding.field_goal', 'Primary goal')}</label>
						<select id="ob-goal" bind:value={primaryGoal} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] focus:border-[var(--color-primary)] focus:outline-none">
							<option value="">{t('onboarding.select_option', 'Select...')}</option>
							<option value="work">{t('onboarding.goal_work', 'Work / Career')}</option>
							<option value="study">{t('onboarding.goal_study', 'Study / Education')}</option>
							<option value="business">{t('onboarding.goal_business', 'Business / Entrepreneurship')}</option>
							<option value="family">{t('onboarding.goal_family', 'Family / Reunion')}</option>
							<option value="lifestyle">{t('onboarding.goal_lifestyle', 'Lifestyle / Quality of life')}</option>
							<option value="unsure">{t('onboarding.goal_unsure', "I'm not sure yet")}</option>
						</select>
					</div>
				</div>
			{/if}

			<!-- Step 3: Interests/Tags -->
			{#if currentStep === 3}
				<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step3_title', 'What are you interested in?')}</h1>
				<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step3_subtitle', 'Select at least {n} topics to personalize your feed.').replace('{n}', String(minTags))}</p>
				<div class="flex flex-wrap gap-2">
					{#each data.tags as tag (tag.id)}
						<button
							type="button"
							onclick={() => toggleTag(tag.slug)}
							class="rounded-full border px-3 py-1.5 text-sm transition-colors {selectedTags.includes(tag.slug)
								? 'border-[var(--color-primary)] bg-[var(--color-primary)] text-[var(--color-primary-contrast)]'
								: 'border-[var(--color-border)] text-[var(--color-text-subtle)] hover:border-[var(--color-primary)]'}"
						>
							#{tag.name}
						</button>
					{/each}
				</div>
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('onboarding.selected_count', '{n} selected').replace('{n}', String(selectedTags.length))}</p>
			{/if}

			<!-- Step 4: Recommended Spaces & Connections -->
			{#if currentStep === 4}
				<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step4_title', 'Connect with your community')}</h1>
				<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step4_subtitle', 'Follow at least {n} spaces and people to start your journey.').replace('{n}', String(Math.max(minSpaces, minUsers)))}</p>
				<div class="space-y-2">
					{#each data.recommendations as rec (rec.id)}
						<button
							type="button"
							onclick={() => toggleRec(rec.id)}
							class="flex w-full items-center gap-3 rounded-[var(--radius)] border p-3 text-left transition-colors {selectedRecs.includes(rec.id)
								? 'border-[var(--color-primary)] bg-[var(--color-surface-subtle)]'
								: 'border-[var(--color-border)] hover:bg-[var(--color-surface-subtle)]'}"
						>
							{#if rec.avatar_url}
								<img src={rec.avatar_url} alt="" class="h-10 w-10 rounded-full object-cover" />
							{:else}
								<span class="grid h-10 w-10 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-sm font-medium text-[var(--color-text-subtle)]">{rec.name.charAt(0)}</span>
							{/if}
							<span class="min-w-0 flex-1">
								<span class="block truncate text-sm font-medium text-[var(--color-text)]">{rec.name}</span>
								<span class="block text-xs text-[var(--color-muted)]">{rec.type === 'space' ? t('onboarding.rec_space', 'Space') : t('onboarding.rec_user', 'User')} · {rec.reason.replace(/_/g, ' ')}</span>
							</span>
							{#if selectedRecs.includes(rec.id)}
								<Check size={18} class="text-[var(--color-primary)]" />
							{/if}
						</button>
					{/each}
				</div>
				<p class="mt-4 text-xs text-[var(--color-muted)]">{t('onboarding.selected_count', '{n} selected').replace('{n}', String(selectedRecs.length))}</p>
			{/if}

			<!-- Step 5: First Action Nudge -->
			{#if currentStep === 5}
				<div class="text-center">
					<div class="mx-auto mb-4 grid h-16 w-16 place-items-center rounded-full bg-[var(--color-primary)]">
						<Check size={32} class="text-[var(--color-primary-contrast)]" />
					</div>
					<h1 class="mb-2 text-2xl font-bold text-[var(--color-text)]">{t('onboarding.step5_title', "You're all set!")}</h1>
					<p class="mb-6 text-sm text-[var(--color-text-subtle)]">{t('onboarding.step5_body', 'Your feed is ready. Explore stories, join discussions, and connect with the diaspora community.')}</p>
				</div>
			{/if}

			<!-- Error -->
			{#if error}
				<p class="mt-4 text-sm text-red-500">{error}</p>
			{/if}

			<!-- Actions -->
			<div class="mt-8 flex items-center justify-between">
				<button
					type="button"
					onclick={skipStep}
					disabled={saving}
					class="text-sm text-[var(--color-muted)] hover:text-[var(--color-text)] disabled:opacity-50"
				>
					{t('onboarding.skip', 'Skip')}
				</button>
				<button
					type="button"
					onclick={saveStep}
					disabled={saving}
					class="flex items-center gap-2 rounded-[var(--radius)] bg-[var(--color-primary)] px-6 py-2.5 text-sm font-semibold text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
				>
					{saving ? t('onboarding.saving', 'Saving...') : currentStep === totalSteps ? t('onboarding.finish', 'Get started') : t('onboarding.next', 'Next')}
					{#if currentStep < totalSteps}<ChevronRight size={16} />{/if}
				</button>
			</div>
		</div>

		<!-- Step label -->
		<p class="mt-4 text-center text-xs text-[var(--color-muted)]">{currentStep} / {totalSteps} — {stepLabels[currentStep - 1]}</p>
	</div>
</div>
