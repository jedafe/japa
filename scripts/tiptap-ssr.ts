#!/usr/bin/env bun
/**
 * TipTap SSR renderer for JapaConnect.
 *
 * Reads ProseMirror JSON from stdin, renders to HTML using TipTap,
 * and writes the HTML to stdout.
 *
 * Usage: echo '{"type":"doc","content":[...]}' | bun run scripts/tiptap-ssr.ts
 */

import { generateHTML } from '@tiptap/html';
import StarterKit from '@tiptap/starter-kit';
import Underline from '@tiptap/extension-underline';
import Link from '@tiptap/extension-link';
import Image from '@tiptap/extension-image';
import Mention from '@tiptap/extension-mention';

// Custom extension for platform embeds (YouTube, Twitter, etc.)
const PlatformEmbed = {
  name: 'platformEmbed',
  addAttributes() {
    return {
      platform: { default: null },
      url: { default: null },
      html: { default: null },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-type="platform-embed"]' }];
  },
  renderHTML(node) {
    return ['div', { 'data-type': 'platform-embed', 'data-platform': node.attrs.platform, 'data-url': node.attrs.url }, node.attrs.html || ''];
  },
};

// Custom extension for mentions (users, tags, content, spaces)
const JapaMention = {
  name: 'japaMention',
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

const extensions = [
  StarterKit,
  Underline,
  Link.configure({ openOnClick: false }),
  Image,
  Mention,
  PlatformEmbed,
  JapaMention,
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
