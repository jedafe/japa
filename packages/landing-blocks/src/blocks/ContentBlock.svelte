<script lang="ts">
  import type { ContentProps } from '../types';

  interface Props {
    props: ContentProps;
  }
  let { props }: Props = $props();

  const proseClass = $derived(
    props.prose_style === 'minimal' ? 'prose prose-sm max-w-none dark:prose-invert' :
    props.prose_style === 'article' ? 'prose prose-lg max-w-3xl mx-auto dark:prose-invert' :
    props.prose_style === 'documentation' ? 'prose prose-sm max-w-4xl mx-auto dark:prose-invert' :
    'prose prose-lg max-w-none dark:prose-invert'
  );
</script>

<section class="px-6 py-16">
  <div class="mx-auto max-w-4xl">
    <div class={proseClass}>
      {@html props.html}
    </div>
  </div>
</section>
