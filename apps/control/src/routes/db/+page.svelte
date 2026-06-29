<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
</script>

<PageHeader title="DB Health" description="Table bloat, dead tuples, and long-running queries" />

<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<p class="text-sm font-medium" style="color: var(--color-muted)">DB health monitoring API is not implemented yet.</p>
	<p class="mt-1 text-xs" style="color: var(--color-muted)">This endpoint will expose PostgreSQL metrics once the backend is wired.</p>
</div>
