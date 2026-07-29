<script lang="ts">
	import { t } from '$lib/i18n/store.svelte';
	import { page } from '$app/state';

	const status = $derived(page.data.status);
</script>

<div class="flex flex-col items-center gap-4 text-center">
	{#if status === 'verifying'}
		<p class="text-[var(--color-muted)]">{t('auth.verify_email.verifying')}</p>
	{:else if status === 'success'}
		<h1 class="text-2xl font-semibold text-green-600">{t('auth.verify_email.success')}</h1>
		<a href="/" class="text-sm text-[var(--color-primary)] hover:underline">
			{t('auth.verify_email.back_to_home')}
		</a>
	{:else if status === 'already_verified'}
		<h1 class="text-2xl font-semibold text-[var(--color-text)]">{t('auth.verify_email.already_verified')}</h1>
		<a href="/" class="text-sm text-[var(--color-primary)] hover:underline">
			{t('auth.verify_email.back_to_home')}
		</a>
	{:else}
		<h1 class="text-2xl font-semibold text-red-500">{t('auth.verify_email.invalid_token')}</h1>
		<a href="/login" class="text-sm text-[var(--color-primary)] hover:underline">
			{t('auth.forgot_password.back_to_login')}
		</a>
	{/if}
</div>
