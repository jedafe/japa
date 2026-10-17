<script lang="ts">
	import { mainAppUrl, buildSeoMeta } from '$lib/seo';
	import SeoHead from '$lib/components/seo/SeoHead.svelte';
	import { page } from '$app/state';
	import Icon from '$lib/components/ui/Icon.svelte';
	import ToolDisabled from '$lib/components/ui/ToolDisabled.svelte';
	import { t } from '$lib/i18n';

	/**
	 * JapaBot landing — §20, §69.
	 * The chat itself lives on the main app at /ai/chat (the canonical
	 * implementation). Signed-in users are redirected there by the server
	 * load; this page only ever renders for guests (sign-in prompt) or when
	 * the tool flag is off.
	 */
	let { data } = $props();

	const seo = $derived(
		buildSeoMeta({
			title: t('japabot.seo_title', 'JapaBot — Your AI Japa Guide | {site} Tools'),
			description: t(
				'japabot.seo_description',
				'Chat with JapaBot — answers grounded in {site}\'s published guides, forum threads, and community knowledge via RAG.'
			),
			path: '/japabot',
			origin: page.url.origin,
			ogImage: `${page.url.origin}/og/og-japabot.png`,
			jsonLd: {
				'@context': 'https://schema.org',
				'@type': 'SoftwareApplication',
				name: 'JapaBot',
				applicationCategory: 'UtilitiesApplication',
				operatingSystem: 'Web',
				offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
				description: 'AI japa guide with answers grounded in published guides and community knowledge.'
			}
		})
	);
</script>

<SeoHead seo={seo} ogType="website" twitterCard="summary" />

<div class="flex flex-col h-[calc(100vh-var(--header-height)-64px)]">
	{#if !data.enabled}
		<ToolDisabled tool={data.tool} />
	{:else}
		<!-- Sign-in required -->
		<div class="flex-1 flex items-center justify-center p-4">
			<div class="text-center max-w-sm">
				<div class="flex justify-center mb-4">
					<div class="w-16 h-16 rounded-2xl bg-[var(--tool-accent)] flex items-center justify-center">
						<Icon name="bot" size={32} class="text-white" />
					</div>
				</div>
				<h2 class="text-xl font-semibold text-text">{t('japabot.title', 'JapaBot')}</h2>
				<p class="mt-2 text-sm text-text-subtle">
					{t('japabot.subtitle', "Your AI guide for the japa journey. Ask about visas, scholarships, cost of living, and more — answers are grounded in {site}'s published content.")}
				</p>
				<div class="mt-5 flex flex-col gap-2">
					<a
						href={mainAppUrl('/login?redirect=/ai?mode=chat')}
						class="px-4 py-2.5 bg-[var(--tool-accent)] text-white rounded-lg text-sm font-medium hover:bg-[var(--tool-accent-hover)] transition-colors"
					>
						{t('japabot.sign_in_to_chat', 'Sign in to chat')}
					</a>
					<a
						href={mainAppUrl('/signup')}
						class="px-4 py-2.5 text-sm font-medium text-text hover:text-[var(--tool-accent)] transition-colors"
					>
						{t('common.create_account', 'Create free account')}
					</a>
				</div>
			</div>
		</div>
	{/if}
</div>
