<script lang="ts">
	import CreateShell from '$lib/components/create/CreateShell.svelte';
	import { PenSquare } from 'lucide-svelte';
	import { EmptyState } from '$lib/components/ui';
	import { t } from '$lib/i18n';

	let { data } = $props();
</script>

{#if data.loadError}
	<div class="mx-auto w-full max-w-2xl py-10">
		<EmptyState
			icon={PenSquare}
			title={t('error.500_title')}
			body={t('error.500_body')}
			action={{ label: t('empty.action_home'), href: '/' }}
		/>
	</div>
{:else}
	<CreateShell
		types={data.types}
		spaces={data.spaces}
		fieldGroups={data.fieldGroups}
		series={data.series}
		draft={data.draft}
		user={data.user}
	/>
{/if}
