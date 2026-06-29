<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	const currentYear = new Date().getFullYear();
</script>

<footer
	class="border-t py-8"
	style="background-color: var(--color-background); border-color: var(--color-border)"
>
	<div class="mx-auto max-w-6xl px-4">
		<div class="flex flex-col items-center justify-between gap-4 sm:flex-row">
			<div class="flex items-center gap-6 text-sm" style="color: var(--color-muted)">
				<a href="/about" class="hover:underline">{$t('nav.about')}</a>
				<a href="/terms" class="hover:underline">{$t('nav.terms')}</a>
				<a href="/privacy" class="hover:underline">{$t('nav.privacy')}</a>
				<a href="/contact" class="hover:underline">{$t('nav.contact')}</a>
			</div>
			<p class="text-sm" style="color: var(--color-muted)">
				{@html $t('footer.copyright').replace('{year}', String(currentYear))}
			</p>
		</div>
	</div>
</footer>
