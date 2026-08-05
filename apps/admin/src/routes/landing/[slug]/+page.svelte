<script lang="ts">
  import type { PageData } from './$types';
  import { goto } from '$app/navigation';
  import { untrack } from 'svelte';
  import {
    LandingRenderer,
    blockRegistry,
    contentEditorRegistry,
    BlockStylePanel,
    landingPalettes,
    getGeoTokens,
    resolveGeoCopy,
    type Block,
    type BlockStyle,
    type GeoContext,
  } from '@japaconnect/landing-blocks';
  import { api } from '$lib/api';
  import { t } from '$lib/stores/i18n.svelte';
  import { Tabs, Dialog, Select } from 'bits-ui';
  import * as LucideIcons from 'lucide-svelte';
  import type { Component } from 'svelte';
  import MediaPickerModal from '$lib/components/ui/MediaPickerModal.svelte';
  // Cast for DI — the editors expect Component<any>, MediaPickerModal has bindable props
  const MediaPicker = MediaPickerModal as unknown as Component;

  let { data }: { data: PageData } = $props();

  // State — seeded once from `data` via untrack so the editor owns its own
  // working copy (subsequent server-side changes to `data` are intentionally
  // ignored to avoid clobbering in-progress edits).
  let blocks = $state<Block[]>(
    untrack(() =>
      Array.isArray(data.page.blocks) ? data.page.blocks as Block[] :
      typeof data.page.blocks === 'string' ? JSON.parse(data.page.blocks) as Block[] : []
    )
  );
  let selectedBlockId = $state<string | null>(null);
  let previewCountry = $state('global');
  let unsavedChanges = $state(false);
  let deviceMode = $state<'desktop' | 'tablet' | 'mobile'>('desktop');
  let activeTab = $state<'content' | 'style' | 'settings'>('content');
  let saving = $state(false);
  let publishing = $state(false);
  let showPublishDialog = $state(false);
  let pageTitle = $state(untrack(() => data.page.title));
  let saveError = $state('');
  let publishError = $state('');

  // Derived
  const selectedBlock = $derived(blocks.find((b) => b.id === selectedBlockId) ?? null);
  const visibleBlocks = $derived(blocks.filter((b) => b.visible));

  const previewCtx: GeoContext = $derived({
    contentCountry: previewCountry,
    tokens: getGeoTokens(previewCountry, 'en'),
    locale: 'en',
  });

  const resolvedBlocks = $derived(resolveGeoCopy(visibleBlocks, previewCtx));

  const deviceWidth = $derived(
    deviceMode === 'desktop' ? 1280 : deviceMode === 'tablet' ? 768 : 375
  );

  // Geo countries for preview dropdown — sourced from /geo/countries (Rule 16).
  // 'global' is prepended as the no-geo-override default.
  const geoCountries = $derived(
    ['global', ...data.countries.map((c: { geo_code: string }) => c.geo_code)]
  );

  // Block CRUD
  function addBlock(type: Block['type']): void {
    const newBlock = {
      id: crypto.randomUUID(),
      type,
      visible: true,
      props: {} as Block['props'],
    } as Block;
    blocks = [...blocks, newBlock];
    selectedBlockId = newBlock.id;
    unsavedChanges = true;
  }

  function removeBlock(id: string): void {
    blocks = blocks.filter((b) => b.id !== id);
    if (selectedBlockId === id) selectedBlockId = null;
    unsavedChanges = true;
  }

  function duplicateBlock(id: string): void {
    const block = blocks.find((b) => b.id === id);
    if (!block) return;
    const copy = { ...block, id: crypto.randomUUID() };
    const idx = blocks.findIndex((b) => b.id === id);
    blocks = [...blocks.slice(0, idx + 1), copy, ...blocks.slice(idx + 1)];
    selectedBlockId = copy.id;
    unsavedChanges = true;
  }

  function toggleBlockVisibility(id: string): void {
    blocks = blocks.map((b) => (b.id === id ? { ...b, visible: !b.visible } : b));
    unsavedChanges = true;
  }

  function reorderBlocks(fromIdx: number, toIdx: number): void {
    const reordered = [...blocks];
    const [moved] = reordered.splice(fromIdx, 1);
    reordered.splice(toIdx, 0, moved);
    blocks = reordered;
    unsavedChanges = true;
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  function updateBlockProps(id: string, props: any): void {
    blocks = blocks.map((b) => (b.id === id ? { ...b, props } : b)) as Block[];
    unsavedChanges = true;
  }

  function updateBlockStyle(id: string, style: BlockStyle): void {
    blocks = blocks.map((b) => (b.id === id ? { ...b, style } : b));
    unsavedChanges = true;
  }

  // Save/Publish
  async function saveDraft(): Promise<boolean> {
    saving = true;
    saveError = '';
    const result = await api(`/admin/pages/${data.page.id}`, {
      method: 'PATCH',
      body: { title: pageTitle, blocks: JSON.stringify(blocks) },
    });
    saving = false;
    if (result.ok) {
      unsavedChanges = false;
      return true;
    }
    saveError = result.error ?? t('landing.save_failed', 'Failed to save draft');
    return false;
  }

  async function publish(): Promise<void> {
    publishError = '';
    const saved = await saveDraft();
    if (!saved) {
      publishError = saveError || t('landing.save_failed', 'Failed to save draft');
      return;
    }
    publishing = true;
    const result = await api(`/admin/pages/${data.page.id}/publish`, { method: 'POST' });
    publishing = false;
    if (result.ok) {
      showPublishDialog = false;
      return;
    }
    publishError = result.error ?? t('landing.publish_failed', 'Failed to publish page');
  }

  // Drag and drop
  let dragIdx = $state<number | null>(null);
  function onDragStart(idx: number) { dragIdx = idx; }
  function onDrop(idx: number) {
    if (dragIdx !== null && dragIdx !== idx) reorderBlocks(dragIdx, idx);
    dragIdx = null;
  }

  // Icon helper
  function getIcon(name: string): Component | null {
    const found = (LucideIcons as Record<string, unknown>)[name];
    return (found as Component) ?? null;
  }

  // Content editor component for selected block
  const ContentEditor = $derived(selectedBlock ? contentEditorRegistry[selectedBlock.type] : null);

  // beforeunload warning
  $effect(() => {
    if (!unsavedChanges) return;
    const handler = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = ''; };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });
