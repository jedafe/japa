<script lang="ts">
	import PageHeader from '$lib/components/PageHeader.svelte';
</script>

<PageHeader title="Queue Monitor" description="Asynq task queue depth and recent history" />

<div class="rounded-lg border p-8 text-center" style="border-color: var(--color-border); background-color: var(--color-surface)">
	<p class="text-sm font-medium" style="color: var(--color-muted)">Queue monitoring API is not implemented yet.</p>
	<p class="mt-1 text-xs" style="color: var(--color-muted)">This endpoint will expose Asynq queue metrics once the backend is wired.</p>
</div>
