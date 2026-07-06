<script lang="ts">
	let { title, message }: { title: string; message: string } = $props();
</script>

<div data-testid="empty-state">
	<h3>{title}</h3>
	<p>{message}</p>
</div>
