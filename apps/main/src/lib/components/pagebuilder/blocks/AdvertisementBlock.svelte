<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from '$lib/i18n';

	interface Props {
		props: {
			provider?: 'adsense' | 'ezoic' | 'media_net' | 'custom';
			slot_id?: string;
			format?: string;
			custom_html?: string;
			fallback_url?: string;
			lazy_load?: boolean;
		};
	}

	let { props }: Props = $props();

	let container: HTMLDivElement;

	// Inject the ad script after mount so SSR doesn't break.
	$effect(() => {
		if (!container) return;
		const provider = props.provider ?? 'adsense';

		if (provider === 'custom' && props.custom_html) {
			container.innerHTML = props.custom_html;
			return;
		}

		if (provider === 'adsense' && props.slot_id) {
			// Render an AdSense ins element; the adsbygoogle script (loaded
			// once in the app shell) pushes the ad into the slot.
			const ins = document.createElement('ins');
			ins.className = 'adsbygoogle';
			ins.style.display = 'block';
			ins.setAttribute('data-ad-client', 'ca-pub-0000000000000000');
			ins.setAttribute('data-ad-slot', props.slot_id);
			ins.setAttribute('data-ad-format', props.format ?? 'auto');
			ins.setAttribute('data-full-width-responsive', 'true');
			container.innerHTML = '';
			container.appendChild(ins);
			try {
				((window as any).adsbygoogle = (window as any).adsbygoogle || []).push({});
			} catch {
				// adsbygoogle not loaded yet — the script will pick it up
			}
		} else if (provider === 'ezoic' && props.slot_id) {
			// Ezoic uses a placeholder div with data-ezoic-id.
			const div = document.createElement('div');
			div.setAttribute('data-ezoic-id', props.slot_id);
			container.innerHTML = '';
			container.appendChild(div);
		} else if (provider === 'media_net' && props.slot_id) {
			// Media.net uses a script-based slot.
			const div = document.createElement('div');
			div.id = `media-net-${props.slot_id}`;
			container.innerHTML = '';
			container.appendChild(div);
		}
	});
</script>

<div
	bind:this={container}
	class="advertisement-block rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-2"
	data-ad-provider={props.provider ?? 'adsense'}
	data-ad-slot={props.slot_id ?? ''}
	aria-label={t('ads.label', 'Advertisement')}
>
	{#if !props.slot_id && !props.custom_html}
		<div class="grid h-[250px] place-items-center text-xs text-[var(--color-muted)]">
			{t('ads.label', 'Advertisement')}
		</div>
	{/if}
</div>
