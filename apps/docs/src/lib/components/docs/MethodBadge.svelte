<script lang="ts">
	interface Props {
		method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
	}

	let { method }: Props = $props();
</script>

<span class="method-badge method-{method}">{method}</span>
