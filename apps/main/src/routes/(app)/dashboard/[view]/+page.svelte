<script lang="ts">
	import SeoHead from '#lib/seo/SeoHead.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { t } from '#lib/i18n/index.js';
	import { resolveView } from '#lib/components/hub/dashboard/dashboardViews.js';
	import { DASHBOARD_VIEW_COMPONENTS } from '#lib/components/hub/dashboard/dashboardComponents.js';

	let { data } = $props();
	const meta = $derived(resolveView(data.view, data.dashboardMenu?.items));
	const ViewComponent = $derived(DASHBOARD_VIEW_COMPONENTS[data.view] ?? null);

	// L2 — Client-side auth guard: if the user is not authenticated (e.g. session
	// expired during a client-side navigation), redirect to login immediately
	// rather than rendering a partial/empty dashboard view.
	$effect(() => {
		if (!page.data.user) {
			goto(`/login?redirect=${encodeURIComponent(page.url.pathname)}`);
		}
	});
</script>

<SeoHead title={`${t(meta?.label ?? 'dashboard.nav.label', 'Dashboard')} — ${t('app.name', '{site}')}`} description={t('dashboard.description', 'Your dashboard — overview, content, and activity on {site}.')} />

{#if ViewComponent && page.data.user}
	<ViewComponent data={data.viewData} />
{/if}
