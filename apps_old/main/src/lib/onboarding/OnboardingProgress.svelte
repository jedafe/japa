<script lang="ts">
	let { currentStep, steps } = $props();
</script>

<nav aria-label="Onboarding progress" class="flex items-center justify-between">
	{#each steps as step, i}
		<div class="flex items-center">
			<div class="flex flex-col items-center">
				<div
					class="flex h-8 w-8 items-center justify-center rounded-full text-xs font-semibold transition-all"
					style={currentStep > step.number
						? 'background: var(--color-success); color: white;'
						: currentStep === step.number
							? 'background: var(--color-primary); color: white;'
							: 'background: var(--color-surface); color: var(--color-muted);'}
				>
					{#if currentStep > step.number}
						&#10003;
					{:else}
						{step.number}
					{/if}
				</div>
				<span
					class="mt-1 text-xs font-medium hidden sm:block"
					style={currentStep >= step.number ? 'color: var(--color-foreground)' : 'color: var(--color-muted)'}
				>
					{step.label}
				</span>
			</div>
			{#if i < steps.length - 1}
				<div
					class="mx-2 h-0.5 w-8 sm:w-12"
					style={currentStep > step.number ? 'background: var(--color-success)' : 'background: var(--color-border)'}
				></div>
			{/if}
		</div>
	{/each}
</nav>
