#!/usr/bin/env bun
/**
 * TipTap SSR renderer for JapaConnect.
 *
 * Reads ProseMirror JSON from stdin, renders to HTML using TipTap,
 * and writes the HTML to stdout.
 *
 * Usage: echo '{"type":"doc","content":[...]}' | bun run scripts/tiptap-ssr.ts
 *
 * Ref: Section 15 — Block Editor (TipTap), ADR-006.
 *
 * The HTML emitted here is subsequently passed through the Go-side
 * allowlist sanitizer (internal/content.SanitizeHTML) before caching,
 * so any unsafe markup is stripped. This script additionally avoids
 * emitting raw embed HTML directly (§5.1 / §55 P4 — render-time
 * resolution stores IDs, not pre-resolved markup).
 */

import { generateHTML } from '@tiptap/html';
import StarterKit from '@tiptap/starter-kit';
import Link from '@tiptap/extension-link';
import Image from '@tiptap/extension-image';
import Placeholder from '@tiptap/extension-placeholder';
import Table from '@tiptap/extension-table';
import TableRow from '@tiptap/extension-table-row';
import TableCell from '@tiptap/extension-table-cell';
import TableHeader from '@tiptap/extension-table-header';

// Custom Link extension that also renders the affiliate_provider_id
// attribute when present (Section 55 P4 — render-time affiliate link
// rewriting). The Go backend's affiliate resolver reads this attribute
// to inject tracking params at render time.
const AffiliateLink = Link.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      affiliateProviderId: {
        default: null,
        renderHTML: (attrs) => {
          if (!attrs.affiliateProviderId) {
            return {};
          }
          return { 'data-affiliate-provider-id': attrs.affiliateProviderId };
        },
        parseHTML: (el) => el.getAttribute('data-affiliate-provider-id') || null,
      },
    };
  },
});

// Custom extension for platform embeds (YouTube, Twitter, etc.)
// Per §55 P4 (render-time over save-time), the node stores the platform
// and url — NOT pre-resolved HTML. The SSR script emits only safe data
// attributes; the actual iframe/embed markup is resolved at render time
// by the Go backend's embed resolver. This prevents stored XSS from a
// crafted `html` attribute (§5.1).
const PlatformEmbed = {
  name: 'platformEmbed',
  addAttributes() {
    return {
      platform: { default: null },
      url: { default: null },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-type="platform-embed"]' }];
  },
  renderHTML(node) {
    // Emit only safe data attributes — never the raw `html` attr.
    // The Go sanitizer would strip it anyway, but defense in depth.
    return ['div', { 'data-type': 'platform-embed', 'data-platform': node.attrs.platform, 'data-url': node.attrs.url }, 0];
  },
};

// Custom extension for mentions (users, tags, content, spaces)
// Section 15: all reference nodes store { id, entity_type, label }.
// Node name 'mention' matches the editor's Mention.extend({name:'mention'}).
const JapaMention = {
  name: 'mention',
  addAttributes() {
    return {
      id: { default: null },
      entity_type: { default: null },
      label: { default: null },
    };
  },
  parseHTML() {
    return [{ tag: 'span[data-type="japa-mention"]' }];
  },
  renderHTML(node) {
    const { id, entity_type, label } = node.attrs;
    return ['span', { 'data-type': 'japa-mention', 'data-id': id, 'data-entity-type': entity_type }, `@${label || '[deleted]'}`];
  },
};

// Coupon embed node (Section 55 — render-time coupon/affiliate URL
// resolution). Stores the coupon ID; the Go backend resolves the
// current code and tracking URL at render time (P4).
// Node name 'couponEmbed' matches the editor's custom node.
const CouponEmbed = {
  name: 'couponEmbed',
  addAttributes() {
    return {
      coupon_id: { default: null },
      label: { default: null },
      display_mode: { default: 'full' },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-type="coupon-embed"]' }];
  },
  renderHTML(node) {
    const attrs: Record<string, string> = {
      'data-type': 'coupon-embed',
      'data-coupon-id': node.attrs.coupon_id,
    };
    if (node.attrs.display_mode && node.attrs.display_mode !== 'full') {
      attrs['data-display-mode'] = node.attrs.display_mode;
    }
    return ['div', attrs, 0];
  },
};

// Tag link node (Section 15 — #tag inline node). Stores the tag id/slug/label.
// Node name 'hashtag' matches the editor's Mention.extend({name:'hashtag'}).
const TagLink = {
  name: 'hashtag',
  addAttributes() {
    return {
      id: { default: null },
      label: { default: null },
      entity_type: { default: null },
    };
  },
  parseHTML() {
    return [{ tag: 'a[data-type="tag-link"]' }];
  },
  renderHTML(node) {
    const label = node.attrs.label || 'tag';
    return ['span', { 'data-type': 'hashtag', 'data-tag-id': node.attrs.id }, `#${label}`];
  },
};

// Content/space interlink node (Section 15 — [[content/space interlinking).
// Stores the target ID and entity_type; renders as a safe relative link
// with a "deleted content" fallback if the label is missing.
// Node name 'interlink' matches the editor's Mention.extend({name:'interlink'}).
const ContentLink = {
  name: 'interlink',
  addAttributes() {
    return {
      id: { default: null },
      entity_type: { default: null },
      label: { default: null },
    };
  },
  parseHTML() {
    return [{ tag: 'a[data-type="content-link"]' }];
  },
  renderHTML(node) {
    const label = node.attrs.label || '[deleted content]';
    return ['span', { 'data-type': 'interlink', 'data-content-id': node.attrs.id, 'data-entity-type': node.attrs.entity_type }, label];
  },
};

const extensions = [
  StarterKit,
  AffiliateLink.configure({ openOnClick: false }),
  Image,
  Placeholder,
  Table,
  TableRow,
  TableCell,
  TableHeader,
  PlatformEmbed,
  JapaMention,
  CouponEmbed,
  TagLink,
  ContentLink,
];

async function main() {
  try {
    // Read all stdin
    const chunks: Uint8Array[] = [];
    const reader = Bun.stdin.stream();
    for await (const chunk of reader) {
      chunks.push(chunk);
    }
    const input = new TextDecoder().decode(Buffer.concat(chunks));

    // Parse JSON
    const doc = JSON.parse(input);

    // Render to HTML
    const html = generateHTML(doc, extensions);

    // Write to stdout
    process.stdout.write(html);
    process.exit(0);
  } catch (err) {
    console.error('SSR render error:', err);
    process.exit(1);
  }
}

main();
