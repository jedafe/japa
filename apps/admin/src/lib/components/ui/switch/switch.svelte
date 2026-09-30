<script lang="ts">
	import { Switch } from "bits-ui";
	import { clsx, type ClassValue } from "clsx";

	let {
		checked = $bindable(false),
		onCheckedChange,
		disabled = false,
		ariaLabel,
		class: className
	}: {
		checked?: boolean;
		onCheckedChange?: (checked: boolean) => void;
		disabled?: boolean;
		ariaLabel?: string;
		class?: ClassValue;
	} = $props();
</script>

<Switch.Root
	bind:checked
	{onCheckedChange}
	{disabled}
	aria-label={ariaLabel}
	class={clsx(
		"relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
		checked ? "bg-primary" : "bg-surface",
		className
	)}
	style="--ring-color: var(--color-primary); --ring-offset-color: var(--color-surface)"
>
	<Switch.Thumb
		class={clsx(
			"inline-block h-4 w-4 transform rounded-full bg-surface shadow-sm transition-transform",
			checked ? "translate-x-6" : "translate-x-1"
		)}
	/>
</Switch.Root>
