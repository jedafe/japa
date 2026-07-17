<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { onMount } from 'svelte';
	import { api, type AdRender } from '$lib/api';
	import { imgproxy } from '$lib/utils/imgproxy';

	interface Props {
		slotName: string;
	}

	let { slotName }: Props = $props();

	let ad = $state<AdRender | null>(null);
	let loading = $state(true);

	onMount(async () => {
		try {
			ad = await api.renderAd(slotName);
			if (ad?.id) {
				api.trackAdEvent(ad.id, 'impression').catch(() => {});
			}
		} catch {
			ad = null;
		} finally {
			loading = false;
		}
	});

	function handleClick() {
		if (ad?.id) {
			api.trackAdEvent(ad.id, 'click').catch(() => {});
		}
	}
</script>

{#if loading}
	<div class="h-24 w-full animate-pulse rounded-lg" style="background-color: var(--color-surface)"></div>
{:else if ad}
	<!-- svelte-ignore a11y_click_events_have_key_events -->
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div
		class="ad-slot rounded-lg border overflow-hidden"
		style="border-color: var(--color-border); background: var(--color-surface)"
	>
		<a
			href={ad.click_url}
			target="_blank"
			rel="noopener noreferrer sponsored"
			class="block"
			onclick={handleClick}
		>
			{#if ad.image_url}
				<img
					src={imgproxy(ad.image_url, { width: 800, format: 'webp', quality: 85 })}
					alt={ad.title}
					class="w-full object-cover"
					loading="lazy"
				/>
			{:else}
				<div class="p-4 text-center">
					<p class="text-sm font-medium" style="color: var(--color-foreground)">{ad.title}</p>
				</div>
			{/if}
		</a>
	</div>
{/if}
