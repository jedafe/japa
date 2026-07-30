<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/stores/i18n.svelte';
	import { Home, AlertCircle, RefreshCw } from 'lucide-svelte';

	const status = $derived(page.status);
	const isNotFound = $derived(status === 404);
	const errorTitle = $derived(
		isNotFound
			? t('error.not_found_title', 'Page Not Found')
			: t('error.server_error_title', 'Server Error')
	);
	const errorMessage = $derived(
		isNotFound
			? t('error.not_found_message', 'The page you are looking for does not exist or has been moved.')
			: page.error?.message ?? t('error.something_wrong', 'Something went wrong.')
	);

	function refreshPage() {
		if (typeof window !== 'undefined') {
			window.location.reload();
		}
	}
</script>

<svelte:head>
	<title>{errorTitle} — {t('control.brand', 'JapaControl')}</title>
</svelte:head>

<div class="flex min-h-[60vh] flex-col items-center justify-center px-4 text-center">
	<div class="mb-6 flex items-center justify-center">
		{#if isNotFound}
			<AlertCircle class="text-muted-foreground size-12" />
		{:else}
			<AlertCircle class="text-destructive size-12" />
		{/if}
	</div>
	<h1 class="text-foreground mb-2 text-5xl font-bold">
		{status}
	</h1>
	<h2 class="text-foreground mb-3 text-xl font-semibold">
		{errorTitle}
	</h2>
	<p class="text-muted-foreground mb-8 max-w-md text-sm">
		{errorMessage}
	</p>
	<div class="flex items-center gap-3">
		<a
			href="/"
			class="bg-foreground text-background hover:bg-foreground/90 inline-flex items-center gap-2 rounded-lg px-5 py-2 text-sm font-medium transition-colors"
		>
			<Home class="size-4" />
			{t('error.back_to_dashboard', 'Back to Dashboard')}
		</a>
		{#if !isNotFound}
			<button
				onclick={refreshPage}
				class="border-border-input bg-background-alt hover:bg-muted text-foreground inline-flex items-center gap-2 rounded-lg border px-5 py-2 text-sm font-medium transition-colors"
			>
				<RefreshCw class="size-4" />
				{t('common.refresh', 'Refresh')}
			</button>
		{/if}
	</div>
</div>
