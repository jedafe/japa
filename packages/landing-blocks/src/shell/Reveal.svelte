<script lang="ts">
  import type { MotionConfig } from '../types';
  import { onMount } from 'svelte';

  interface Props {
    motion?: MotionConfig;
    children: import('svelte').Snippet;
  }
  let { motion, children }: Props = $props();

  let el = $state<HTMLDivElement | null>(null);
  let visible = $state(false);

  const presetClasses: Record<string, string> = {
    'none': '',
    'fade-up': 'opacity-0 translate-y-4 transition-all duration-700',
    'fade-in': 'opacity-0 transition-opacity duration-700',
    'slide-in': 'opacity-0 -translate-x-4 transition-all duration-700',
    'scale-in': 'opacity-0 scale-95 transition-all duration-700',
  };

  onMount(() => {
    if (!el) return;
    // Respect prefers-reduced-motion — show immediately.
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      visible = true;
      return;
    }
    if (!motion || motion.preset === 'none') {
      visible = true;
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            visible = true;
            observer.disconnect();
          }
        }
      },
      { threshold: 0.1 }
    );
    observer.observe(el);
    return () => observer.disconnect();
  });

  const delayMs = $derived(motion?.delay ?? 0);
  const baseClass = $derived(motion?.preset ? presetClasses[motion.preset] ?? '' : '');
  const stateClass = $derived(visible ? 'opacity-100 translate-y-0 translate-x-0 scale-100' : baseClass);
</script>

<div bind:this={el} class={stateClass} style="transition-delay: {delayMs}ms">
  {@render children()}
</div>
