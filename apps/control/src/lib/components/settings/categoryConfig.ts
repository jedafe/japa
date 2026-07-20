import { Bell, Cpu, CreditCard, FolderOpen, Globe, Mail, Monitor, Palette, Server, Settings, Shield, Sliders } from 'lucide-svelte';

export const categoryIcons: Record<string, typeof Settings> = {
    general: Monitor,
    advanced: Cpu,
    feed: FolderOpen,
    appearance: Palette,
    media: Server,
    i18n: Globe,
    payment: CreditCard,
    notification: Bell,
    security: Shield,
    email: Mail,
    cors: Globe,
    content: Sliders,
    auth: Shield,
    spaces: Globe,
    moderation: Shield,
    wallet: CreditCard,
    api: Server,
    ads: Sliders,
    ai: Cpu
};

export const categoryNames: Record<string, string> = {
    auth: 'Authentication',
    feed: 'Feed & Content',
    content: 'Feed & Content',
    spaces: 'Spaces',
    moderation: 'Moderation',
    security: 'Security',
    wallet: 'Wallet',
    api: 'API Rate Limits',
    cors: 'CORS',
    ads: 'Ads',
    ai: 'AI',
    general: 'General',
    advanced: 'Advanced',
    appearance: 'Appearance',
    media: 'Media',
    i18n: 'Languages',
    payment: 'Payment',
    notification: 'Notifications',
    email: 'Email'
};

export const sensitiveKeys: string[] = [
    'email.smtp_password',
    'email.smtp_username',
    'payment.secret_key',
    'payment.publishable_key',
    'media.storage.access_key',
    'media.storage.secret_key',
    'security.jwt_secret',
    'security.encryption_key',
    'oauth.client_secret',
    'oauth.google_client_secret',
    'oauth.github_client_secret'
];

export const textareaKeys: string[] = [
    'general.site_description',
    'cors.allowed_origins',
    'content.link_preview.blacklist'
];