</script>

<div class="flex h-screen flex-col">
  <!-- Top bar -->
  <header class="flex items-center justify-between border-b border-[var(--border)] bg-[var(--background)] px-4 py-2">
    <div class="flex items-center gap-3">
      <a href="/landing" class="text-sm text-[var(--muted)] hover:text-[var(--foreground)]">&larr; {t('landing.back_to_list', 'Landing Pages')}</a>
      <input type="text" bind:value={pageTitle} class="rounded border border-[var(--border-input)] px-2 py-1 text-sm font-medium text-[var(--foreground)]" />
      {#if unsavedChanges}<span class="h-2 w-2 rounded-full bg-[var(--warning)]" title={t('landing.unsaved_changes', 'Unsaved changes')}></span>{/if}
    </div>
    <div class="flex items-center gap-2">
      <!-- Preview country -->
      <select bind:value={previewCountry} class="rounded border border-[var(--border-input)] px-2 py-1 text-xs" aria-label={t('landing.preview_country', 'Preview country')}>
        {#each geoCountries as c}<option value={c}>{c === 'global' ? t('landing.global', 'Global') : c}</option>{/each}
      </select>
      <!-- Device toggle -->
      <div class="flex gap-1">
        <button onclick={() => deviceMode = 'desktop'} class="rounded px-2 py-1 text-xs {deviceMode === 'desktop' ? 'bg-[var(--primary)] text-white' : 'border border-[var(--border)]'}">{t('landing.device_desktop', 'Desktop')}</button>
        <button onclick={() => deviceMode = 'tablet'} class="rounded px-2 py-1 text-xs {deviceMode === 'tablet' ? 'bg-[var(--primary)] text-white' : 'border border-[var(--border)]'}">{t('landing.device_tablet', 'Tablet')}</button>
        <button onclick={() => deviceMode = 'mobile'} class="rounded px-2 py-1 text-xs {deviceMode === 'mobile' ? 'bg-[var(--primary)] text-white' : 'border border-[var(--border)]'}">{t('landing.device_mobile', 'Mobile')}</button>
      </div>
      <!-- Save / Publish -->
      <button onclick={saveDraft} disabled={saving || !unsavedChanges} class="rounded border border-[var(--border)] px-3 py-1 text-sm disabled:opacity-50">
        {saving ? t('landing.saving', 'Saving...') : t('landing.save_draft', 'Save Draft')}
      </button>
      <button onclick={() => { publishError = ''; showPublishDialog = true; }} disabled={publishing} class="rounded bg-[var(--primary)] px-3 py-1 text-sm font-semibold text-white">
        {t('landing.publish', 'Publish')}
      </button>
    </div>
  </header>
  {#if saveError}
    <div class="px-4 py-2 text-sm" style="background: color-mix(in srgb, var(--color-danger) 10%, transparent); color: var(--color-danger)">
      {saveError}
    </div>
  {/if}

  <div class="flex flex-1 overflow-hidden">
    <!-- Left pane: section list -->
    <aside class="w-64 overflow-y-auto border-r border-[var(--border)] bg-[var(--background)] p-2">
      <div class="mb-2 flex items-center justify-between">
        <h2 class="text-xs font-semibold uppercase text-[var(--muted)]">{t('landing.sections', 'Sections')}</h2>
      </div>
      {#each blocks as block, i (block.id)}
        {@const meta = blockRegistry[block.type]}
        <div
          class="group flex items-center gap-2 rounded px-2 py-1.5 text-sm {selectedBlockId === block.id ? 'bg-[var(--surface)]' : 'hover:bg-[var(--surface)]'}"
          draggable="true"
          role="button"
          tabindex="0"
          aria-label="{t('landing.select_section', 'Select section')} {meta?.label ?? block.type}"
          ondragstart={() => onDragStart(i)}
          ondrop={() => onDrop(i)}
          ondragover={(e) => e.preventDefault()}
          onclick={() => selectedBlockId = block.id}
          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); selectedBlockId = block.id; } }}
        >
          {#if meta}
            {@const Icon = getIcon(meta.icon)}
            {#if Icon}<Icon size={14} />{/if}
          {/if}
          <span class="flex-1 truncate text-[var(--foreground)]">{meta?.label ?? block.type}</span>
          {#if !block.visible}<span class="text-xs text-[var(--muted)]">{t('landing.hidden', 'hidden')}</span>{/if}
        </div>
      {/each}
      <button onclick={() => { addBlock('hero'); }} class="mt-2 w-full rounded border border-dashed border-[var(--border)] py-1.5 text-xs text-[var(--muted)] hover:bg-[var(--surface)]">
        + {t('landing.add_section', 'Add Section')}
      </button>
    </aside>

    <!-- Center pane: live preview -->
    <main class="flex-1 overflow-y-auto p-4" style="background: var(--color-surface)">
      <div class="mx-auto shadow-lg" style="width: {deviceWidth}px; max-width: 100%; background: var(--color-background);">
        <LandingRenderer blocks={resolvedBlocks} />
      </div>
    </main>

    <!-- Right pane: inspector -->
    <aside class="w-96 overflow-y-auto border-l border-[var(--border)] bg-[var(--background)]">
      {#if selectedBlock}
        {@const meta = blockRegistry[selectedBlock.type]}
        <div class="border-b border-[var(--border)] p-3">
          <h3 class="text-sm font-semibold text-[var(--foreground)]">{meta?.label ?? selectedBlock.type}</h3>
        </div>
        <Tabs.Root bind:value={activeTab} class="flex flex-col h-full">
          <Tabs.List class="flex border-b border-[var(--border)]">
            <Tabs.Trigger value="content" class="flex-1 px-3 py-2 text-xs font-medium {activeTab === 'content' ? 'border-b-2 border-[var(--primary)] text-[var(--primary)]' : 'text-[var(--muted)]'}">{t('landing.tab_content', 'Content')}</Tabs.Trigger>
            <Tabs.Trigger value="style" class="flex-1 px-3 py-2 text-xs font-medium {activeTab === 'style' ? 'border-b-2 border-[var(--primary)] text-[var(--primary)]' : 'text-[var(--muted)]'}">{t('landing.tab_style', 'Style')}</Tabs.Trigger>
            <Tabs.Trigger value="settings" class="flex-1 px-3 py-2 text-xs font-medium {activeTab === 'settings' ? 'border-b-2 border-[var(--primary)] text-[var(--primary)]' : 'text-[var(--muted)]'}">{t('landing.tab_settings', 'Settings')}</Tabs.Trigger>
          </Tabs.List>
          <Tabs.Content value="content" class="flex-1 overflow-y-auto p-3">
            {#if ContentEditor}
              {@const Editor = ContentEditor}
              <Editor
                props={selectedBlock.props}
                onchange={(v: unknown) => updateBlockProps(selectedBlock.id, v)}
                mediaPickerComponent={MediaPicker}
                geoCtx={previewCtx}
              />
            {/if}
          </Tabs.Content>
          <Tabs.Content value="style" class="flex-1 overflow-y-auto p-3">
            <BlockStylePanel
              value={selectedBlock.style}
              onchange={(v: BlockStyle) => updateBlockStyle(selectedBlock.id, v)}
              mediaPickerComponent={MediaPicker}
            />
          </Tabs.Content>
          <Tabs.Content value="settings" class="flex-1 overflow-y-auto p-3">
            <div class="space-y-4">
              <label class="flex items-center justify-between text-sm">
                <span class="text-[var(--foreground)]">{t('landing.visible', 'Visible')}</span>
                <input type="checkbox" checked={selectedBlock.visible} onchange={() => toggleBlockVisibility(selectedBlock.id)} />
              </label>

              <!-- Motion / Animation -->
              <div class="space-y-3 rounded border border-[var(--border)] p-3">
                <h4 class="text-xs font-semibold uppercase text-[var(--muted)]">{t('landing.animation', 'Animation')}</h4>

                <div>
                  <label for="motion-preset" class="mb-1 block text-xs text-[var(--muted)]">{t('landing.motion_preset', 'Preset')}</label>
                  <select id="motion-preset" class="w-full rounded border border-[var(--border-input)] px-2 py-1 text-sm" value={selectedBlock.motion?.preset ?? 'none'} onchange={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? { ...b, motion: { ...(b.motion ?? { delay: 0 }), preset: e.currentTarget.value as any } } : b); unsavedChanges = true; }}>
                    <optgroup label={t('landing.motion_none', 'None')}>
                      <option value="none">{t('landing.motion_none', 'None')}</option>
                    </optgroup>
                    <optgroup label={t('landing.motion_fade', 'Fade')}>
                      <option value="fade-in">{t('landing.motion_fade_in', 'Fade In')}</option>
                      <option value="fade-up">{t('landing.motion_fade_up', 'Fade Up')}</option>
                      <option value="fade-down">{t('landing.motion_fade_down', 'Fade Down')}</option>
                    </optgroup>
                    <optgroup label={t('landing.motion_fly_in', 'Fly In')}>
                      <option value="fly-in-left">{t('landing.motion_fly_left', 'Fly In from Left')}</option>
                      <option value="fly-in-right">{t('landing.motion_fly_right', 'Fly In from Right')}</option>
                      <option value="fly-in-up">{t('landing.motion_fly_bottom', 'Fly In from Bottom')}</option>
                      <option value="fly-in-down">{t('landing.motion_fly_top', 'Fly In from Top')}</option>
                    </optgroup>
                    <optgroup label={t('landing.motion_slide', 'Slide')}>
                      <option value="slide-in">{t('landing.motion_slide_in', 'Slide In')}</option>
                      <option value="slide-in-left">{t('landing.motion_slide_left', 'Slide In from Left')}</option>
                      <option value="slide-in-right">{t('landing.motion_slide_right', 'Slide In from Right')}</option>
                    </optgroup>
                    <optgroup label={t('landing.motion_scale_zoom', 'Scale & Zoom')}>
                      <option value="scale-in">{t('landing.motion_scale_in', 'Scale In')}</option>
                      <option value="scale-out">{t('landing.motion_scale_out', 'Scale Out')}</option>
                      <option value="zoom-in">{t('landing.motion_zoom_in', 'Zoom In')}</option>
                      <option value="zoom-out">{t('landing.motion_zoom_out', 'Zoom Out')}</option>
                    </optgroup>
                    <optgroup label={t('landing.motion_blur_flip', 'Blur & Flip')}>
                      <option value="blur-in">{t('landing.motion_blur_in', 'Blur In')}</option>
                      <option value="blur-out">{t('landing.motion_blur_out', 'Blur Out')}</option>
                      <option value="flip-up">{t('landing.motion_flip_up', 'Flip Up')}</option>
                      <option value="flip-down">{t('landing.motion_flip_down', 'Flip Down')}</option>
                    </optgroup>
                    <optgroup label={t('landing.motion_special', 'Special')}>
                      <option value="bounce-in">{t('landing.motion_bounce_in', 'Bounce In')}</option>
                    </optgroup>
                  </select>
                </div>

                {#if (selectedBlock.motion?.preset ?? 'none') !== 'none'}
                  <div class="grid grid-cols-2 gap-2">
                    <div>
                      <label for="motion-delay" class="mb-1 block text-xs text-[var(--muted)]">{t('landing.motion_delay', 'Delay (ms)')}</label>
                      <input id="motion-delay" type="number" min="0" step="50" class="w-full rounded border border-[var(--border-input)] px-2 py-1 text-sm" value={selectedBlock.motion?.delay ?? 0} oninput={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? { ...b, motion: { ...(b.motion ?? { preset: 'none' }), delay: Number(e.currentTarget.value) } } : b); unsavedChanges = true; }} />
                    </div>
                    <div>
                      <label for="motion-duration" class="mb-1 block text-xs text-[var(--muted)]">{t('landing.motion_duration', 'Duration (ms)')}</label>
                      <input id="motion-duration" type="number" min="100" step="50" class="w-full rounded border border-[var(--border-input)] px-2 py-1 text-sm" value={selectedBlock.motion?.duration ?? 700} oninput={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? { ...b, motion: { ...(b.motion ?? { preset: 'none', delay: 0 }), duration: Number(e.currentTarget.value) } } : b); unsavedChanges = true; }} />
                    </div>
                  </div>

                  <div>
                    <label for="motion-easing" class="mb-1 block text-xs text-[var(--muted)]">{t('landing.motion_easing', 'Easing')}</label>
                    <select id="motion-easing" class="w-full rounded border border-[var(--border-input)] px-2 py-1 text-sm" value={selectedBlock.motion?.easing ?? 'cubic-bezier(0.16, 1, 0.3, 1)'} onchange={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? { ...b, motion: { ...(b.motion ?? { preset: 'none', delay: 0 }), easing: e.currentTarget.value } } : b); unsavedChanges = true; }}>
                      <option value="cubic-bezier(0.16, 1, 0.3, 1)">{t('landing.easing_smooth', 'Smooth (default)')}</option>
                      <option value="ease-out">{t('landing.easing_out', 'Ease Out')}</option>
                      <option value="ease-in-out">{t('landing.easing_in_out', 'Ease In Out')}</option>
                      <option value="ease-in">{t('landing.easing_in', 'Ease In')}</option>
                      <option value="linear">{t('landing.easing_linear', 'Linear')}</option>
                      <option value="cubic-bezier(0.34, 1.56, 0.64, 1)">{t('landing.easing_bounce', 'Bounce')}</option>
                      <option value="cubic-bezier(0.68, -0.55, 0.27, 1.55)">{t('landing.easing_elastic', 'Elastic')}</option>
                    </select>
                  </div>

                  <div>
                    <label for="motion-threshold" class="mb-1 block text-xs text-[var(--muted)]">{t('landing.motion_threshold', 'Viewport threshold')}: {selectedBlock.motion?.threshold ?? 0.1}</label>
                    <input id="motion-threshold" type="range" min="0" max="1" step="0.05" class="w-full" value={selectedBlock.motion?.threshold ?? 0.1} oninput={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? { ...b, motion: { ...(b.motion ?? { preset: 'none', delay: 0 }), threshold: Number(e.currentTarget.value) } } : b); unsavedChanges = true; }} />
                  </div>

                  <label class="flex items-center justify-between text-xs">
                    <span class="text-[var(--foreground)]">{t('landing.motion_once', 'Animate only once')}</span>
                    <input type="checkbox" checked={selectedBlock.motion?.once ?? true} onchange={(e) => { blocks = blocks.map((b) => b.id === selectedBlock.id ? { ...b, motion: { ...(b.motion ?? { preset: 'none', delay: 0 }), once: e.currentTarget.checked } } : b); unsavedChanges = true; }} />
                  </label>

                  <button type="button" onclick={() => { blocks = blocks.map((b) => b.id === selectedBlock.id ? { ...b, motion: undefined } : b); unsavedChanges = true; }} class="w-full rounded border border-[var(--border)] px-2 py-1 text-xs text-[var(--muted)] hover:bg-[var(--surface)]">
                    {t('landing.motion_reset', 'Reset animation')}
                  </button>
                {/if}
              </div>

              <div class="flex gap-2 pt-2">
                <button onclick={() => duplicateBlock(selectedBlock.id)} class="flex-1 rounded border border-[var(--border)] px-2 py-1 text-xs">{t('landing.duplicate', 'Duplicate')}</button>
                <button onclick={() => removeBlock(selectedBlock.id)} class="flex-1 rounded border border-[var(--danger)] px-2 py-1 text-xs text-[var(--danger)]">{t('common.delete', 'Delete')}</button>
              </div>
            </div>
          </Tabs.Content>
        </Tabs.Root>
      {:else}
        <div class="p-6 text-center text-sm text-[var(--muted)]">
          {t('landing.select_section_hint', 'Select a section to edit its content, style, and settings.')}
        </div>
      {/if}
    </aside>
  </div>
