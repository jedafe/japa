<script lang="ts">
  import type { BlockStyle } from '../types';
  import type { Component } from 'svelte';
  import { Accordion } from 'bits-ui';
  import BackgroundPicker from './BackgroundPicker.svelte';
  import IconPicker from './IconPicker.svelte';

  type DeviceMode = 'desktop' | 'tablet' | 'mobile';

  interface Props {
    value?: BlockStyle;
    onchange: (v: BlockStyle) => void;
    mediaPickerComponent?: Component;
    /** Which device's style to edit. 'desktop' edits the base style; 'tablet'/'mobile' edit responsive overrides. */
    device?: DeviceMode;
  }
  let { value, onchange, mediaPickerComponent, device = 'desktop' }: Props = $props();

  const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';
  const labelClass = 'mb-1 block text-xs text-[var(--color-muted)]';
  const triggerClass = 'flex w-full items-center justify-between rounded-[var(--radius)] px-3 py-2 text-sm font-medium text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)] [&[data-state=open]>svg]:rotate-90';
  const contentClass = 'space-y-2 px-3 pb-3 pt-1';
  const resetClass = 'text-xs text-[var(--color-danger)] hover:underline';
  const itemClass = 'border-b border-[var(--color-border)]';

  type Opt = { value: string; label: string };

  // ── Per-device editing helpers ──────────────────────────────────────────
  // When device !== 'desktop', edits go into value.responsive[device] instead
  // of the base style. This lets the user configure tablet/mobile overrides
  // without affecting the desktop layout.

  /** The style object currently being edited (base or override). */
  const editingStyle = $derived(
    device === 'desktop'
      ? value
      : (value?.responsive?.[device] as BlockStyle | undefined)
  );

  function updateGroup(group: keyof BlockStyle, patch: Record<string, unknown>) {
    if (device === 'desktop') {
      onchange({ ...value, [group]: { ...(value?.[group] as Record<string, unknown> ?? {}), ...patch } });
    } else {
      const override = (value?.responsive?.[device] ?? {}) as BlockStyle;
      const updatedOverride = { ...override, [group]: { ...((override[group] as Record<string, unknown>) ?? {}), ...patch } };
      onchange({ ...value, responsive: { ...value?.responsive, [device]: updatedOverride } });
    }
  }

  function resetGroups(groups: (keyof BlockStyle)[]) {
    if (device === 'desktop') {
      onchange({ ...value, ...Object.fromEntries(groups.map((g) => [g, undefined])) });
    } else {
      const override = { ...((value?.responsive?.[device] ?? {}) as BlockStyle) };
      for (const g of groups) delete override[g];
      onchange({ ...value, responsive: { ...value?.responsive, [device]: override } });
    }
  }

  function g(group: keyof BlockStyle): Record<string, unknown> {
    return ((editingStyle?.[group] as Record<string, unknown>) ?? {});
  }

  /** Update the background (device-aware). */
  function updateBackground(bg: BlockStyle['background']) {
    if (device === 'desktop') {
      onchange({ ...value, background: bg });
    } else {
      const override = (value?.responsive?.[device] ?? {}) as BlockStyle;
      onchange({ ...value, responsive: { ...value?.responsive, [device]: { ...override, background: bg } } });
    }
  }

  /** Update custom CSS (device-aware). */
  function updateCustomCss(css: string | undefined) {
    if (device === 'desktop') {
      if (css) onchange({ ...value, custom_css: css });
      else { const { custom_css, ...rest } = value ?? {}; onchange(rest); }
    } else {
      const override = { ...((value?.responsive?.[device] ?? {}) as BlockStyle) };
      if (css) override.custom_css = css;
      else delete override.custom_css;
      onchange({ ...value, responsive: { ...value?.responsive, [device]: override } });
    }
  }

  /** Clear all overrides for the current device. */
  function resetAllOverrides() {
    if (device === 'desktop' || !value?.responsive) return;
    const { [device]: _, ...rest } = value.responsive;
    onchange({ ...value, responsive: Object.keys(rest).length > 0 ? rest : undefined });
  }

  // ── Option arrays ──
  const positionOpts: Opt[] = [{ value: 'static', label: 'Static' }, { value: 'relative', label: 'Relative' }, { value: 'absolute', label: 'Absolute' }, { value: 'fixed', label: 'Fixed' }, { value: 'sticky', label: 'Sticky' }];
  const zIndexOpts: Opt[] = [{ value: 'auto', label: 'Auto' }, ...[0, 10, 20, 30, 40, 50, 100, 999].map((n) => ({ value: String(n), label: String(n) }))];
  const widthOpts: Opt[] = [{ value: 'auto', label: 'Auto' }, { value: 'full', label: 'Full' }, { value: 'contained', label: 'Contained' }, { value: 'fit', label: 'Fit' }];
  const displayOpts: Opt[] = [{ value: 'block', label: 'Block' }, { value: 'flex', label: 'Flex' }, { value: 'grid', label: 'Grid' }, { value: 'inline-flex', label: 'Inline Flex' }, { value: 'inline-block', label: 'Inline Block' }, { value: 'hidden', label: 'Hidden' }];
  const flexDirectionOpts: Opt[] = [{ value: 'row', label: 'Row' }, { value: 'column', label: 'Column' }, { value: 'row-reverse', label: 'Row Reverse' }, { value: 'column-reverse', label: 'Column Reverse' }];
  const alignOpts: Opt[] = [{ value: 'start', label: 'Start' }, { value: 'center', label: 'Center' }, { value: 'end', label: 'End' }, { value: 'stretch', label: 'Stretch' }, { value: 'baseline', label: 'Baseline' }];
  const justifyOpts: Opt[] = [{ value: 'start', label: 'Start' }, { value: 'center', label: 'Center' }, { value: 'end', label: 'End' }, { value: 'between', label: 'Between' }, { value: 'around', label: 'Around' }, { value: 'evenly', label: 'Evenly' }];
  const gapOpts: Opt[] = ['none', 'xs', 'sm', 'md', 'lg', 'xl', '2xl'].map((v) => ({ value: v, label: v === 'none' ? 'None' : v.toUpperCase() }));
  const sizeOpts: Opt[] = ['xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', '4xl', '5xl'].map((v) => ({ value: v, label: v.toUpperCase() }));
  const bodySizeOpts: Opt[] = sizeOpts.slice(0, 5);
  const weightOpts: Opt[] = [100, 200, 300, 400, 500, 600, 700, 800, 900].map((n) => ({ value: String(n), label: String(n) }));
  const familyOpts: Opt[] = [{ value: 'sans', label: 'Sans' }, { value: 'serif', label: 'Serif' }, { value: 'mono', label: 'Mono' }];
  const textAlignOpts: Opt[] = [{ value: 'left', label: 'Left' }, { value: 'center', label: 'Center' }, { value: 'right', label: 'Right' }, { value: 'justify', label: 'Justify' }];
  const borderWidthOpts: Opt[] = [{ value: '0', label: 'None' }, { value: '1', label: '1px' }, { value: '2', label: '2px' }, { value: '4', label: '4px' }, { value: '8', label: '8px' }];
  const borderStyleOpts: Opt[] = [{ value: 'none', label: 'None' }, { value: 'solid', label: 'Solid' }, { value: 'dashed', label: 'Dashed' }, { value: 'dotted', label: 'Dotted' }, { value: 'double', label: 'Double' }];
  const radiusOpts: Opt[] = ['none', 'sm', 'md', 'lg', 'xl', '2xl', '3xl', 'full'].map((v) => ({ value: v, label: v === 'full' ? 'Full' : v === 'none' ? 'None' : v.toUpperCase() }));
  const sidesOpts: Opt[] = [{ value: 'all', label: 'All' }, { value: 'top', label: 'Top' }, { value: 'bottom', label: 'Bottom' }, { value: 'left', label: 'Left' }, { value: 'right', label: 'Right' }, { value: 'x', label: 'X' }, { value: 'y', label: 'Y' }];
  const spacingOpts: Opt[] = ['none', 'xs', 'sm', 'md', 'lg', 'xl', '2xl', '3xl'].map((v) => ({ value: v, label: v === 'none' ? 'None' : v.toUpperCase() }));
  const shadowOpts: Opt[] = [{ value: 'none', label: 'None' }, { value: 'sm', label: 'SM' }, { value: 'md', label: 'MD' }, { value: 'lg', label: 'LG' }, { value: 'xl', label: 'XL' }, { value: '2xl', label: '2XL' }, { value: 'inner', label: 'Inner' }];
  const blurOpts: Opt[] = [{ value: 'none', label: 'None' }, { value: 'sm', label: 'SM' }, { value: 'md', label: 'MD' }, { value: 'lg', label: 'LG' }, { value: 'xl', label: 'XL' }];
  const overflowOpts: Opt[] = [{ value: 'visible', label: 'Visible' }, { value: 'hidden', label: 'Hidden' }, { value: 'clip', label: 'Clip' }, { value: 'scroll', label: 'Scroll' }, { value: 'auto', label: 'Auto' }];
  const transitionPropOpts: Opt[] = [{ value: 'none', label: 'None' }, { value: 'all', label: 'All' }, { value: 'colors', label: 'Colors' }, { value: 'opacity', label: 'Opacity' }, { value: 'transform', label: 'Transform' }, { value: 'shadow', label: 'Shadow' }];
  const easingOpts: Opt[] = [{ value: 'linear', label: 'Linear' }, { value: 'ease', label: 'Ease' }, { value: 'ease-in', label: 'Ease In' }, { value: 'ease-out', label: 'Ease Out' }, { value: 'ease-in-out', label: 'Ease In Out' }];
  const iconSizeOpts: Opt[] = ['xs', 'sm', 'md', 'lg', 'xl', '2xl'].map((v) => ({ value: v, label: v.toUpperCase() }));
  const shapeOpts: Opt[] = [{ value: 'none', label: 'None' }, { value: 'circle', label: 'Circle' }, { value: 'square', label: 'Square' }, { value: 'rounded', label: 'Rounded' }];
  const iconPositionOpts: Opt[] = [{ value: 'left', label: 'Left' }, { value: 'top', label: 'Top' }, { value: 'right', label: 'Right' }, { value: 'none', label: 'None' }];

  // ── Select field configs per group ──
  const layoutFields: Array<{ label: string; key: string; options: Opt[] }> = [
    { label: 'Position', key: 'position', options: positionOpts },
    { label: 'Z-Index', key: 'z_index', options: zIndexOpts },
    { label: 'Width', key: 'width', options: widthOpts },
    { label: 'Display', key: 'display', options: displayOpts },
    { label: 'Flex Direction', key: 'flex_direction', options: flexDirectionOpts },
    { label: 'Align Items', key: 'align_items', options: alignOpts },
    { label: 'Justify Content', key: 'justify_content', options: justifyOpts },
    { label: 'Gap', key: 'gap', options: gapOpts },
  ];

  const typographySelects: Array<{ label: string; key: string; options: Opt[] }> = [
    { label: 'Heading Size', key: 'heading_size', options: sizeOpts },
    { label: 'Heading Weight', key: 'heading_weight', options: weightOpts },
    { label: 'Heading Family', key: 'heading_family', options: familyOpts },
    { label: 'Body Size', key: 'body_size', options: bodySizeOpts },
    { label: 'Body Weight', key: 'body_weight', options: weightOpts },
    { label: 'Text Align', key: 'text_align', options: textAlignOpts },
  ];

  const borderSelects: Array<{ label: string; key: string; options: Opt[] }> = [
    { label: 'Width', key: 'width', options: borderWidthOpts },
    { label: 'Style', key: 'style', options: borderStyleOpts },
    { label: 'Radius', key: 'radius', options: radiusOpts },
    { label: 'Sides', key: 'sides', options: sidesOpts },
  ];

  const effectsSelects: Array<{ label: string; key: string; options: Opt[] }> = [
    { label: 'Shadow', key: 'shadow', options: shadowOpts },
    { label: 'Backdrop Blur', key: 'backdrop_blur', options: blurOpts },
    { label: 'Overflow', key: 'overflow', options: overflowOpts },
  ];

  const transitionSelects: Array<{ label: string; key: string; options: Opt[] }> = [
    { label: 'Property', key: 'property', options: transitionPropOpts },
    { label: 'Easing', key: 'easing', options: easingOpts },
  ];

  const iconSelects: Array<{ label: string; key: string; options: Opt[] }> = [
    { label: 'Size', key: 'size', options: iconSizeOpts },
    { label: 'Shape', key: 'shape', options: shapeOpts },
    { label: 'Position', key: 'position', options: iconPositionOpts },
  ];
