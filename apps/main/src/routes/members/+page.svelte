<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';

	let { data } = $props();

	let members = $derived(data.members);
</script>

<svelte:head>
	<title>{t('members.title')} — JapaConnect</title>
</svelte:head>

<h1 class="mb-4 text-2xl font-bold tracking-tight">{t('members.title')}</h1>

<div class="overflow-x-auto rounded-lg border" style="border-color: var(--color-border)">
	<table class="w-full text-sm">
		<thead>
			<tr style="border-bottom: 1px solid var(--color-border)">
				<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('members.user')}</th>
				<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('members.role')}</th>
				<th class="px-4 py-3 text-left font-medium" style="color: var(--color-muted); background-color: var(--color-surface)">{t('members.joined')}</th>
			</tr>
		</thead>
		<tbody>
			{#each members as member (member.user_id)}
				<tr class="border-t" style="border-color: var(--color-border)">
					<td class="px-4 py-3">{member.user_id}</td>
					<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{member.role}</td>
					<td class="px-4 py-3 text-xs" style="color: var(--color-muted)">{new Date(member.joined_at).toLocaleDateString()}</td>
				</tr>
			{:else}
				<tr>
					<td colspan="3" class="px-4 py-8 text-center" style="color: var(--color-muted)">{t('members.empty')}</td>
				</tr>
			{/each}
		</tbody>
	</table>
</div>