</div>

<!-- Publish confirmation dialog -->
<Dialog.Root bind:open={showPublishDialog}>
  <Dialog.Portal>
    <Dialog.Overlay class="fixed inset-0 bg-black/50 z-50" />
    <Dialog.Content class="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border border-[var(--border)] bg-[var(--background)] p-6 shadow-xl">
      <Dialog.Title class="mb-2 text-lg font-semibold text-[var(--foreground)]">{t('landing.publish_confirm_title', 'Publish Landing Page?')}</Dialog.Title>
      <p class="mb-4 text-sm text-[var(--muted)]">{t('landing.publish_confirm_body', 'This will make the current draft live. Visitors will see the published version immediately.')}</p>
      {#if publishError}
        <p class="mb-4 text-sm" style="color: var(--color-danger)">{publishError}</p>
      {/if}
      <div class="flex justify-end gap-2">
        <button onclick={() => showPublishDialog = false} class="rounded border border-[var(--border)] px-4 py-2 text-sm">{t('common.cancel', 'Cancel')}</button>
        <button onclick={publish} disabled={publishing} class="rounded bg-[var(--primary)] px-4 py-2 text-sm font-semibold text-white disabled:opacity-50">
          {publishing ? t('landing.publishing', 'Publishing...') : t('landing.publish', 'Publish')}
        </button>
      </div>
      <Dialog.Close class="absolute right-4 top-4 text-[var(--muted)]" aria-label="Close">&#10005;</Dialog.Close>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