</script>

{#snippet sel(label: string, group: keyof BlockStyle, field: string, options: Opt[], current: unknown)}
  <label class="block">
    <span class={labelClass}>{label}</span>
    <select class={inputClass} value={(current as string) ?? ''} onchange={(e) => updateGroup(group, { [field]: e.currentTarget.value || undefined })}>
      <option value="">—</option>
      {#each options as opt}<option value={opt.value}>{opt.label}</option>{/each}
    </select>
  </label>
{/snippet}

{#snippet col(label: string, group: keyof BlockStyle, field: string, current: unknown)}
  <label class="block">
    <span class={labelClass}>{label}</span>
    <div class="flex gap-2">
      <input type="color" value={(current as string) ?? '#000000'} oninput={(e) => updateGroup(group, { [field]: e.currentTarget.value })} class="h-8 w-12 shrink-0 rounded border border-[var(--color-border)]" />
      <input type="text" value={(current as string) ?? ''} oninput={(e) => updateGroup(group, { [field]: e.currentTarget.value })} placeholder="#000000" class={inputClass} />
    </div>
  </label>
{/snippet}

{#snippet num(label: string, group: keyof BlockStyle, field: string, current: unknown, placeholder?: string)}
  <label class="block">
    <span class={labelClass}>{label}</span>
    <input type="number" value={(current as number) ?? ''} oninput={(e) => updateGroup(group, { [field]: e.currentTarget.value ? Number(e.currentTarget.value) : undefined })} {placeholder} class={inputClass} />
  </label>
{/snippet}

{#snippet rng(label: string, group: keyof BlockStyle, field: string, min: number, max: number, current: unknown, suffix?: string)}
  <label class="block">
    <span class={labelClass}>{label}: {current ?? min}{suffix ?? ''}</span>
    <input type="range" {min} {max} value={(current as number) ?? min} oninput={(e) => updateGroup(group, { [field]: Number(e.currentTarget.value) })} class="w-full" />
  </label>
{/snippet}

{#snippet resetBtn(label: string, groups: (keyof BlockStyle)[])}
  <button type="button" class={resetClass} onclick={() => resetGroups(groups)}>Reset {label}</button>
{/snippet}

{#if device !== 'desktop'}
  <div class="flex items-center justify-between rounded-[var(--radius)] px-3 py-1.5 text-xs" style="background: color-mix(in srgb, var(--color-primary) 10%, transparent); color: var(--color-primary)">
    <span class="font-medium">Editing {device} overrides</span>
    <button type="button" class="text-[10px] underline" onclick={() => resetAllOverrides()}>Clear all {device} overrides</button>
  </div>
{/if}

<Accordion.Root type="multiple" class="w-full border-y border-[var(--color-border)]">
  <!-- 1. Layout -->
  <Accordion.Item value="layout" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Layout
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      {#each layoutFields as f}{@render sel(f.label, 'layout', f.key, f.options, g('layout')[f.key])}{/each}
      {@render resetBtn('Layout', ['layout'])}
    </Accordion.Content>
  </Accordion.Item>

  <!-- 2. Typography -->
  <Accordion.Item value="typography" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Typography
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      {#each typographySelects as f}{@render sel(f.label, 'typography', f.key, f.options, g('typography')[f.key])}{/each}
      {@render col('Heading Color', 'typography', 'heading_color', g('typography')['heading_color'])}
      {@render col('Body Color', 'typography', 'body_color', g('typography')['body_color'])}
      {@render resetBtn('Typography', ['typography'])}
    </Accordion.Content>
  </Accordion.Item>

  <!-- 3. Color & Background -->
  <Accordion.Item value="color" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Color &amp; Background
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      {@render col('Text Color', 'color', 'text', g('color')['text'])}
      {@render col('Heading Color', 'color', 'heading', g('color')['heading'])}
      {@render col('Accent Color', 'color', 'accent', g('color')['accent'])}
      <label class="block">
        <span class={labelClass}>Background</span>
        <BackgroundPicker value={editingStyle?.background} onchange={(v) => updateBackground(v)} {mediaPickerComponent} />
      </label>
      {@render resetBtn('Colors', ['color', 'background'])}
    </Accordion.Content>
  </Accordion.Item>

  <!-- 4. Border -->
  <Accordion.Item value="border" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Border
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      {#each borderSelects as f}{@render sel(f.label, 'border', f.key, f.options, g('border')[f.key])}{/each}
      {@render col('Border Color', 'border', 'color', g('border')['color'])}
      {@render resetBtn('Border', ['border'])}
    </Accordion.Content>
  </Accordion.Item>

  <!-- 5. Spacing -->
  <Accordion.Item value="spacing" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Spacing
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      {@render sel('Padding', 'spacing', 'padding', spacingOpts, g('spacing')['padding'])}
      {@render sel('Margin', 'spacing', 'margin', spacingOpts, g('spacing')['margin'])}
      {@render resetBtn('Spacing', ['spacing'])}
    </Accordion.Content>
  </Accordion.Item>

  <!-- 6. Effects -->
  <Accordion.Item value="effects" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Effects
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      {#each effectsSelects as f}{@render sel(f.label, 'effects', f.key, f.options, g('effects')[f.key])}{/each}
      {@render rng('Opacity', 'effects', 'opacity', 0, 100, g('effects')['opacity'], '%')}
      {@render resetBtn('Effects', ['effects'])}
    </Accordion.Content>
  </Accordion.Item>

  <!-- 7. Transform -->
  <Accordion.Item value="transform" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Transform
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      {@render rng('Scale', 'transform', 'scale', 50, 200, g('transform')['scale'], '%')}
      {@render rng('Rotate', 'transform', 'rotate', -360, 360, g('transform')['rotate'], '°')}
      {@render num('Translate X', 'transform', 'translate_x', g('transform')['translate_x'], 'px')}
      {@render num('Translate Y', 'transform', 'translate_y', g('transform')['translate_y'], 'px')}
      {@render resetBtn('Transform', ['transform'])}
    </Accordion.Content>
  </Accordion.Item>

  <!-- 8. Transition -->
  <Accordion.Item value="transition" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Transition
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      {#each transitionSelects as f}{@render sel(f.label, 'transition', f.key, f.options, g('transition')[f.key])}{/each}
      {@render num('Duration (ms)', 'transition', 'duration', g('transition')['duration'], 'ms')}
      {@render num('Delay (ms)', 'transition', 'delay', g('transition')['delay'], 'ms')}
      {@render resetBtn('Transition', ['transition'])}
    </Accordion.Content>
  </Accordion.Item>

  <!-- 9. Icon -->
  <Accordion.Item value="icon" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Icon
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      <label class="block">
        <span class={labelClass}>Icon</span>
        <IconPicker value={g('icon')['name'] as string} onchange={(v) => updateGroup('icon', { name: v })} />
      </label>
      {#each iconSelects as f}{@render sel(f.label, 'icon', f.key, f.options, g('icon')[f.key])}{/each}
      {@render col('Icon Color', 'icon', 'color', g('icon')['color'])}
      {@render col('Icon Background', 'icon', 'background_color', g('icon')['background_color'])}
      {@render resetBtn('Icon', ['icon'])}
    </Accordion.Content>
  </Accordion.Item>

  <!-- 10. Advanced (Custom CSS) -->
  <Accordion.Item value="advanced" class={itemClass}>
    <Accordion.Header>
      <Accordion.Trigger class={triggerClass}>
        Advanced
        <svg class="size-4 transition-transform" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18l6-6-6-6" /></svg>
      </Accordion.Trigger>
    </Accordion.Header>
    <Accordion.Content class={contentClass}>
      <label class="block">
        <span class={labelClass}>Custom CSS</span>
        <p class="mb-1.5 text-[10px] text-[var(--color-muted)]">
          Use <code class="rounded bg-[var(--color-surface-subtle)] px-1">&amp;</code> as a placeholder for this block's scoped selector.
          Example: <code class="text-[10px]">&amp; {'>'} h2 {'{'} letter-spacing: 0.05em; {'}'}</code>
        </p>
        <textarea
          value={editingStyle?.custom_css ?? ''}
          oninput={(e) => updateCustomCss(e.currentTarget.value || undefined)}
          placeholder={'& { border-radius: 12px; }\n& > p { line-height: 1.6; }'}
          rows="6"
          class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 font-mono text-xs text-[var(--color-text)]"
          spellcheck="false"
        ></textarea>
      </label>
      {#if editingStyle?.custom_css}
        <button type="button" class={resetClass} onclick={() => updateCustomCss(undefined)}>Clear Custom CSS</button>
      {/if}
    </Accordion.Content>
  </Accordion.Item>
</Accordion.Root>
