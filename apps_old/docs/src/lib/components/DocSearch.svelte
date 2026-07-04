<script lang="ts">
	import { t } from '$lib/stores/i18n.svelte';
	import { Search } from 'lucide-svelte';

	let query = $state('');
</script>

<div class="relative mb-8">
	<Search class="pointer-events-none absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2" style="color: var(--color-muted)" />
	<input
		type="text"
		bind:value={query}
		placeholder={t('search.placeholder')}
		class="w-full rounded-lg border py-3 pl-10 pr-4 text-base outline-none focus:ring-2 focus:ring-blue-500"
		style="border-color: var(--color-border); background: var(--color-surface)"
	/>
</div>
