<script lang="ts">
	let {
		width = '100%',
		height = '20px',
		radius = '4px',
		class: className = ''
	}: {
		width?: string;
		height?: string;
		radius?: string;
		class?: string;
	} = $props();
</script>

<div
	class="jc-skeleton {className}"
	style="width: {width}; height: {height}; border-radius: {radius};"
	aria-hidden="true"
></div>
