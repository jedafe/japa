<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { AlertTriangle } from 'lucide-svelte';

	const status = $derived(page.status);
	const isNotFound = $derived(status === 404);
	const isForbidden = $derived(status === 403);
	const title = $derived(
		isNotFound
			? t('error.404_title', 'Page Not Found')
			: isForbidden
				? t('error.403_title', 'Access Denied')
				: t('error.500_title', 'Server Error')
	);
	const body = $derived(
		isNotFound
			? t('error.404_body', 'The page you are looking for does not exist or has been moved.')
			: isForbidden
				? t('error.403_body', "You don't have permission to view this page.")
				: page.error?.message ?? t('error.500_body', 'Something went wrong on our end.')
	);
</script>

<svelte:head>
	<title>{title} — {t('app.name', 'JapaConnect Learn')}</title>
</svelte:head>

<!--
  Error page — rendered INSIDE the root +layout.svelte which already provides
  the AppShell (LeftNav + RightSidebar). Do NOT wrap in a second AppShell or
  the left nav will be duplicated. Just render the error content directly.
-->
<div class="flex min-h-[60vh] flex-col items-center justify-center px-4 text-center">
	<div class="mb-4 flex items-center justify-center">
		<AlertTriangle
			class="size-12 {isNotFound ? 'text-[var(--color-muted)]' : 'text-[var(--destructive)]'}"
		/>
	</div>
	<h1 class="text-foreground mb-2 text-5xl font-bold">{status}</h1>
	<h2 class="text-foreground mb-3 text-xl font-semibold">{title}</h2>
	<p class="text-muted-foreground mb-8 max-w-md text-sm">{body}</p>
	<a
		href="/"
		class="bg-foreground text-background hover:bg-foreground/90 inline-flex items-center gap-2 rounded-lg px-5 py-2 text-sm font-medium transition-colors"
	>
		{t('error.back_to_home', 'Back to Home')}
	</a>
</div>
