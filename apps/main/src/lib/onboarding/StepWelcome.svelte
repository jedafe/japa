<script lang="ts">
	import { Globe, MapPin, User } from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';

	let { stepData, onupdate, onnext, submitting } = $props();

	// svelte-ignore state_referenced_locally
	let name = $state((stepData.name as string) || '');
	// svelte-ignore state_referenced_locally
	let japaStatus = $state((stepData.japa_status as string) || '');
	// svelte-ignore state_referenced_locally
	let currentLocation = $state((stepData.current_location as string) || '');

	const japaOptions = [
		{ value: 'planning', label: 'Planning to relocate' },
		{ value: 'relocated', label: 'Already relocated' },
		{ value: 'exploring', label: 'Exploring options' }
	];

	function handleNext() {
		onupdate({ name, japa_status: japaStatus, current_location: currentLocation });
		onnext();
	}
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background: var(--color-primary); color: white;">
			<User size={20} />
		</div>
		<div>
			<h2 class="text-xl font-semibold">{t('onboarding.welcome')}</h2>
			<p class="text-sm" style="color: var(--color-muted)">{t('onboarding.tell_about')}</p>
		</div>
	</div>

	<div class="space-y-4">
		<div>
			<label for="name" class="mb-1 block text-sm font-medium">{t('onboarding.your_name')}</label>
			<input
				id="name"
				type="text"
				bind:value={name}
				placeholder={t('onboarding.name_placeholder')}
				class="w-full rounded-lg border px-4 py-2.5 text-sm outline-none focus:ring-2"
				style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary);"
			/>
		</div>

		<div>
			<label for="japa-status" class="mb-1 block text-sm font-medium">{t('onboarding.japa_status')}</label>
			<div class="relative">
				<select
					id="japa-status"
					bind:value={japaStatus}
					class="w-full appearance-none rounded-lg border px-4 py-2.5 text-sm outline-none focus:ring-2"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary);"
				>
					<option value="">{t('onboarding.status_placeholder')}</option>
					{#each japaOptions as option}
						<option value={option.value}>{option.label}</option>
					{/each}
				</select>
				<Globe class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2" size={16} style="color: var(--color-muted)" />
			</div>
		</div>

		<div>
			<label for="location" class="mb-1 block text-sm font-medium">{t('onboarding.current_location')}</label>
			<div class="relative">
				<input
					id="location"
					type="text"
					bind:value={currentLocation}
					placeholder={t('onboarding.location_placeholder')}
					class="w-full rounded-lg border px-4 py-2.5 text-sm outline-none focus:ring-2"
					style="border-color: var(--color-border); background: var(--color-background); color: var(--color-foreground); --tw-ring-color: var(--color-primary);"
				/>
				<MapPin class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2" size={16} style="color: var(--color-muted)" />
			</div>
		</div>
	</div>

	<button
		class="mt-6 w-full rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
		style="background: var(--color-primary)"
		onclick={handleNext}
		disabled={!name || !japaStatus || !currentLocation || submitting}
	>
		{t('onboarding.continue')}
	</button>
</div>
