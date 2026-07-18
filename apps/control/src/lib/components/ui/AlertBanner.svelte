<script lang="ts">
	import { AlertTriangle, Info, CheckCircle, XCircle } from 'lucide-svelte';

	let {
		variant = 'info',
		message,
		class: className = ''
	}: {
		variant?: 'info' | 'warning' | 'success' | 'error';
		message: string;
		class?: string;
	} = $props();

	const icons = {
		info: Info,
		warning: AlertTriangle,
		success: CheckCircle,
		error: XCircle
	};

	const colors = {
		info: 'border-accent text-accent-foreground bg-accent/50',
		warning: 'border-tertiary text-tertiary bg-tertiary/10',
		success: 'border-emerald-500 text-emerald-600 bg-emerald-500/10',
		error: 'border-destructive text-destructive bg-destructive/10'
	};

	const Icon = $derived(icons[variant]);
</script>

<div class="flex items-center gap-2 rounded-lg border px-4 py-3 text-sm {colors[variant]} {className}">
	<Icon class="size-4 flex-shrink-0" />
	<span>{message}</span>
</div>
