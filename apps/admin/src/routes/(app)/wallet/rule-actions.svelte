<script lang="ts">
	import { ActionCell } from '$lib/components/ui/data-table';
	import { Pencil, ToggleLeft, ToggleRight, Trash2 } from 'lucide-svelte';
	import { t } from '$lib/stores/i18n.svelte';
	import type { Component } from 'svelte';

	let { row, onEdit, onToggle, onDelete }: {
		row: any;
		onEdit: (row: any) => void;
		onToggle: (row: any) => void;
		onDelete: (row: any) => void;
	} = $props();

	const actions = $derived([
		{
			label: '',
			icon: Pencil as unknown as Component,
			onclick: () => onEdit(row),
			variant: 'ghost' as const
		},
		{
			label: '',
			icon: (row.is_enabled ? ToggleRight : ToggleLeft) as unknown as Component,
			onclick: () => onToggle(row),
			variant: 'ghost' as const
		},
		{
			label: '',
			icon: Trash2 as unknown as Component,
			onclick: () => onDelete(row),
			variant: 'destructive' as const
		}
	]);
</script>

<ActionCell {actions} />
