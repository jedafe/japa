<script lang="ts">
  import type { GuestFooterProps, Block, GuestFooterLinkProps } from '../../../types';
  import GuestCardListEditor from './GuestCardListEditor.svelte';
  import NestedBlocksEditor from '../../NestedBlocksEditor.svelte';

  interface Props {
    props: GuestFooterProps;
    onchange: (v: GuestFooterProps) => void;
  }

  let { props, onchange }: Props = $props();

  const inputClass =
    'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm text-[var(--color-text)]';

  function update<K extends keyof GuestFooterProps>(key: K, value: GuestFooterProps[K]) {
    onchange({ ...props, [key]: value });
  }

  function defaultSocial() {
    return { id: crypto.randomUUID(), icon: 'Globe', url: '', label: '' };
  }

  function defaultColumn() {
    return { id: crypto.randomUUID(), title: '', links: [{ label: '', url: '' }] };
  }

  function linkToBlock(link: { label: string; url?: string; target?: string; icon?: string }): Block {
    return {
      id: crypto.randomUUID(),
      type: 'guest_footer_link',
      visible: true,
      props: { label: link.label, url: link.url, target: link.target, icon: link.icon } as GuestFooterLinkProps,
    } as Block;
  }

  // Convert the three legacy arrays (socials, columns, bottom links) into child
  // blocks partitioned by type so the footer matches its rendered structure.
  function convertToNested() {
    const children: Block[] = [
      ...(props.socials ?? []).map(
        (s) =>
          ({
            id: crypto.randomUUID(),
            type: 'guest_footer_social',
            visible: true,
            props: { icon: s.icon, url: s.url, label: s.label },
          }) as Block
      ),
      ...(props.columns ?? []).map(
        (col) =>
          ({
            id: crypto.randomUUID(),
            type: 'guest_footer_column',
            visible: true,
            props: { title: col.title, children: (col.links ?? []).map(linkToBlock) },
          }) as Block
      ),
      ...(props.bottom_links ?? []).map(linkToBlock),
    ];
    onchange({ ...props, socials: undefined, columns: undefined, bottom_links: undefined, children });
  }

  const hasChildren = $derived((props.children?.length ?? 0) > 0);
  const hasLegacyItems = $derived(
    (props.socials?.length ?? 0) + (props.columns?.length ?? 0) + (props.bottom_links?.length ?? 0) > 0
  );
</script>

