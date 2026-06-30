<script lang="ts">
	import '../app.css';
	import { t } from '$lib/stores/i18n.svelte';
	import DocSidebar from '$lib/components/DocSidebar.svelte';
	import { page } from '$app/state';

	let { children } = $props();

	// Derive audience from URL params (top-level segment after /docs or root)
	let audience = $derived(() => {
		const segs = page.url.pathname.split('/').filter(Boolean);
		return segs[0] || '';
	});
</script>

<div class="flex min-h-screen">
	<DocSidebar audience={audience()} />

	<div class="flex-1 overflow-auto">
		<!-- Header -->
		<header class="border-b px-8 py-4" style="border-color: var(--color-border)">
			<div class="flex items-center justify-between">
				<h1 class="text-sm font-semibold" style="color: var(--color-muted)">
					{t('docs.brand')}
				</h1>
				<div class="flex items-center gap-4 text-sm" style="color: var(--color-muted)">
					<a href="/" class="hover:underline">{t('nav.home')}</a>
				</div>
			</div>
		</header>

		<!-- Main content -->
		<main class="px-8 py-8">
			{@render children()}
		</main>

		<!-- Footer -->
		<footer class="border-t px-8 py-4 text-center text-xs" style="border-color: var(--color-border); color: var(--color-muted)">
			{t('footer.copyright')}
		</footer>
	</div>
</div>
