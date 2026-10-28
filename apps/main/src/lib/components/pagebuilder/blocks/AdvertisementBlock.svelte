<script module lang="ts">
	// Provider script tags are injected once per page — multiple ad blocks
	// (or the same provider in different placements) share one <script>.
	// Module scope, keyed by URL so two networks with the same loader dedupe.
	const injectedScripts = new Set<string>();
</script>

<script lang="ts">
	import { t } from '$lib/i18n';
	import { api } from '$lib/api/client';
	import { sanitizeContentHtml, safeHref } from '$lib/utils/safeHtml';

	interface Props {
		props: {
			// §25 amendment: provider is one of the 25 values in the
			// ad_networks provider enum (adsense, google_admanager, prebid,
			// mediavine, …). Open string so admin-added providers need no
			// frontend change — they render via the generic injector below.
			provider?: string;
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
		provider: string;
		name: string;
		// Backend GetAdNetworkConfig returns client_id as a nullable string
		// (providers like Ezoic/custom may not use one).
		client_id: string | null;
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
			// Try the block's format key, then a network-level 'default'
			// fallback — an admin mapping keyed only by 'default' still
			// fills every placement without needing per-format entries.
			for (const key of [props.format ?? 'default', 'default']) {
				const mapped = network.slot_mapping[key];
				if (mapped !== undefined) return String(mapped);
			}
		}
		return props.slot_id ?? null;
	}

	function ensureScript(url: string, attrs?: Record<string, unknown>): void {
		// Ad-provider loader scripts are powerful: only inject https URLs and
		// only honour a safe boolean subset (async/defer). Arbitrary
		// script_attrs (src, onload, …) would be a stored-XSS vector.
		if (!isSafeScriptUrl(url)) return;
		if (injectedScripts.has(url)) return;
		if (document.querySelector(`script[data-jc-ad-network][src="${CSS.escape(url)}"]`)) {
			injectedScripts.add(url);
			return;
		}
		injectedScripts.add(url);
		const script = document.createElement('script');
		script.src = url;
		script.async = true;
		script.setAttribute('data-jc-ad-network', props.provider ?? '');
		const raw = attrs ?? {};
		if (raw.async === true || raw.async === 'true') script.async = true;
		if (raw.defer === true || raw.defer === 'true') script.defer = true;
		// data-* pass-through only — every other attribute is dropped.
		for (const [k, v] of Object.entries(raw)) {
			if (!k.startsWith('data-') && k !== 'crossorigin') continue;
			if (v !== null && v !== undefined) script.setAttribute(k, String(v));
		}
		document.head.appendChild(script);
	}

	/** Provider loader scripts must be absolute https URLs — never
	 *  javascript:/data:, protocol-relative, or same-host-relative paths
	 *  (a compromised relative path would run first-party script). */
	function isSafeScriptUrl(url: string): boolean {
		if (!url || typeof url !== 'string') return false;
		const trimmed = url.trim();
		if (!trimmed || trimmed.startsWith('//')) return false;
		const cleaned = trimmed
			.split('')
			.filter((ch) => ch.charCodeAt(0) > 32)
			.join('')
			.toLowerCase();
		if (/^(javascript|data|vbscript):/.test(cleaned)) return false;
		return /^https:\/\//i.test(trimmed);
	}

	function genericRender(network: AdNetwork, slotId: string | null): void {
		// Config-driven renderer: the network's `config.script_url` is the
		// loader (e.g. a header-bidding or managed-provider tag), and the
		// placement is a plain div carrying the resolved slot id. No API
		// keys are rendered — the backend never serializes them.
		container.innerHTML = '';
		const div = document.createElement('div');
		div.id = `jc-ad-${network.provider}-${slotId ?? 'default'}`;
		div.setAttribute('data-ad-network', network.provider);
		if (slotId) div.setAttribute('data-ad-slot-id', slotId);
		const placementId = network.config?.placement_id;
		if (typeof placementId === 'string' && placementId) div.setAttribute('data-ad-placement', placementId);
		container.appendChild(div);
		const scriptUrl = typeof network.config?.script_url === 'string' ? network.config.script_url : null;
		if (scriptUrl) {
			ensureScript(scriptUrl, network.config?.script_attrs as Record<string, unknown> | undefined);
		}
	}

	// Inject the ad script after mount so SSR doesn't break.
	$effect(() => {
		if (!container) return;

		const network = resolveNetwork();
		const slotId = resolveSlotId(network);
		const provider = props.provider ?? 'adsense';

		if (provider === 'custom' && props.custom_html) {
			// Admin-authored ad HTML still passes the hardened content
			// sanitizer — raw innerHTML would execute stored script.
			container.innerHTML = sanitizeContentHtml(props.custom_html);
			if (!container.textContent?.trim() && !container.querySelector('img,iframe,video,svg')) {
				const note = document.createElement('p');
				note.className = 'p-4 text-center text-xs text-[var(--color-muted)]';
				note.textContent = t('ads.custom_stripped', 'Custom ad HTML contained unsupported content (scripts are stripped for security)');
				container.appendChild(note);
			}
			if (props.fallback_url && safeHref(props.fallback_url)) {
				const a = document.createElement('a');
				a.href = safeHref(props.fallback_url);
				a.rel = 'noopener noreferrer sponsored';
				a.textContent = t('ads.fallback', 'Learn more');
				container.appendChild(a);
			}
			return;
		}

		if (provider === 'adsense' && slotId && network?.client_id) {
			// Real AdSense fill needs the loader AND a real client id — the
			// previous path pushed to a queue whose script was never loaded
			// (and fell back to a dummy ca-pub id), so it could never serve.
			ensureScript(
				`https://pagead2.googlesyndication.com/pagead/js/adsbygoogle.js?client=${encodeURIComponent(network.client_id)}`,
				{ 'data-ad-client': network.client_id, 'crossorigin': 'anonymous' }
			);
			const ins = document.createElement('ins');
			ins.className = 'adsbygoogle';
			ins.style.display = 'block';
			ins.setAttribute('data-ad-client', network.client_id);
			ins.setAttribute('data-ad-slot', slotId);
			ins.setAttribute('data-ad-format', props.format ?? 'auto');
			ins.setAttribute('data-full-width-responsive', 'true');
			container.innerHTML = '';
			container.appendChild(ins);
			try {
				((window as any).adsbygoogle = (window as any).adsbygoogle || []).push({});
			} catch {
				// adsbygoogle not loaded yet — the loader picks the ins up.
			}
		} else if (network) {
			// Every other provider — ezoic, media_net, google_admanager,
			// prebid, mediavine, raptive, amazon_aps, … — resolves through
			// the config-driven renderer (script_url + placement div).
			genericRender(network, slotId);
		}
	});

	// Fetch ad network config on mount.
	// Ref: japaconnect-engineering/sections/25-ad-placement.md — public route
	// GET /ads/networks/config. The `api` client already prefixes /api/v1, so
	// the path must NOT include it (a double prefix resolves to
	// /api/v1/api/v1/... and 404s).
	$effect(() => {
		void (async () => {
			try {
				const result = await api.get<AdNetwork[]>('/ads/networks/config');
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
