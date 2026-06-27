<script lang="ts">
	import { ArrowRight, CheckCircle, Compass, PenLine, Users } from 'lucide-svelte';

	let { oncomplete, onskip, submitting } = $props();

	const actions = [
		{
			id: 'create',
			icon: PenLine,
			title: 'Create your first post',
			description: 'Share your experience or ask a question'
		},
		{
			id: 'explore',
			icon: Compass,
			title: 'Explore the feed',
			description: 'Discover content from the community'
		},
		{
			id: 'join',
			icon: Users,
			title: 'Join a space',
			description: 'Find a community that matches your interests'
		}
	];
</script>

<div class="space-y-6">
	<div class="flex items-center gap-3">
		<div class="flex h-10 w-10 items-center justify-center rounded-full" style="background: var(--color-success); color: white;">
			<CheckCircle size={20} />
		</div>
		<div>
			<h2 class="text-xl font-semibold">You're all set!</h2>
			<p class="text-sm" style="color: var(--color-muted)">Here are some things you can do next</p>
		</div>
	</div>

	<div class="space-y-3">
		{#each actions as action}
			<button
				class="flex w-full items-center gap-4 rounded-lg border p-4 text-left transition-all hover:opacity-80"
				style="border-color: var(--color-border); background: var(--color-background);"
				onclick={oncomplete}
			>
				<div
					class="flex h-10 w-10 items-center justify-center rounded-lg"
					style="background: var(--color-surface); color: var(--color-primary);"
				>
					<action.icon size={20} />
				</div>
				<div class="flex-1">
					<p class="text-sm font-medium">{action.title}</p>
					<p class="text-xs" style="color: var(--color-muted)">{action.description}</p>
				</div>
				<ArrowRight size={16} style="color: var(--color-muted)" />
			</button>
		{/each}
	</div>

	<div class="flex flex-col gap-3 mt-6">
		<button
			class="w-full rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90 disabled:opacity-50"
			style="background: var(--color-primary)"
			onclick={oncomplete}
			disabled={submitting}
		>
			Go to my feed
		</button>
		<button
			class="w-full rounded-lg px-4 py-2.5 text-sm font-medium transition-colors hover:opacity-80"
			style="color: var(--color-muted)"
			onclick={onskip}
			disabled={submitting}
		>
			Skip for now
		</button>
	</div>
</div>
