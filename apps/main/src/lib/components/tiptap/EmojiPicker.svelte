<script lang="ts">
	/**
	 * Lightweight emoji picker popover for TipTap editors.
	 * Inserts the selected emoji as text at the cursor position.
	 * No external dependency — uses a curated set of common emojis.
	 *
	 * Ref: japaconnect-engineering/sections/15-tiptap-rich-text.md
	 */

	import { Smile } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		/** Called with the selected emoji character. */
		onselect: (emoji: string) => void;
	}

	let { onselect }: Props = $props();

	let open = $state(false);
	let buttonEl: HTMLButtonElement;
	let popoverEl: HTMLDivElement | undefined = $state(undefined);
	let popoverStyle = $state('');

	function computePosition() {
		if (!buttonEl) return;
		const rect = buttonEl.getBoundingClientRect();
		const popoverWidth = 288; // w-72 = 18rem = 288px
	 const viewportWidth = window.innerWidth;
		// Anchor to the right edge of the button, but clamp within viewport.
		let left = rect.right - popoverWidth;
		if (left < 8) left = 8;
		if (left + popoverWidth > viewportWidth - 8) left = viewportWidth - popoverWidth - 8;
		// Position above the button; if not enough room, place below.
		const topAbove = rect.top - 8;
		const topBelow = rect.bottom + 8;
		const top = topAbove > 200 ? topAbove : topBelow;
		popoverStyle = `position:fixed;left:${left}px;top:${top}px;z-index:9999;`;
	}

	const categories = [
		{
			label: 'Smileys',
			emojis: ['😀', '😃', '😄', '😁', '😆', '😅', '🤣', '😂', '🙂', '😉', '😊', '😇', '🥰', '😍', '🤩', '😘', '😗', '😚', '😙', '🥲', '😋', '😛', '😜', '🤪', '😝', '🤑', '🤗', '🤭', '🤫', '🤔', '🤐', '🤨', '😐', '😑', '😶', '😏', '😒', '🙄', '😬', '😮‍💨', '🥱', '😴', '🤤', '😪', '😵', '🤯', '🥳', '🤠', '😎', '🤓', '🧐', '😕', '😟', '🙁', '☹️', '😮', '😯', '😲', '😳', '🥺', '😦', '😧', '😨', '😰', '😥', '😢', '😭', '😱', '😖', '😣', '😞', '😓', '😩', '😫', '🥹', '😤', '😡', '😠', '🤬', '😈', '👿', '💀', '☠️', '💩', '🤡', '👹', '👺', '👻', '👽', '👾', '🤖']
		},
		{
			label: 'Gestures',
			emojis: ['👍', '👎', '👌', '🤌', '🤏', '✌️', '🤞', '🤟', '🤘', '🤙', '👈', '👉', '👆', '👇', '☝️', '✋', '🤚', '🖐️', '🖖', '👋', '🤝', '🙏', '✍️', '💪', '🦾', '🦿', '🦵', '🦶', '👂', '🦻', '👃', '🧠', '🫀', '🦷', '🦴', '👀', '👁️', '👅', '👄', '💋', '🩷', '❤️', '🧡', '💛', '💚', '💙', '🩵', '💜', '🤎', '🖤', '🩶', '🤍', '❤️‍🔥', '❤️‍🩹', '❣️', '💔', '❤️‍🔥', '💯', '💢', '💥', '💫', '💦', '💨', '🕳️', '💣', '💬', '👁️‍🗨️', '🗨️', '🗯️', '💭', '💤']
		},
		{
			label: 'Objects',
			emojis: ['⭐', '🌟', '✨', '⚡', '🔥', '🌈', '☀️', '⛅', '☁️', '🌧️', '⛈️', '🌩️', '🌨️', '❄️', '☃️', '⛄', '🌬️', '💨', '💧', '💦', '🌊', '🎉', '🎊', '🎈', '🎁', '🏆', '🥇', '🥈', '🥉', '🏅', '🎖️', '🎗️', '🎫', '🎟️', '🎪', '🎭', '🎨', '🎬', '🎤', '🎧', '🎼', '🎵', '🎶', '🎹', '🥁', '🎷', '🎺', '🎸', '🪕', '🎻', '🎲', '🎯', '🎳', '🎮', '🎰', '🧩', '📚', '📖', '📝', '✏️', '✒️', '🖋️', '🖊️', '🖌️', '🖍️', '📌', '📍', '📎', '✂️', '📐', '📏', '🧮', '🔧', '🔨', '⚙️', '💡', '🔦', '🕯️', '🪔', '🧯', '🛒', '🛍️', '💰', '💳', '💎']
		},
		{
			label: 'Food',
			emojis: ['🍎', '🍐', '🍊', '🍋', '🍌', '🍉', '🍇', '🍓', '🫐', '🍈', '🍒', '🍑', '🥭', '🍍', '🥥', '🥝', '🍅', '🍆', '🥑', '🥦', '🥬', '🥒', '🌶️', '🫑', '🌽', '🥕', '🫒', '🧄', '🧅', '🥔', '🍠', '🥐', '🥯', '🍞', '🥖', '🥨', '🧀', '🥚', '🍳', '🧈', '🥞', '🧇', '🥓', '🥩', '🍗', '🍖', '🌭', '🍔', '🍟', '🍕', '🥪', '🌮', '🌯', '🥙', '🧆', '🥗', '🍝', '🍜', '🍲', '🍛', '🍣', '🍱', '🥟', '🦪', '🍤', '🍙', '🍚', '🍘', '🍥', '🥠', '🥮', '🍢', '🍡', '🍧', '🍨', '🍦', '🥧', '🧁', '🍰', '🎂', '🍮', '🍭', '🍬', '🍫', '🍿', '🍩', '🍪', '🌰', '🥜', '🍯', '🥛', '☕', '🍵', '🧃', '🥤', '🍶', '🍺', '🍻', '🥂', '🍷', '🥃', '🍸', '🍹', '🧉', '🍾']
		},
		{
			label: 'Activities',
			emojis: ['⚽', '🏀', '🏈', '⚾', '🥎', '🎾', '🏐', '🏉', '🥏', '🎱', '🪀', '🏓', '🏸', '🥅', '🏒', '🏑', '🥍', '🏏', '🪃', '🥊', '🥋', '⛳', '🪁', '🏹', '🎣', '🤿', '🥊', '🎽', '🛹', '🛼', '🛷', '⛸️', '🥌', '🎿', '⛷️', '🏂', '🪂', '🏋️', '🤼', '🤸', '🤺', '⛹️', '🤾', '🏌️', '🏇', '🧘', '🏄', '🏊', '🤽', '🚣', '🧗', '🚵', '🚴', '🏆', '🥇', '🥈', '🥉', '🏅', '🎖️', '🎉', '🎊', '🎈', '🎁', '🎗️', '🎟️', '🎫', '🎪', '🎭', '🎨', '🎬', '🎤', '🎧', '🎼', '🎹', '🥁', '🎷', '🎺', '🎸', '🪕', '🎻', '🎲', '🎯', '🎳', '🎮', '🎰', '🧩']
		},
		{
			label: 'Symbols',
			emojis: ['✅', '❌', '❎', '✔️', '✖️', '➕', '➖', '➗', '✖️', '♾️', '‼️', '⁉️', '❓', '❔', '❕', '❗', '〰️', '♻️', '⚠️', '🚸', '🔱', '⚜️', '🔰', '⭕', '✅', '❌', '❎', '🆗', '🆕', '🆙', '🆒', '🆓', '🆖', '🆔', '🉐', '🈁', '🈂️', '🈚', '🈯', '🈲', '🈳', '🈴', '🈵', '🈶', '🈷️', '🈸', '🈹', '🈺', '🉑', '㊗️', '㊙️', '🅰️', '🅱️', '🆎', '🆑', '🅾️', '🆘', '🆙', '🆚', '🅿️', '🈁', '🔴', '🟠', '🟡', '🟢', '🔵', '🟣', '⚫', '⚪', '🟥', '🟧', '🟨', '🟩', '🟦', '🟪', '⬛', '⬜']
		}
	];

	let activeCategory = $state(0);

	function toggle(event: MouseEvent) {
		event.stopPropagation();
		if (!open) computePosition();
		open = !open;
	}

	function select(emoji: string) {
		onselect(emoji);
		open = false;
	}

	function handleOutsideClick(event: MouseEvent) {
		if (open && popoverEl && buttonEl) {
			if (!popoverEl.contains(event.target as Node) && !buttonEl.contains(event.target as Node)) {
				open = false;
			}
		}
	}

	$effect(() => {
		if (open) {
			document.addEventListener('click', handleOutsideClick);
		}
		return () => document.removeEventListener('click', handleOutsideClick);
	});
