<script lang="ts">
	import SeoHead from '$lib/seo/SeoHead.svelte';
	import SignupForm from '$lib/components/auth/SignupForm.svelte';
	import { t } from '$lib/i18n';
	import { page } from '$app/state';

	const redirectTarget = $derived(page.url.searchParams.get('redirect') ?? '/onboarding');
</script>

<SeoHead title={`${t('auth.signup.title')} — ${t('app.name')}`} description={t('auth.signup.subtitle')} />

<SignupForm {redirectTarget} />
