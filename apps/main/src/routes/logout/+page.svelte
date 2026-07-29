<script lang="ts">
	// Minimal placeholder — the server load/action redirects to /login before
	// this renders. Shown only for the brief moment before the redirect lands.
	import { t } from '$lib/i18n/store.svelte';
</script>

<div class="grid min-h-screen place-items-center bg-[var(--color-bg)]">
	<p class="text-sm text-[var(--color-muted)]">{t('nav.logout', 'Logging out…')}</p>
</div>
