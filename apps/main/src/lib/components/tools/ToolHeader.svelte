<script lang="ts">
	import type { IconComponent } from '$lib/components/ui';

	interface Props {
		icon: IconComponent;
		title: string;
		description?: string;
	}

	let { icon: Icon, title, description }: Props = $props();
</script>

<!-- Tool page header: icon in a rounded chip beside a title + optional
	 description. Mobile-first: compact on small screens, comfortable on
	 desktop. Replaces the old bare `<Icon size={20}/> text-lg` heading. -->
<div class="mb-5 flex items-center gap-3 sm:mb-6 sm:gap-4">
	<div
		class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius-card)] bg-[var(--color-surface-subtle)] text-[var(--color-primary)] sm:h-12 sm:w-12"
	>
		<Icon size={20} />
	</div>
	<div class="min-w-0">
		<h1 class="truncate text-base font-semibold leading-tight text-[var(--color-text)] sm:text-lg">
			{title}
		</h1>
		{#if description}
			<p class="mt-0.5 line-clamp-2 text-xs text-[var(--color-muted)] sm:text-sm">{description}</p>
		{/if}
	</div>
</div>
