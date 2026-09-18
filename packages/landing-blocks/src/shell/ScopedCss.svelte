<script lang="ts">
  // Injects a raw CSS string into the document head as a <style> element.
  // Extracted into its own component because svelte-check's TSX transform
  // cannot handle `{@html '<style>...'}` inside components that also declare
  // a real <style> block — this file deliberately has none.
  interface Props {
    css: string | null | undefined;
  }
  let { css }: Props = $props();
</script>

<svelte:head>
  {@html css ? '<style>' + css + '<\/style>' : ''}
</svelte:head>
