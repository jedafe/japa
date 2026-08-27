<script lang="ts">
	// Catch-all for /compare and /compare/*.
	// The +layout.server.ts redirects to apps/jce when feature.jce is enabled.
	// When JCE is disabled, this page renders a graceful fallback because the
	// Main app no longer ships a legacy comparison composer.
	import { t } from '$lib/i18n';
	import { Scale } from 'lucide-svelte';
</script>

<div class="grid min-h-[50vh] place-items-center px-4">
	<div class="flex max-w-sm flex-col items-center gap-4 text-center">
		<div class="grid h-14 w-14 place-items-center rounded-full bg-[var(--color-surface-subtle)] text-[var(--color-muted)]">
			<Scale size={28} />
		</div>
		<h1 class="text-lg font-semibold text-[var(--color-text)]">
			{t('compare.unavailable_title', 'Compare is not available')}
		</h1>
		<p class="text-sm text-[var(--color-text-subtle)]">
			{t('compare.unavailable_body', 'The comparison engine is currently disabled. Enable JCE in settings to use this feature.')}
		</p>
	</div>
</div>
