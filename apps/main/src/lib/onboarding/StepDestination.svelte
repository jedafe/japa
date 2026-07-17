<script lang="ts">
	import { ArrowLeft, ArrowRight, Plane, Target, MapPin } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { api } from '$lib/api';

	let { stepData, onupdate, onnext, onback, submitting } = $props();

	// svelte-ignore state_referenced_locally
	let destination = $state((stepData.destination as string) || '');
	// svelte-ignore state_referenced_locally
	let primaryGoal = $state((stepData.primary_goal as string) || '');

	// Detected current location (from IP) — shown as helpful context, not a hard input.
	let detectedCountry = $state('');

	let destinations = $state([
		{ value: 'not_sure', label: 'Not sure yet' }
	]);

	const goals = [
		{ value: 'education', label: 'Education' },
		{ value: 'career', label: 'Career' },
		{ value: 'business', label: 'Business' },
		{ value: 'family', label: 'Family' },
		{ value: 'not_sure', label: 'Not sure yet' }
	];

	onMount(async () => {
		try {
			const [geo, countries] = await Promise.all([api.geoMe(), api.geoCountries()]);
			if (geo.country) detectedCountry = geo.country;
			if (Array.isArray(countries) && countries.length > 0) {
				destinations = [
					...countries.map((c) => ({ value: c.code.toLowerCase(), label: c.name })),
					{ value: 'not_sure', label: 'Not sure yet' }
				];
			}
		} catch {
			// geo/countries are best-effort fallback
		}
	});

	function handleNext() {
		onupdate({ destination, primary_goal: primaryGoal });
		onnext();
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background: var(--color-primary); color: white;">
			<Plane size={20} />
		</div>
		<div>
			<h2 class="text-xl font-semibold">{t('onboarding.destination')}</h2>
			<p class="text-sm" style="color: var(--color-muted)">{t('onboarding.destination_subtitle')}</p>
		</div>
	</div>

	{#if detectedCountry}
		<div
			class="flex items-center gap-2 rounded-lg px-3 py-2 text-sm"
			style="background: color-mix(in srgb, var(--color-primary) 8%, transparent); color: var(--color-muted)"
		>
			<MapPin class="h-4 w-4" style="color: var(--color-primary)" />
			<span>{t('lib.onboarding.StepDestination.svelte.detected_you_re_browsing_from')}<strong style="color: var(--color-foreground)">{detectedCountry}</strong></span>
		</div>
	{/if}

	<div class="space-y-4">
		<div>
			<label for="destination" class="mb-1 block text-sm font-medium">{t('onboarding.destination_country')}</label>
			<div class="relative">
				<select
					id="destination"
					bind:value={destination}
					class="w-full appearance-none rounded-lg border px-4 py-2.5 text-sm outline-none focus:ring-2"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary);"
				>
					<option value="">{t('onboarding.destination_placeholder')}</option>
					{#each destinations as dest}
						<option value={dest.value}>{dest.label}</option>
					{/each}
				</select>
				<Plane class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2" size={16} style="color: var(--color-muted)" />
			</div>
		</div>

		<div>
			<label for="goal" class="mb-1 block text-sm font-medium">{t('onboarding.primary_goal')}</label>
			<div class="relative">
				<select
					id="goal"
					bind:value={primaryGoal}
					class="w-full appearance-none rounded-lg border px-4 py-2.5 text-sm outline-none focus:ring-2"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary);"
				>
					<option value="">{t('onboarding.goal_placeholder')}</option>
					{#each goals as goal}
						<option value={goal.value}>{goal.label}</option>
					{/each}
				</select>
				<Target class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2" size={16} style="color: var(--color-muted)" />
			</div>
		</div>
	</div>

	<div class="flex gap-3 mt-6">
		<button
			class="flex items-center gap-2 rounded-lg border px-4 py-2.5 text-sm font-medium transition-colors hover:opacity-80"
			style="border-color: var(--color-border); color: var(--color-foreground)"
			onclick={onback}
		>
			<ArrowLeft size={16} />
			{t('onboarding.back')}
		</button>
		<button
			class="flex flex-1 items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
			style="background: var(--color-primary)"
			onclick={handleNext}
			disabled={!destination || !primaryGoal || submitting}
		>
			{t('onboarding.continue')}
			<ArrowRight size={16} />
		</button>
	</div>
</div>
