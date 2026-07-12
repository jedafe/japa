<script lang="ts">
	import { ArrowLeft, ArrowRight, Tag } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import { api } from '$lib/api';

	let { stepData, onupdate, onnext, onback, submitting } = $props();

	// svelte-ignore state_referenced_locally
	let selectedInterests = $state<string[]>((stepData.interests as string[]) || []);
	const minInterests = 3;

	interface Interest {
		id: string;
		label: string;
	}

	let interests = $state<Interest[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			const tags = await api.getTrendingTags(20);
			if (tags.items && tags.items.length > 0) {
				interests = tags.items.map((tag) => ({ id: tag.slug, label: tag.name }));
			} else {
				error = t('onboarding.no_recommendations');
			}
		} catch {
			error = t('common.error');
		} finally {
			loading = false;
		}
	});

	function toggleInterest(id: string) {
		if (selectedInterests.includes(id)) {
			selectedInterests = selectedInterests.filter((i) => i !== id);
		} else {
			selectedInterests = [...selectedInterests, id];
		}
	}

	function handleNext() {
		onupdate({ interests: selectedInterests });
		onnext();
	}

	const canProceed = $derived(selectedInterests.length >= minInterests);
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background: var(--color-primary); color: white;">
			<Tag size={20} />
		</div>
		<div>
			<h2 class="text-xl font-semibold">{t('onboarding.interests')}</h2>
			<p class="text-sm" style="color: var(--color-muted)">{t('onboarding.interests_subtitle').replace('{count}', String(minInterests))}</p>
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-8">
			<p class="text-sm" style="color: var(--color-muted)">{t('common.loading')}</p>
		</div>
	{:else if error}
		<div class="rounded-lg border p-4 text-center" style="border-color: var(--color-border); background: var(--color-surface)">
			<p class="text-sm" style="color: var(--color-muted)">{error}</p>
		</div>
	{:else}
		<div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
			{#each interests as interest}
				<button
					class="rounded-lg border px-3 py-2.5 text-sm font-medium transition-all"
					style={selectedInterests.includes(interest.id)
						? 'background: var(--color-primary); color: white; border-color: var(--color-primary);'
						: 'border-color: var(--color-border); color: var(--color-foreground); background: var(--color-background);'}
					onclick={() => toggleInterest(interest.id)}
				>
					{interest.label}
				</button>
			{/each}
		</div>
	{/if}

	<p class="text-xs" style="color: var(--color-muted)">
		{t('onboarding.selected_count').replace('{selected}', String(selectedInterests.length)).replace('{min}', String(minInterests))}
	</p>

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
			disabled={!canProceed || submitting}
		>
			{t('onboarding.continue')}
			<ArrowRight size={16} />
		</button>
	</div>
</div>
