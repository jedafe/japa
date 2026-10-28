<script lang="ts">
	// TipDialog (§125) — send points/credits/fiat tips to a user, org, or the
	// platform. Fiat methods redirect to the provider checkout URL returned
	// by POST /tips. External links and Open Collective render as outbound
	// CTAs per the public tip-config.
	//
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { untrack } from 'svelte';
	import { t } from '$lib/i18n';
	import { Modal } from '$lib/components/ui';
	import { api } from '$lib/api/client';
	import { sendTip, newIdempotencyKey, type TipConfig } from '$lib/api/tips';
	import { AlertCircle, CheckCircle2, Coins, CreditCard, ExternalLink, Heart, Loader2, Sparkles } from 'lucide-svelte';

	interface Props {
		open: boolean;
		recipientType: 'user' | 'organization' | 'platform';
		recipientId?: string; // uuid for user/org
		contentId?: string;
		recipientName: string;
		config: TipConfig;
		onclose?: () => void;
	}
	let { open = $bindable(), recipientType, recipientId, contentId, recipientName, config, onclose }: Props = $props();

	let method = $state('credits');
	let amount = $state('5');
	let message = $state('');
	let isAnonymous = $state(false);
	let submitting = $state(false);
	let error = $state<string | null>(null);
	let succeeded = $state(false);
	let idemKey = newIdempotencyKey();

	const methods = $derived(config.methods ?? []);
	const internal = $derived(methods.filter((m) => ['points', 'credits'].includes(m)));
	const fiat = $derived(methods.filter((m) => !['points', 'credits', 'opencollective'].includes(m)));

	$effect(() => {
		if (open) {
			method = internal[0] ?? methods[0] ?? 'credits';
			amount = '5';
			message = '';
			isAnonymous = false;
			error = null;
			succeeded = false;
			idemKey = newIdempotencyKey();
		}
	});

	const unit = $derived(method === 'points' ? 'points' : method === 'credits' ? 'credits' : (config as { currency?: string }).currency ?? 'NGN');

	async function submit() {
		const amt = Number(amount);
		if (!amt || amt <= 0) {
			error = 'invalid_amount';
			return;
		}
		submitting = true;
		error = null;
		const res = await sendTip(
			{
				recipient_type: recipientType,
				recipient_id: recipientId,
				content_id: contentId,
				method,
				amount: amt,
				message: message || undefined,
				is_anonymous: isAnonymous
			},
			idemKey
		);
		submitting = false;
		if (res.error || !res.data) {
			error = res.error?.message ?? 'tip_failed';
			return;
		}
		if (res.data.checkout_url) {
			window.location.href = res.data.checkout_url; // provider checkout
			return;
		}
		succeeded = true;
		idemKey = newIdempotencyKey();
	}

	$effect(() => {
		if (!open) onclose?.();
	});
</script>

<Modal bind:open title="{t('tips.title')} — {recipientName}">
	{#if succeeded}
		<div class="flex flex-col items-center gap-3 py-6 text-center">
			<CheckCircle2 class="size-10 text-green-500" />
			<p class="font-medium">{config.thank_you_message || t('tips.thanks')}</p>
		</div>
	{:else}
		{#if error}
			<div class="mb-3 flex items-center gap-2 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
				<AlertCircle class="size-4" />{error}
			</div>
		{/if}

		{#if internal.length > 0}
			<p class="mb-1 text-xs font-semibold uppercase text-muted-foreground">{t('tips.with_wallet')}</p>
			<div class="mb-4 grid grid-cols-2 gap-2">
				{#each internal as m}
					<button type="button" onclick={() => (method = m)}
						class="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium transition {method === m ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted'}">
						{#if m === 'points'}<Sparkles class="size-4" />{t('tips.points')}
						{:else}<Coins class="size-4" />{t('tips.credits')}{/if}
					</button>
				{/each}
			</div>
		{/if}

		{#if fiat.length > 0}
			<p class="mb-1 text-xs font-semibold uppercase text-muted-foreground">{t('tips.with_card')}</p>
			<div class="mb-4 grid grid-cols-2 gap-2">
				{#each fiat as m}
					<button type="button" onclick={() => (method = m)}
						class="flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium capitalize transition {method === m ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted'}">
						<CreditCard class="size-4" />{m === 'manual_transfer' ? t('tips.bank_transfer') : m}
					</button>
				{/each}
			</div>
		{/if}

		{#if methods.length > 0}
			<label class="mb-3 block">
				<span class="mb-1 block text-sm font-medium">{t('tips.amount')} ({unit})</span>
				<input bind:value={amount} type="number" min="1" inputmode="numeric"
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm" />
			</label>
			<label class="mb-3 block">
				<span class="mb-1 block text-sm font-medium">{t('tips.message_label')}</span>
				<textarea bind:value={message} rows="2" maxlength="280" placeholder={t('tips.message_placeholder')}
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"></textarea>
			</label>
			<label class="mb-4 flex items-center gap-2 text-sm">
				<input bind:checked={isAnonymous} type="checkbox" class="size-4 rounded border-border" />
				{t('tips.anonymous')}
			</label>
			<button type="button" onclick={submit} disabled={submitting}
				class="flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground transition hover:bg-primary/90 disabled:opacity-50">
				{#if submitting}<Loader2 class="size-4 animate-spin" />{:else}<Heart class="size-4" />{/if}
				{t('tips.send')}
			</button>
		{/if}

		{#if config.oc_collective_slug || (config.external_links ?? []).length > 0}
			<div class="mt-4 border-t border-border pt-3">
				<p class="mb-2 text-xs font-semibold uppercase text-muted-foreground">{t('tips.external')}</p>
				<div class="flex flex-col gap-2">
					{#if config.oc_collective_slug}
						<a href="https://opencollective.com/{config.oc_collective_slug}/donate" target="_blank" rel="noopener"
							class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm hover:bg-muted">
							<ExternalLink class="size-4" />Open Collective
						</a>
					{/if}
					{#each config.external_links ?? [] as link}
						<a href={link.url} target="_blank" rel="noopener"
							class="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm hover:bg-muted">
							<ExternalLink class="size-4" />{link.label}
						</a>
					{/each}
				</div>
			</div>
		{/if}
	{/if}
</Modal>
