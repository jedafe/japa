<script lang="ts">
	import { t } from '$lib/i18n';
	import ChangePasswordForm from '../flows/ChangePasswordForm.svelte';
	import TotpEnrollFlow from '../flows/TotpEnrollFlow.svelte';

	interface SecurityData {
		totp?: { enabled: boolean };
	}

	let { data }: { data: unknown } = $props();
	const sec = $derived(data as SecurityData | null);
</script>

<section class="max-w-2xl space-y-6">
	<header>
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t('settings.nav.security')}</h1>
		<p class="text-sm text-[var(--color-muted)]">{t('settings.security.desc')}</p>
	</header>

	<div class="rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
		<h2 class="mb-3 text-sm font-medium text-[var(--color-text)]">{t('settings.security.password_title')}</h2>
		<ChangePasswordForm />
	</div>

	<TotpEnrollFlow enabled={sec?.totp?.enabled ?? false} />
</section>
