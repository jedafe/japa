<script lang="ts">
	// §09: incomplete-onboarding reminder card. Shown at the top of the home
	// feed until the wizard is finished; the X button snoozes it for the
	// browser session (it reappears on the next visit while incomplete).
	import { page } from '$app/state';
	import { browser } from '$app/environment';
	import { X, ArrowRight } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	const SNOOZE_KEY = 'jc:onboarding_banner_snoozed';
	const TOTAL_STEPS = 5; // mirrors internal/onboarding/service.go totalSteps

	let snoozed = $state(browser && sessionStorage.getItem(SNOOZE_KEY) === '1');

	// browser gate: skip SSR entirely — a session-snoozed user would
	// otherwise see the card flash on every page load before hydration
	// reads sessionStorage.
	const visible = $derived(
		browser && !!page.data.user && page.data.onboardingCompleted === false && !snoozed
	);
	const step = $derived((page.data.onboardingStep as number) ?? 1);

	function snooze() {
		sessionStorage.setItem(SNOOZE_KEY, '1');
		snoozed = true;
	}
</script>

{#if visible}
	<div
		class="mb-3 flex items-center gap-3 rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
		role="status"
	>
		<div class="min-w-0 flex-1">
			<p class="text-sm font-medium text-[var(--color-text)]">
				{t('onboarding.reminder_title', 'Finish setting up your account')}
			</p>
			<p class="mt-0.5 text-xs text-[var(--color-muted)]">
				{t('onboarding.reminder_progress', 'Step {step} of {total}', { step, total: TOTAL_STEPS })}
			</p>
			<div class="mt-2 h-1 overflow-hidden rounded-full bg-[var(--color-surface-subtle)]">
				<div
					class="h-full rounded-full bg-[var(--color-primary)] transition-all"
					style="width: {Math.min(100, Math.max(0, ((step - 1) / TOTAL_STEPS) * 100))}%"
				></div>
			</div>
		</div>
		<a
			href="/onboarding"
			class="flex shrink-0 items-center gap-1 rounded-[var(--radius)] bg-[var(--color-primary)] px-3 py-1.5 text-xs font-medium text-[var(--color-primary-contrast)] hover:opacity-90"
		>
			{t('onboarding.reminder_cta', 'Continue setup')}
			<ArrowRight size={14} />
		</a>
		<button
			type="button"
			onclick={snooze}
			class="shrink-0 rounded-full p-1 text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]"
			aria-label={t('onboarding.reminder_dismiss', 'Dismiss reminder')}
		>
			<X size={14} />
		</button>
	</div>
{/if}