</script>

<div class="relative inline-block">
	<button
		type="button"
		bind:this={buttonEl}
		onclick={toggle}
		class="grid place-items-center rounded p-1 transition-colors text-[var(--color-muted)] hover:text-[var(--color-text)]"
		aria-label={t('create.editor.toolbar.emoji', 'Emoji')}
		title={t('create.editor.toolbar.emoji', 'Emoji')}
	>
		<Smile size={14} />
	</button>

	{#if open}
		<div
			bind:this={popoverEl}
			style={popoverStyle}
			class="w-72 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] shadow-lg"
		>
			<!-- Category tabs -->
			<div class="flex flex-wrap gap-1 border-b border-[var(--color-border)] px-2 py-1.5">
				{#each categories as cat, i}
					<button
						type="button"
						onclick={() => (activeCategory = i)}
						class="rounded px-1.5 py-0.5 text-[10px] font-medium whitespace-nowrap transition-colors {activeCategory === i ? 'bg-[var(--color-primary)]/10 text-[var(--color-primary)]' : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>
						{cat.label}
					</button>
				{/each}
			</div>
			<!-- Emoji grid -->
			<div class="max-h-48 overflow-y-auto p-2">
				<div class="grid grid-cols-8 gap-0.5">
					{#each categories[activeCategory].emojis as emoji}
						<button
							type="button"
							onclick={() => select(emoji)}
							class="grid place-items-center rounded p-1 text-lg leading-none transition-colors hover:bg-[var(--color-surface-subtle)]"
						>
							{emoji}
						</button>
					{/each}
				</div>
			</div>
		</div>
	{/if}
</div>
