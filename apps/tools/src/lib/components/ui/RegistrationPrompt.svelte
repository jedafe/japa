<script lang="ts">
	import Icon from './Icon.svelte';
	import { mainAppUrl } from '$lib/seo';
	import { t } from '$lib/i18n';

	/**
	 * RegistrationPrompt — the modal shown when an anonymous user attempts a
	 * gated action (tick, save, submit, create alert). Links to the Main app's
	 * /signup and /login routes (the Main app owns auth — ADR-086 / §78).
	 *
	 * Accessibility: uses role="dialog" + aria-modal="true" (not role="button"),
	 * closes on Escape and backdrop click, and traps focus within the dialog.
	 * Ref: §2 (UI/UX accessibility), WCAG 2.1 dialog pattern.
	 */
	let {
		open = false,
		onClose,
		icon = 'user-plus',
		title,
		description
	}: {
		open?: boolean;
		onClose: () => void;
		icon?: string;
		title: string;
		description: string;
	} = $props();

	let dialogEl: HTMLDivElement | null = $state(null);
	let previouslyFocused: HTMLElement | null = null;

	// Focus trap: when the dialog opens, move focus into it and trap Tab
	// navigation within the dialog's focusable elements (WCAG 2.1 dialog
	// pattern). Restore focus to the trigger on close.
	function getFocusable(): HTMLElement[] {
		if (!dialogEl) return [];
		return Array.from(
			dialogEl.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input, [tabindex]:not([tabindex="-1"])')
		).filter((el) => el.offsetParent !== null);
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			e.stopPropagation();
			onClose();
			return;
		}
		if (e.key === 'Tab' && dialogEl) {
			const focusable = getFocusable();
			if (focusable.length === 0) return;
			const first = focusable[0];
			const last = focusable[focusable.length - 1];
			if (e.shiftKey && document.activeElement === first) {
				e.preventDefault();
				last.focus();
			} else if (!e.shiftKey && document.activeElement === last) {
				e.preventDefault();
				first.focus();
			}
		}
	}

	$effect(() => {
		if (open) {
			previouslyFocused = document.activeElement as HTMLElement;
			// Move focus into the dialog on open.
			queueMicrotask(() => {
				const focusable = getFocusable();
				if (focusable.length > 0) {
					focusable[0].focus();
				} else {
					dialogEl?.focus();
				}
			});
		} else if (previouslyFocused) {
			previouslyFocused.focus();
			previouslyFocused = null;
		}
	});
</script>

{#if open}
	<div
		class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
		onclick={onClose}
		onkeydown={onKeydown}
		role="presentation"
	>
		<div
			bind:this={dialogEl}
			class="bg-surface rounded-[var(--radius-card)] p-6 max-w-sm w-full text-center"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
			role="dialog"
			aria-modal="true"
			aria-labelledby="reg-prompt-title"
			tabindex="-1"
		>
			<div class="flex justify-center mb-4">
				<div class="w-12 h-12 rounded-full bg-primary/10 flex items-center justify-center">
					<Icon name={icon} size={24} class="text-primary" />
				</div>
			</div>
			<h3 id="reg-prompt-title" class="text-lg font-semibold text-text">{title}</h3>
			<p class="mt-2 text-sm text-text-subtle">{description}</p>
			<div class="mt-5 flex flex-col gap-2">
				<a
					href={mainAppUrl('/signup')}
					class="px-4 py-2.5 bg-primary text-primary-contrast rounded-lg text-sm font-medium hover:bg-primary-hover transition-colors"
				>
					{t('common.create_account', 'Create free account')}
				</a>
				<a
					href={mainAppUrl('/login')}
					class="px-4 py-2.5 text-sm font-medium text-text hover:text-primary transition-colors"
				>
					{t('common.sign_in', 'Sign in')}
				</a>
			</div>
		</div>
	</div>
{/if}
