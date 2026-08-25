<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
</script>

<main class="mx-auto max-w-2xl p-8 text-center">
	<h1 class="text-4xl font-bold">{page.status}</h1>
	<p class="mt-2 text-[var(--color-muted)]">
		{t(page.error?.message ?? 'jce.error.fallback', 'Something went wrong')}
	</p>
	<a href="/" class="mt-6 inline-block text-sm underline text-[var(--color-muted)] hover:text-white">
		{t('jce.error.go_home', 'Go home')}
	</a>
</main>
