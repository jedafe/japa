<script lang="ts">
	import { page } from '$app/state';
	import { FileQuestion, ShieldX, TriangleAlert } from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';

	const status = $derived(page.status);
	const isNotFound = $derived(status === 404);
	const isForbidden = $derived(status === 403);
	const meta = $derived(
		isNotFound
			? { icon: FileQuestion, title: t('error.not_found_title', 'Page Not Found'), body: t('error.not_found_body', 'The page you are looking for does not exist or has been moved.') }
			: isForbidden
				? { icon: ShieldX, title: t('error.forbidden_title', 'Access Denied'), body: t('error.forbidden_body', "You don't have permission to view this page.") }
				: { icon: TriangleAlert, title: t('error.server_error_title', 'Server Error'), body: page.error?.message ?? t('error.something_went_wrong', 'Something went wrong.') }
	);
</script>

<svelte:head>
	<title>{meta.title} — JapaConnect Admin</title>
</svelte:head>

<div class="flex min-h-[60vh] flex-col items-center justify-center px-4 text-center">
	<div class="mb-6 flex items-center justify-center">
		<meta.icon class="size-12 {isNotFound ? 'text-[var(--color-muted)]' : 'text-[var(--color-danger)]'}" />
	</div>
	<h1 class="text-foreground mb-2 text-5xl font-bold">{status}</h1>
	<h2 class="text-foreground mb-3 text-xl font-semibold">{meta.title}</h2>
	<p class="text-muted mb-8 max-w-md text-sm">{meta.body}</p>
	<a
		href="/"
		class="bg-foreground text-background hover:bg-foreground/90 inline-flex items-center gap-2 rounded-lg px-5 py-2 text-sm font-medium transition-colors"
	>
		{t('error.back_to_dashboard', 'Back to Dashboard')}
	</a>
</div>
