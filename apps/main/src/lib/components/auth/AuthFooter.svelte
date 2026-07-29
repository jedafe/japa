<script lang="ts">
	// Terms/Privacy footer for auth forms. The login/signup switch affordance
	// moved into the form header (see LoginForm / SignupForm).
	import { t } from '$lib/i18n/store.svelte';
</script>

<div class="flex items-center justify-center gap-4 pt-2 text-xs text-[var(--color-muted)]">
	<a href="/terms" class="hover:underline">{t('auth.footer.terms')}</a>
	<span aria-hidden="true">·</span>
	<a href="/privacy" class="hover:underline">{t('auth.footer.privacy')}</a>
</div>
