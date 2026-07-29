<script lang="ts">
	// The load function redirects before this page renders. This component
	// exists only to satisfy SvelteKit's requirement for a +page.svelte alongside
	// +page.server.ts. Users never see it.
	import { t } from '$lib/i18n';
</script>

<div class="grid min-h-screen place-items-center">
	<p class="text-sm text-[var(--color-muted)]">{t('auth.oauth.redirecting')}</p>
</div>
