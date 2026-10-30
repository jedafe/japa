<script lang="ts">
	// TipButton (§125) — the tip affordance on profiles, content, mentor
	// listings and org pages. Lazily fetches the public tip-config on first
	// open intent; renders nothing when tips are disabled for the recipient.
	//
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { t } from '$lib/i18n';
	import { getUserTipConfig, getOrgTipConfig, type TipConfig } from '$lib/api/tips';
	import TipDialog from './TipDialog.svelte';
	import { Gift } from 'lucide-svelte';

	interface Props {
		recipientType: 'user' | 'organization';
		recipientKey: string; // username or org uuid
		recipientId?: string; // user uuid — required for POST /tips
		recipientName: string;
		contentId?: string;
		compact?: boolean; // icon-only variant for feed/action rows
		class?: string;
	}
	let { recipientType, recipientKey, recipientId, recipientName, contentId, compact = false, class: cls = '' }: Props = $props();

	let open = $state(false);
	let loading = $state(false);
	let config = $state<TipConfig | null>(null);
	let fetched = $state(false);
	let unavailable = $state(false);

	async function ensureConfig(): Promise<TipConfig | null> {
		if (!fetched) {
			loading = true;
			const res =
				recipientType === 'user'
					? await getUserTipConfig(recipientKey)
					: await getOrgTipConfig(recipientKey);
			loading = false;
			// Latch only on a definitive answer — a transient failure leaves the
			// button retryable instead of permanently dead (BUG-021).
			if (!res.error) fetched = true;
			if (!res.error && res.data?.enabled) config = res.data;
		}
		return config;
	}

	async function click() {
		const cfg = await ensureConfig();
		if (cfg) {
			open = true;
		} else {
			unavailable = true;
			setTimeout(() => (unavailable = false), 2500);
		}
	}
</script>

<span class="relative inline-flex">
<button
	type="button"
	onclick={click}
	disabled={loading}
	onmouseenter={ensureConfig}
	class={compact
		? 'group/btn flex shrink-0 items-center justify-center rounded-full p-2 text-[var(--color-muted)] transition-colors hover:bg-[var(--color-primary)]/10 hover:text-[var(--color-primary)] disabled:opacity-50 ' + cls
		: 'inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-sm font-medium transition hover:bg-muted disabled:opacity-50 ' + cls}
	title={t('tips.title')}
>
	<Gift class={compact ? 'size-[18px]' : 'size-4'} />
	{#if !compact}{t('tips.title')}{/if}
</button>
{#if unavailable}
	<span class="absolute left-1/2 top-full z-10 mt-1 -translate-x-1/2 whitespace-nowrap rounded-md bg-surface px-2 py-1 text-xs text-foreground shadow-md border border-border">
		{t('tips.unavailable', 'Tips are not available here')}
	</span>
{/if}
</span>

{#if config}
	<TipDialog
		bind:open
		{recipientType}
		recipientId={recipientId ?? recipientKey}
		{contentId}
		{recipientName}
		{config}
		onclose={() => (open = false)}
	/>
{/if}
