<script lang="ts">
	import type { RichTextProps } from '../types';

	interface Props {
		props: RichTextProps;
	}
	let { props }: Props = $props();
</script>

<section class="mx-auto max-w-3xl px-6 py-10">
	<!-- Sanitized HTML from the backend renderer (Section 14, §5.1). -->
	{@html props.content}
</section>
