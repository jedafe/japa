<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { Tag, ExternalLink, Loader2, Clock } from 'lucide-svelte';
	import { api, type Coupon } from '$lib/api';
	import { onMount } from 'svelte';

	interface Props {
		// The coupon_embed TipTap node stores ONLY the entity id (ADR-052 render-time
		// resolution). The code, expiry, and affiliate URL are resolved fresh here.
		couponId: string;
	}

	let { couponId }: Props = $props();

	let coupon = $state<Coupon | null>(null);
	let loading = $state(true);
	let failed = $state(false);
	let copied = $state(false);

	onMount(async () => {
		try {
			coupon = await api.getCoupon(couponId);
		} catch {
			failed = true;
		} finally {
			loading = false;
		}
	});

	async function handleClaim() {
		if (!coupon) return;
		// Track the click (async, fire-and-forget) then open the affiliate URL.
		api.trackCouponClick(coupon.id).catch(() => {});
		if (coupon.affiliate_url) {
			window.open(coupon.affiliate_url, '_blank', 'noopener,noreferrer');
		}
	}

	async function copyCode() {
		if (!coupon?.code) return;
		try {
			await navigator.clipboard.writeText(coupon.code);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// clipboard unavailable
		}
	}

	const isExpired = $derived(
		coupon?.expires_at ? new Date(coupon.expires_at) < new Date() : false
	);
</script>

{#if loading}
	<div
		class="my-4 flex items-center gap-2 rounded-xl border p-4 text-sm"
		style="border-color: var(--color-border); color: var(--color-muted)"
	>
		<Loader2 class="h-4 w-4 animate-spin" /> Loading offer…
	</div>
{:else if failed || !coupon || !coupon.active || isExpired}
	<!-- Fail closed: never render a stale or inactive coupon -->
	<div
		class="my-4 rounded-xl border border-dashed p-4 text-sm"
		style="border-color: var(--color-border); color: var(--color-muted)"
	>
		This offer is no longer available.
	</div>
{:else}
	<div
		class="my-4 overflow-hidden rounded-xl border"
		style="border-color: var(--color-primary); background: color-mix(in srgb, var(--color-primary) 5%, var(--color-surface))"
	>
		<div class="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
			<div class="flex items-start gap-3">
				<div
					class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-white"
					style="background: var(--color-primary)"
				>
					<Tag class="h-5 w-5" />
				</div>
				<div>
					<p class="font-semibold" style="color: var(--color-foreground)">{coupon.title}</p>
					{#if coupon.description}
						<p class="mt-0.5 text-sm" style="color: var(--color-muted)">{coupon.description}</p>
					{/if}
					{#if coupon.expires_at}
						<p class="mt-1 flex items-center gap-1 text-xs" style="color: var(--color-muted)">
							<Clock class="h-3 w-3" /> Expires {new Date(coupon.expires_at).toLocaleDateString()}
						</p>
					{/if}
				</div>
			</div>

			<div class="flex items-center gap-2">
				{#if coupon.code}
					<button
						onclick={copyCode}
						class="rounded-lg border border-dashed px-3 py-2 font-mono text-sm font-semibold transition-colors"
						style="border-color: var(--color-primary); color: var(--color-primary)"
						title="Copy code"
					>
						{copied ? 'Copied!' : coupon.code}
					</button>
				{/if}
				{#if coupon.affiliate_url}
					<button
						onclick={handleClaim}
						class="flex items-center gap-1.5 rounded-lg px-4 py-2 text-sm font-semibold text-white transition-opacity hover:opacity-90"
						style="background: var(--color-primary)"
					>
						Claim {coupon.discount_label ?? 'offer'}
						<ExternalLink class="h-3.5 w-3.5" />
					</button>
				{/if}
			</div>
		</div>
	</div>
{/if}
