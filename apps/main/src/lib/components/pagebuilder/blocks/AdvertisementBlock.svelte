<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';

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

	interface AdNetwork {
		provider: 'adsense' | 'ezoic' | 'media_net' | 'custom';
		name: string;
		client_id: string;
		slot_mapping: Record<string, number>;
		config: Record<string, unknown>;
		priority: number;
	}

	let networks = $state<AdNetwork[]>([]);

	// Resolve the effective ad network configuration from the API.
	function resolveNetwork(): AdNetwork | null {
		const requestedProvider = props.provider ?? 'adsense';
		const match = networks.find((n) => n.provider === requestedProvider);
		return match ?? null;
	}

	// Resolve the slot ID: prefer slot_mapping from API, fall back to props.slot_id.
	function resolveSlotId(network: AdNetwork | null): string | null {
		if (network?.slot_mapping) {
			const key = props.format ?? 'default';
			const mapped = network.slot_mapping[key];
			if (mapped !== undefined) return String(mapped);
		}
		return props.slot_id ?? null;
	}

	// Inject the ad script after mount so SSR doesn't break.
	$effect(() => {
		if (!container) return;

		const network = resolveNetwork();
		const slotId = resolveSlotId(network);
		const provider = props.provider ?? 'adsense';

		if (provider === 'custom' && props.custom_html) {
			container.innerHTML = props.custom_html;
			return;
		}

		if (provider === 'adsense' && slotId) {
			const clientId = network?.client_id ?? 'ca-pub-0000000000000000';
			// Render an AdSense ins element; the adsbygoogle script (loaded
			// once in the app shell) pushes the ad into the slot.
			const ins = document.createElement('ins');
			ins.className = 'adsbygoogle';
			ins.style.display = 'block';
			ins.setAttribute('data-ad-client', clientId);
			ins.setAttribute('data-ad-slot', slotId);
			ins.setAttribute('data-ad-format', props.format ?? 'auto');
			ins.setAttribute('data-full-width-responsive', 'true');
			container.innerHTML = '';
			container.appendChild(ins);
			try {
				((window as any).adsbygoogle = (window as any).adsbygoogle || []).push({});
			} catch {
				// adsbygoogle not loaded yet — the script will pick it up
			}
		} else if (provider === 'ezoic' && slotId) {
			// Ezoic uses a placeholder div with data-ezoic-id.
			const div = document.createElement('div');
			div.setAttribute('data-ezoic-id', slotId);
			container.innerHTML = '';
			container.appendChild(div);
		} else if (provider === 'media_net' && slotId) {
			// Media.net uses a script-based slot.
			const div = document.createElement('div');
			div.id = `media-net-${slotId}`;
			container.innerHTML = '';
			container.appendChild(div);
		}
	});

	// Fetch ad network config on mount.
	$effect(() => {
		void (async () => {
			try {
				const result = await api.get<AdNetwork[]>('/api/v1/ads/networks/config');
				if (result.data && Array.isArray(result.data)) {
					networks = result.data;
				}
			} catch {
				// Network fetch failed — the rendering effect will use hardcoded fallbacks.
			}
		})();
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
