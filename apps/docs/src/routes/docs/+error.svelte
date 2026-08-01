<script lang="ts">
	import { page } from '$app/state';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { t } from '$lib/i18n';
	import { persistNavCollapse } from '$lib/nav-collapse';
	import type { LayoutData } from '../$types';
	import { AlertTriangle } from 'lucide-svelte';

	let { data }: { data: LayoutData } = $props();

	let collapsed = $state(data.navCollapsed);

	async function toggleCollapse() {
		collapsed = !collapsed;
		await persistNavCollapse(collapsed);
	}

	const status = $derived(page.status);
	const isNotFound = $derived(status === 404);
	const title = $derived(isNotFound ? t('page.not_found', 'Page not found') : t('page.server_error', 'Server error'));
	const message = $derived(
		isNotFound
			? t('page.not_found_body', "The page you're looking for doesn't exist or has been moved.")
			: page.error?.message ?? t('page.server_error_body', 'Something went wrong. Please try again later.')
	);
</script>

<svelte:head>
	<title>{title} — {t('app.name', 'JapaConnect Docs')}</title>
</svelte:head>

<AppShell
	nav={data.nav}
	accessibleAudiences={data.accessibleAudiences}
	logoUrl={(data.siteSettings?.["appearance.logo_url"] as string | undefined) ?? ""}
	footerMenu={data.footerMenu}
	rightSidebarWidgets={data.rightSidebarWidgets}
	activeAudience={null}
	user={data.user}
	collapsed={collapsed}
	onToggleCollapse={toggleCollapse}
>
	<div class="flex min-h-[60vh] flex-col items-center justify-center px-4 text-center">
		<div class="mb-4 flex items-center justify-center">
			<AlertTriangle class="size-12 {isNotFound ? 'text-[var(--color-muted)]' : 'text-[var(--destructive)]'}" />
		</div>
		<h1 class="text-foreground mb-2 text-5xl font-bold">{status}</h1>
		<h2 class="text-foreground mb-3 text-xl font-semibold">{title}</h2>
		<p class="text-muted-foreground mb-8 max-w-md text-sm">{message}</p>
		<a
			href="/docs"
			class="bg-foreground text-background hover:bg-foreground/90 inline-flex items-center gap-2 rounded-lg px-5 py-2 text-sm font-medium transition-colors"
		>
			{t('page.back_to_docs', 'Back to docs')}
		</a>
	</div>
</AppShell>
