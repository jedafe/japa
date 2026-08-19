<script lang="ts">
	import type { RichTextProps } from '../types';
	import { sanitizeContentHtml } from '$lib/utils/safeHtml';

	interface Props {
		props: RichTextProps;
	}
	let { props }: Props = $props();
</script>

<section class="mx-auto max-w-3xl px-6 py-10">
	<!-- Client-side defense-in-depth on top of the server-side sanitizer (Section 14, §5.1). -->
	{@html sanitizeContentHtml(props.content)}
</section>
