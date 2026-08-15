<script lang="ts">
	// Ref: japaconnect-engineering/sections/41-spam-content-safety.md (Layer 2)
	// Honeypot field — a visually hidden input that real users will never
	// fill, but spam bots auto-filling all fields will. The backend rejects
	// any submission where this field is non-empty (error code
	// `honeypot_triggered`). The field name `website` is deliberately
	// generic to lure bots into filling it.
	//
	// Accessibility: the input is hidden from screen readers and keyboard
	// navigation via tabindex=-1, aria-hidden, and autocomplete=off. The
	// sr-only class is NOT used because some bots strip it; instead the
	// field is positioned off-screen and made invisible.
	let {
		value = $bindable('')
	}: {
		value?: string;
	} = $props();
</script>

<input
	type="text"
	name="website"
	autocomplete="off"
	tabindex={-1}
	aria-hidden="true"
	bind:value
	style="position:absolute;left:-9999px;top:-9999px;width:1px;height:1px;opacity:0;pointer-events:none"
/>
