<script lang="ts">
	import { Switch } from 'bits-ui';

	interface Props {
		checked?: boolean;
		id?: string;
		disabled?: boolean;
		onCheckedChange?: (checked: boolean) => void;
		class?: string;
	}

	let { checked = $bindable(false), id, disabled = false, onCheckedChange, class: className = '' }: Props = $props();
</script>

<Switch.Root
	{id}
	bind:checked
	{disabled}
	{onCheckedChange}
	class="bg-muted peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full px-[2px] transition-colors disabled:cursor-not-allowed disabled:opacity-50 data-[state=checked]:bg-emerald-500 {className}"
>
	<Switch.Thumb
		class="bg-background pointer-events-none block size-4 shrink-0 rounded-full shadow-lg transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0"
	/>
</Switch.Root>