<!-- Ref: japaconnect-engineering/sections/16-page-builder.md -->
<div class="space-y-4">
  <div class="grid grid-cols-2 gap-2">
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Light logo URL</span>
      <input
        class={inputClass}
        value={props.logo?.light ?? ''}
        oninput={(e) => update('logo', { ...props.logo, light: e.currentTarget.value || undefined })}
      />
    </label>
    <label class="block">
      <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Dark logo URL</span>
      <input
        class={inputClass}
        value={props.logo?.dark ?? ''}
        oninput={(e) => update('logo', { ...props.logo, dark: e.currentTarget.value || undefined })}
      />
    </label>
  </div>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Tagline</span>
    <input
      class={inputClass}
      value={props.tagline ?? ''}
      oninput={(e) => update('tagline', e.currentTarget.value || undefined)}
    />
  </label>
  <label class="block">
    <span class="mb-1 block text-xs font-medium text-[var(--color-muted)]">Copyright</span>
    <input
      class={inputClass}
      value={props.copyright ?? ''}
      oninput={(e) => update('copyright', e.currentTarget.value || undefined)}
    />
  </label>

  {#if hasChildren}
    <div class="space-y-2">
      <p class="text-xs text-[var(--color-muted)]">
        The footer is composed of nested blocks: <b>Footer Social</b> icons in the brand area, <b>Footer Column</b> blocks in the columns grid, and <b>Footer Link</b> blocks in the bottom bar. Select one in the sidebar to edit it.
      </p>
      <NestedBlocksEditor
        blocks={props.children ?? []}
        allowedTypes={['guest_footer_column', 'guest_footer_social', 'guest_footer_link']}
        onchange={(children) => update('children', children)}
      />
    </div>
  {:else}
    <div class="space-y-2">
      <div class="text-xs font-medium text-[var(--color-muted)]">Social links</div>
      <GuestCardListEditor
        items={props.socials ?? []}
        onchange={(items) => update('socials', items)}
        add={defaultSocial}
        id="footer-socials"
      >
        {#snippet card(social: { icon: string; url: string; label: string }, updateSocial)}
          <div class="space-y-2">
            <input
              class={inputClass}
              value={social.icon}
              oninput={(e) => updateSocial({ ...social, icon: e.currentTarget.value })}
              placeholder="Icon"
            />
            <input
              class={inputClass}
              value={social.url}
              oninput={(e) => updateSocial({ ...social, url: e.currentTarget.value })}
              placeholder="URL"
            />
            <input
              class={inputClass}
              value={social.label}
              oninput={(e) => updateSocial({ ...social, label: e.currentTarget.value })}
              placeholder="Label"
            />
          </div>
        {/snippet}
      </GuestCardListEditor>
    </div>

    <div class="space-y-2">
      <div class="text-xs font-medium text-[var(--color-muted)]">Columns</div>
      <GuestCardListEditor
        items={props.columns ?? []}
        getId={(c) => c.id ?? c.title}
        onchange={(items) => update('columns', items)}
        add={defaultColumn}
        id="footer-columns"
      >
        {#snippet card(col, updateCol)}
          {@const links = col.links ?? []}
          <div class="space-y-2">
            <input
              class={inputClass}
              value={col.title}
              oninput={(e) => updateCol({ ...col, title: e.currentTarget.value })}
              placeholder="Column title"
            />
            <div class="space-y-1 border-s border-[var(--color-border)] ps-2">
              {#each links as link, li (li)}
                <div class="flex items-center gap-2">
                  <input
                    class={inputClass}
                    value={link.label}
                    oninput={(e) => {
                      const next = [...links];
                      next[li] = { ...link, label: e.currentTarget.value };
                      updateCol({ ...col, links: next });
                    }}
                    placeholder="Label"
                  />
                  <input
                    class={inputClass}
                    value={link.url}
                    oninput={(e) => {
                      const next = [...links];
                      next[li] = { ...link, url: e.currentTarget.value };
                      updateCol({ ...col, links: next });
                    }}
                    placeholder="URL"
                  />
                  <button
                    type="button"
                    onclick={() => {
                      const next = links.filter((_: unknown, i: number) => i !== li);
                      updateCol({ ...col, links: next });
                    }}
                    class="text-[var(--color-danger)]"
                  >
                    ×
                  </button>
                </div>
              {/each}
              <button
                type="button"
                onclick={() => updateCol({ ...col, links: [...links, { label: '', url: '' }] })}
                class="text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]"
              >
                + Add link
              </button>
            </div>
          </div>
        {/snippet}
      </GuestCardListEditor>
    </div>

    <div class="space-y-2">
      <div class="text-xs font-medium text-[var(--color-muted)]">Bottom links</div>
      <GuestCardListEditor
        items={props.bottom_links ?? []}
        getId={(l) => l.label}
        onchange={(items) => update('bottom_links', items)}
        add={() => ({ label: '', url: '' })}
        id="footer-bottom-links"
      >
        {#snippet card(link: { label: string; url: string }, updateLink)}
          <div class="flex items-center gap-2">
            <input
              class={inputClass}
              value={link.label}
              oninput={(e) => updateLink({ ...link, label: e.currentTarget.value })}
              placeholder="Label"
            />
            <input
              class={inputClass}
              value={link.url}
              oninput={(e) => updateLink({ ...link, url: e.currentTarget.value })}
              placeholder="URL"
            />
          </div>
        {/snippet}
      </GuestCardListEditor>
    </div>

    {#if hasLegacyItems}
      <button
        type="button"
        onclick={convertToNested}
        class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-2 py-1.5 text-xs text-[var(--color-text)] hover:bg-[var(--color-surface)]"
      >
        Convert to block items
      </button>
    {:else}
      <div class="space-y-2">
        <p class="text-xs text-[var(--color-muted)]">
          Or manage the footer as nested blocks (columns, socials, and links editable in the sidebar):
        </p>
        <NestedBlocksEditor
          blocks={props.children ?? []}
          allowedTypes={['guest_footer_column', 'guest_footer_social', 'guest_footer_link']}
          onchange={(children) => update('children', children)}
        />
      </div>
    {/if}
  {/if}
</div>
