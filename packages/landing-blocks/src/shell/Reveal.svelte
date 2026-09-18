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

  // Initial (hidden) state classes per preset — the "from" state.
  const hiddenStates: Record<string, string> = {
    'none': '',
    'fade-up': 'opacity-0 translate-y-8',
    'fade-in': 'opacity-0',
    'fade-down': 'opacity-0 -translate-y-8',
    'fly-in-left': 'opacity-0 -translate-x-16',
    'fly-in-right': 'opacity-0 translate-x-16',
    'fly-in-up': 'opacity-0 translate-y-16',
    'fly-in-down': 'opacity-0 -translate-y-16',
    'slide-in': 'opacity-0 -translate-x-12',
    'slide-in-left': 'opacity-0 -translate-x-12',
    'slide-in-right': 'opacity-0 translate-x-12',
    'scale-in': 'opacity-0 scale-95',
    'scale-out': 'opacity-0 scale-105',
    'zoom-in': 'opacity-0 scale-75',
    'zoom-out': 'opacity-0 scale-125',
    'blur-in': 'opacity-0 blur-md',
    'blur-out': 'opacity-0 blur-lg scale-105',
    'flip-up': 'opacity-0 [transform:rotateX(-90deg)] [transform-origin:top]',
    'flip-down': 'opacity-0 [transform:rotateX(90deg)] [transform-origin:bottom]',
    'bounce-in': 'opacity-0 scale-50',
  };

  // Visible (final) state classes — the "to" state.
  const visibleState = 'opacity-100 translate-x-0 translate-y-0 scale-100 blur-0 [transform:rotateX(0deg)]';

  const preset = $derived(motion?.preset ?? 'none');
  const durationMs = $derived(motion?.duration ?? 700);
  const easing = $derived(motion?.easing ?? 'cubic-bezier(0.16, 1, 0.3, 1)');
  const delayMs = $derived(motion?.delay ?? 0);
  const threshold = $derived(motion?.threshold ?? 0.1);
  const once = $derived(motion?.once ?? true);
  const stagger = $derived(!!motion?.stagger_children && preset !== 'none');
  const staggerDelay = $derived(motion?.stagger_delay ?? 80);

  const hiddenClass = $derived(hiddenStates[preset] ?? '');
  const stateClass = $derived(preset === 'none' || stagger ? '' : (visible ? visibleState : hiddenClass));
  const transitionStyle = $derived(
    preset === 'none' || stagger ? '' :
    `transition: opacity ${durationMs}ms ${easing} ${delayMs}ms, transform ${durationMs}ms ${easing} ${delayMs}ms, filter ${durationMs}ms ${easing} ${delayMs}ms;`
  );

  // In stagger mode the animation is applied per child element inside the
  // block's content, each offset by `stagger_delay`. Descend single-child
  // wrapper chains so the stagger hits the repeated items (cards, list
  // rows, columns) rather than layout wrappers.
  function staggerTargets(): HTMLElement[] {
    if (!el) return [];
    const shell = el.firstElementChild as HTMLElement | null;
    if (!shell) return [];
    const content = shell.lastElementChild as HTMLElement | null;
    if (!content) return [];
    let node = content;
    while (node.children.length === 1 && node.firstElementChild instanceof HTMLElement) {
      node = node.firstElementChild as HTMLElement;
    }
    const targets = Array.from(node.children) as HTMLElement[];
    return targets.length > 0 ? targets : [node];
  }

  function applyStagger() {
    if (!stagger || !el) return;
    const hidden = hiddenClass.split(' ').filter(Boolean);
    const shown = visibleState.split(' ').filter(Boolean);
    staggerTargets().forEach((t, i) => {
      const d = delayMs + i * staggerDelay;
      t.style.transition = `opacity ${durationMs}ms ${easing} ${d}ms, transform ${durationMs}ms ${easing} ${d}ms, filter ${durationMs}ms ${easing} ${d}ms`;
      t.style.willChange = 'transform';
      if (visible) {
        t.classList.remove(...hidden);
        t.classList.add(...shown);
      } else {
        t.classList.remove(...shown);
        t.classList.add(...hidden);
      }
    });
  }

  $effect(() => {
    // Re-apply whenever the visible flag flips (or stagger config changes).
    void visible;
    applyStagger();
  });

  onMount(() => {
    if (!el) return;
    // Respect prefers-reduced-motion — show immediately.
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      visible = true;
      return;
    }
    if (preset === 'none') {
      visible = true;
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            visible = true;
            if (once) observer.disconnect();
          } else if (!once) {
            visible = false;
          }
        }
      },
      { threshold }
    );
    observer.observe(el);
    return () => observer.disconnect();
  });
</script>

<div bind:this={el} class="{stateClass} {preset === 'none' || stagger ? '' : 'will-change-transform'}" style={transitionStyle}>
  {@render children()}
</div>
