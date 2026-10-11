<script lang="ts">
	import NewPageForm from '$lib/components/docs/NewPageForm.svelte';
	import type { PageData, ActionData } from './$types';

	let { data, form }: { data: PageData; form: ActionData } = $props();
</script>

<NewPageForm {data} {form} />
