<script lang="ts">
	import { page } from '$app/state';
	import { FileQuestion, ShieldX, TriangleAlert } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { t } from '$lib/i18n';

	const status = $derived(page.status);
	const meta = $derived(
		status === 404
			? { icon: FileQuestion, title: t('error.404_title'), body: t('error.404_body') }
			: status === 403
				? { icon: ShieldX, title: t('error.403_title'), body: t('error.403_body') }
				: { icon: TriangleAlert, title: t('error.500_title'), body: t('error.500_body') }
	);
</script>

<svelte:head>
	<title>{meta.title} — {t('app.name', 'JapaConnect')}</title>
</svelte:head>

<div class="grid min-h-screen place-items-center bg-[var(--color-bg)] px-4">
	<EmptyState
		icon={meta.icon}
		title={meta.title}
		body={page.error?.message ?? meta.body}
		action={{ label: t('empty.action_home'), href: '/' }}
	/>
</div>
