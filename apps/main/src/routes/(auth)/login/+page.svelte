<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import { t } from '$lib/i18n/store.svelte';

	import LoginForm from '$lib/components/auth/LoginForm.svelte';
	import { page } from '$app/state';

	const oauthError = $derived(page.data.oauthError);
	const redirectTarget = $derived(page.url.searchParams.get('redirect') ?? '/');
</script>

<SeoHead title={`${t('auth.login.title')} — ${t('app.name')}`} description={t('auth.login.subtitle')} />

{#if oauthError}
	<p class="mb-4 rounded-[var(--radius)] bg-red-500/10 px-3 py-2 text-sm text-red-500" role="alert">
		{t(`auth.errors.oauth_${oauthError}`, t('auth.errors.generic'))}
	</p>
{/if}

<LoginForm {redirectTarget} />
