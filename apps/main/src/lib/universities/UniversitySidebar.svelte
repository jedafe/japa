<script lang="ts">
	import { Globe, DollarSign, BookOpen, Award, ExternalLink } from 'lucide-svelte';

	let {
		fields = []
	}: {
		fields: Array<{
			id: string;
			name: string;
			value: any;
			field_type?: string;
		}>;
	} = $props();

	const fieldIcons: Record<string, any> = {
		ranking: Award,
		tuition: DollarSign,
		programs: BookOpen,
		website: Globe
	};

	function getIcon(name: string) {
		const lower = name.toLowerCase();
		for (const [key, icon] of Object.entries(fieldIcons)) {
			if (lower.includes(key)) return icon;
		}
		return Globe;
	}

	function formatValue(value: any): string {
		if (value === null || value === undefined) return '—';
		if (typeof value === 'boolean') return value ? 'Yes' : 'No';
		if (Array.isArray(value)) return value.join(', ');
		return String(value);
	}
</script>

<aside class="space-y-4">
	<h3 class="text-lg font-semibold" style="color: var(--color-foreground)">Key Information</h3>

	{#if fields.length > 0}
		<div class="space-y-3">
			{#each fields as field}
				{@const Icon = getIcon(field.name)}
				<div class="rounded-lg p-3" style="background-color: var(--color-background)">
					<div class="flex items-center gap-2 text-sm" style="color: var(--color-muted)">
						<Icon class="h-4 w-4" />
						{field.name}
					</div>
					<div class="mt-1 font-medium" style="color: var(--color-foreground)">
						{formatValue(field.value)}
					</div>
				</div>
			{/each}
		</div>
	{:else}
		<p class="text-sm" style="color: var(--color-muted)">
			No additional information available.
		</p>
	{/if}
</aside>
