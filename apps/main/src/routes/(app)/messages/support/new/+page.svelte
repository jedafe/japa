<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
	// Full-page new-ticket form — the standalone fallback for the inline
	// ?new=1 pane in the /messages/support workbench (same TicketNewPane).
	import { t } from '#lib/i18n/index.js';
	import type { TicketType } from '#lib/types/index.js';
	import TicketNewPane from '#lib/components/hub/TicketNewPane.svelte';

	let { data } = $props();
	const types = $derived(data.types as TicketType[]);
</script>

<SeoHead title={`${t('tickets.new_title')} — ${t('app.name')}`} description={t('tickets.new_subtitle')} robots="noindex" />

<section class="space-y-4">
	<header>
		<a href="/messages/support" class="text-sm text-[var(--color-muted)] hover:underline">
			← {t('tickets.back_to_list')}
		</a>
		<h1 class="mt-2 text-lg font-medium text-[var(--color-text)]">{t('tickets.new_title')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('tickets.new_subtitle')}</p>
	</header>

	<TicketNewPane {types} />
</section>
