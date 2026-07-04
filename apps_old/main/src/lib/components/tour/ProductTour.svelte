<script lang="ts">
	import { X, ArrowRight } from 'lucide-svelte';

	let { open = false, onclose } = $props();

	let currentStep = $state(0);

	const tourSteps = [
		{
			target: 'nav',
			title: 'Navigation',
			description: 'Use the sidebar to switch between spaces and access your feed.'
		},
		{
			target: 'space-switcher',
			title: 'Space Switcher',
			description: 'Switch between different community spaces from here.'
		},
		{
			target: 'create-btn',
			title: 'Create Content',
			description: 'Tap here to create a new post, article, or question.'
		}
	];

	function nextStep() {
		if (currentStep < tourSteps.length - 1) {
			currentStep++;
		} else {
			close();
		}
	}

	function close() {
		currentStep = 0;
		onclose();
	}
</script>

{#if open}
	<div class="fixed inset-0 z-50 flex items-center justify-center" style="background: rgba(0,0,0,0.5);">
		<div
			class="mx-4 w-full max-w-sm rounded-xl border p-6 shadow-xl"
			style="border-color: var(--color-border); background: var(--color-background);"
			role="dialog"
			aria-label="Product tour"
		>
			<div class="mb-4 flex items-center justify-between">
				<span class="text-xs font-medium" style="color: var(--color-muted)">
					Step {currentStep + 1} of {tourSteps.length}
				</span>
				<button
					class="rounded-lg p-1 transition-colors hover:opacity-70"
					style="color: var(--color-muted)"
					onclick={close}
					aria-label="Close tour"
				>
					<X size={18} />
				</button>
			</div>

			<div class="mb-6">
				<h3 class="text-lg font-semibold">{tourSteps[currentStep].title}</h3>
				<p class="mt-2 text-sm" style="color: var(--color-muted)">
					{tourSteps[currentStep].description}
				</p>
			</div>

			<div class="flex gap-2">
				<button
					class="flex-1 rounded-lg px-4 py-2.5 text-sm font-medium transition-colors hover:opacity-80"
					style="color: var(--color-muted)"
					onclick={close}
				>
					Skip tour
				</button>
				<button
					class="flex flex-1 items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium text-white transition-colors hover:opacity-90"
					style="background: var(--color-primary)"
					onclick={nextStep}
				>
					{currentStep === tourSteps.length - 1 ? 'Finish' : 'Next'}
					<ArrowRight size={16} />
				</button>
			</div>
		</div>
	</div>
{/if}
