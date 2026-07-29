<script lang="ts">
	import { AlertCircle, RefreshCw } from "lucide-svelte";
	import { t } from "$lib/stores/i18n.svelte";
	import { Button } from "$lib/components/ui/button";

	let {
		message = "",
		onretry,
		retryLabel
	}: {
		message?: string;
		onretry?: () => void;
		retryLabel?: string;
	} = $props();
</script>

<div
	class="flex flex-col items-center justify-center gap-3 rounded-lg border p-8 text-center"
	style="border-color: var(--color-danger); background-color: color-mix(in srgb, var(--color-danger) 5%, transparent)"
	role="alert"
>
	<AlertCircle class="h-8 w-8" style="color: var(--color-danger)" />
	<p class="text-sm font-medium" style="color: var(--color-danger)">
		{message || t("common.error", "Something went wrong")}
	</p>
	{#if onretry}
		<Button variant="outline" size="sm" onclick={onretry}>
			<RefreshCw class="h-3.5 w-3.5" />
			{retryLabel || t("common.retry", "Retry")}
		</Button>
	{/if}
</div>
