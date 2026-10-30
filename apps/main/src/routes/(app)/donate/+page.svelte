<script lang="ts">
	// /donate — platform donation page (§125). Loads GET /donate config and
	// opens the TipDialog pointed at the platform recipient.
	//
	// Ref: japaconnect-engineering/sections/125-donations-tips.md
	import { t } from '$lib/i18n';
	import { getAccessToken } from '$lib/api/token.svelte';
	import TipDialog from '$lib/components/tips/TipDialog.svelte';
	import { ExternalLink, Heart } from 'lucide-svelte';

	let { data } = $props();
	const config = $derived(data.config);
	let open = $state(false);
	const authed = $derived(!!getAccessToken());
</script>

<svelte:head>
	<title>{t('donate.title')}</title>
	<meta name="description" content={t('donate.description')} />
</svelte:head>

<div class="mx-auto max-w-lg px-4 py-16 text-center">
	{#if !config.enabled}
		<h1 class="mb-2 text-2xl font-bold">{t('donate.title')}</h1>
		<p class="text-muted-foreground">{t('donate.unavailable')}</p>
	{:else}
		<Heart class="mx-auto mb-4 size-10 text-primary" />
		<h1 class="mb-2 text-3xl font-bold">{t('donate.title')}</h1>
		<p class="mb-8 text-muted-foreground">
			{config.thank_you_message || t('donate.description')}
		</p>

		{#if authed}
			<button type="button" onclick={() => (open = true)}
				class="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 font-semibold text-primary-foreground transition hover:bg-primary/90">
				<Heart class="size-4" />{t('donate.cta')}
			</button>
		{:else}
			<a href="/login?return=/donate" class="inline-flex items-center gap-2 rounded-lg bg-primary px-6 py-3 font-semibold text-primary-foreground transition hover:bg-primary/90">
				{t('donate.sign_in')}
			</a>
		{/if}

		{#if config.oc_collective_slug}
			<!-- §125 — platform collective renders as the OC donate embed
			     (iframe — no third-party script on our origin); the outbound
			     link stays as a fallback for users who prefer it. -->
			<div class="mb-8 overflow-hidden rounded-lg border border-border">
				<iframe
					src="https://opencollective.com/embed/{config.oc_collective_slug}/donate"
					title={t('donate.oc_title', 'Donate via Open Collective')}
					class="h-[380px] w-full"
					loading="lazy"
				></iframe>
			</div>
		{/if}

		{#if config.oc_collective_slug || (config.external_links ?? []).length > 0}
			<div class="mt-8 border-t border-border pt-6">
				<p class="mb-3 text-xs font-semibold uppercase text-muted-foreground">{t('tips.external')}</p>
				<div class="flex flex-col items-center gap-2">
					{#if config.oc_collective_slug}
						<a href="https://opencollective.com/{config.oc_collective_slug}/donate" target="_blank" rel="noopener"
							class="inline-flex items-center gap-2 rounded-lg border border-border px-4 py-2 text-sm hover:bg-muted">
							<ExternalLink class="size-4" />Open Collective
						</a>
					{/if}
					{#each config.external_links ?? [] as link}
						<a href={link.url} target="_blank" rel="noopener"
							class="inline-flex items-center gap-2 rounded-lg border border-border px-4 py-2 text-sm hover:bg-muted">
							<ExternalLink class="size-4" />{link.label}
						</a>
					{/each}
				</div>
			</div>
		{/if}

		<TipDialog bind:open recipientType="platform" recipientName={t('donate.platform_name')} {config} />
	{/if}
</div>
