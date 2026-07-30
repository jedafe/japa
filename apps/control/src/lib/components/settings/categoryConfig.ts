import { CreditCard, FolderOpen, Globe, Monitor, Server, Settings, Sliders } from 'lucide-svelte';

// Only categories that have Panel: "control" settings appear in the control
// panel. Admin-only categories (general, auth, feed, appearance, content,
// spaces, moderation, wallet, api, ads, ai, i18n, email, notifications, etc.)
// are intentionally absent — they are managed in the admin panel.

export const categoryIcons: Record<string, typeof Settings> = {
    cors: Globe,
    media: Server,
    pricing: CreditCard,
    university: Monitor,
    geo_personalization: Globe,
    linkpreview: Sliders,
    content: Sliders,
    onboarding: FolderOpen
};

export const categoryNames: Record<string, string> = {
    cors: 'CORS',
    media: 'Media Storage',
    pricing: 'Pricing Plans',
    university: 'Japa University',
    geo_personalization: 'Geo Personalization',
    linkpreview: 'Link Preview',
    content: 'Content',
    onboarding: 'Onboarding'
};

// Sensitive keys that should be masked in the UI (control-panel settings only).
export const sensitiveKeys: string[] = [
    'media.storage.access_key',
    'media.storage.secret_key'
];

// Settings that render as textareas (control-panel settings only).
export const textareaKeys: string[] = [
    'cors.allowed_origins'
];

// Settings that accept file uploads (control-panel settings only).
// Currently no control-panel settings use file uploads.
export const uploadKeys: string[] = [];

// Settings that use color pickers (control-panel settings only).
// Currently no control-panel settings use color pickers.
export const colorKeys: string[] = [];
