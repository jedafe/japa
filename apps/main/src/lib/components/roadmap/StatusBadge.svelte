<script lang="ts">
	// Ref: japaconnect-engineering/sections/113-public-product-roadmap.md (Status Lifecycle)
	// Pill badge mapping a roadmap item status to an i18n label + colour.
	import { t } from '$lib/i18n';
	import { roadmapStatusLabel } from '$lib/enumOptions';
	import type { RoadmapStatus } from '$lib/types';

	interface Props {
		status: RoadmapStatus | string;
	}
	let { status }: Props = $props();

	const tone = $derived.by(() => {
		switch (status) {
			case 'under_review':
				return 'bg-amber-500/10 text-amber-600 border-amber-500/30';
			case 'planned':
				return 'bg-sky-500/10 text-sky-600 border-sky-500/30';
			case 'in_progress':
				return 'bg-[var(--color-primary)]/10 text-[var(--color-primary)] border-[var(--color-primary)]/30';
			case 'shipped':
				return 'bg-green-500/10 text-green-600 border-green-500/30';
			case 'declined':
				return 'bg-[var(--color-danger)]/10 text-[var(--color-danger)] border-[var(--color-danger)]/30';
			case 'archived':
				return 'bg-[var(--color-muted)]/10 text-[var(--color-muted)] border-[var(--color-border)]';
			default: // pending
				return 'bg-[var(--color-surface-subtle)] text-[var(--color-muted)] border-[var(--color-border)]';
		}
	});
</script>

<span
	class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium {tone}"
>
	{roadmapStatusLabel(t, status)}
</span>
