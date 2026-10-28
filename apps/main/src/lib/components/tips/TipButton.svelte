<script lang="ts">
	// TipButton (§125) — the tip affordance on profiles, content, mentor
	// listings and org pages. Lazily fetches the public tip-config on first
	// open intent; renders nothing when tips are disabled for the recipient.
	//
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { t } from '$lib/i18n';
	import { getAccessToken } from '$lib/api/token.svelte';
	import { getUserTipConfig, getOrgTipConfig, type TipConfig } from '$lib/api/tips';
	import TipDialog from './TipDialog.svelte';
	import { Gift } from 'lucide-svelte';

	interface Props {
		recipientType: 'user' | 'organization';
		recipientKey: string; // username or org uuid
		recipientId?: string; // user uuid — required for POST /tips
		recipientName: string;
		contentId?: string;
		class?: string;
	}
	let { recipientType, recipientKey, recipientId, recipientName, contentId, class: cls = '' }: Props = $props();

	let open = $state(false);
	let loading = $state(false);
	let config = $state<TipConfig | null>(null);
	let fetched = $state(false);

	async function ensureConfig(): Promise<TipConfig | null> {
		if (!fetched) {
			loading = true;
			fetched = true;
			const res =
				recipientType === 'user'
					? await getUserTipConfig(recipientKey)
					: await getOrgTipConfig(recipientKey);
			loading = false;
			if (!res.error && res.data?.enabled) config = res.data;
		}
		return config;
	}

	async function click() {
		const cfg = await ensureConfig();
		if (cfg) open = true;
	}
</script>

<button
	type="button"
	onclick={click}
	disabled={loading}
	onmouseenter={ensureConfig}
	class="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-sm font-medium transition hover:bg-muted disabled:opacity-50 {cls}"
	title={t('tips.title')}
>
	<Gift class="size-4" />
	{t('tips.title')}
</button>

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
