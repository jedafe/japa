<script lang="ts">
	import { goto } from '$app/navigation';
	import StepWelcome from '$lib/onboarding/StepWelcome.svelte';
	import StepDestination from '$lib/onboarding/StepDestination.svelte';
	import StepInterests from '$lib/onboarding/StepInterests.svelte';
	import StepSpaces from '$lib/onboarding/StepSpaces.svelte';
	import StepFirstAction from '$lib/onboarding/StepFirstAction.svelte';
	import OnboardingProgress from '$lib/onboarding/OnboardingProgress.svelte';

	let { data } = $props();

	let currentStep = $state(data.onboarding.current_step);
	let stepData = $state<Record<string, unknown>>(data.onboarding.step_data || {});
	let submitting = $state(false);

	const steps = [
		{ number: 1, label: 'Identity' },
		{ number: 2, label: 'Destination' },
		{ number: 3, label: 'Interests' },
		{ number: 4, label: 'Spaces' },
		{ number: 5, label: 'First Steps' }
	];

	function updateStepData(newData: Record<string, unknown>) {
		stepData = { ...stepData, ...newData };
	}

	async function nextStep() {
		submitting = true;
		try {
			await fetch('/api/v1/onboarding/step', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ step: currentStep + 1, step_data: stepData })
			});

			currentStep = Math.min(currentStep + 1, 5);
		} finally {
			submitting = false;
		}
	}

	async function skipStep() {
		await nextStep();
	}

	async function completeOnboarding() {
		submitting = true;
		try {
			await fetch('/api/v1/onboarding/complete', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' }
			});

			goto('/');
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Welcome to JapaConnect</title>
	<meta name="description" content="Complete your onboarding to get the most out of JapaConnect." />
</svelte:head>

<main class="mx-auto max-w-2xl px-6 py-12">
	<OnboardingProgress {currentStep} {steps} />

	<div class="mt-10 rounded-xl border p-8" style="border-color: var(--color-border); background: var(--color-surface);">
		{#if currentStep === 1}
			<StepWelcome
				{stepData}
				onupdate={updateStepData}
				onnext={nextStep}
				{submitting}
			/>
		{:else if currentStep === 2}
			<StepDestination
				{stepData}
				onupdate={updateStepData}
				onnext={nextStep}
				onback={() => (currentStep = 1)}
				{submitting}
			/>
		{:else if currentStep === 3}
			<StepInterests
				{stepData}
				onupdate={updateStepData}
				onnext={nextStep}
				onback={() => (currentStep = 2)}
				{submitting}
			/>
		{:else if currentStep === 4}
			<StepSpaces
				{stepData}
				onnext={nextStep}
				onback={() => (currentStep = 3)}
			/>
		{:else if currentStep === 5}
			<StepFirstAction
				oncomplete={completeOnboarding}
				onskip={completeOnboarding}
				{submitting}
			/>
		{/if}
	</div>

	{#if currentStep < 5}
		<div class="mt-4 text-center">
			<button
				class="text-sm underline"
				style="color: var(--color-muted)"
				onclick={skipStep}
				disabled={submitting}
			>
				Skip for now
			</button>
		</div>
	{/if}
</main>
