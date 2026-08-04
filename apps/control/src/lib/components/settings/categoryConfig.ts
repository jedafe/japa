import {
    CreditCard,
    FolderOpen,
    Globe,
    Monitor,
    Server,
    Settings,
    Sliders,
    Brain,
    Wand,
    Bot,
    FileCheck,
    ClipboardCheck,
    Calculator,
    GraduationCap,
    Banknote,
    Gift,
    Megaphone,
    Mail,
    Briefcase,
    Trophy,
    Download,
    BarChart3,
    Plane,
    Bell,
    Wrench,
    Sparkles,
} from 'lucide-svelte';

// Only categories that have Panel: "control" settings appear in the control
// panel. Admin-only categories (general, auth, feed, appearance, content,
// spaces, moderation, wallet, api, ai, i18n, email, notifications, etc.)
// are intentionally absent — they are managed in the admin panel.
//
// V2 categories (JEOS §§60–78, ADRs 061–086) are infrastructure-level toggles,
// caps, schedules, and subdomain/domain configs managed by super admins.

export const categoryIcons: Record<string, typeof Settings> = {
    cors: Globe,
    media: Server,
    pricing: CreditCard,
    university: Monitor,
    geo_personalization: Globe,
    linkpreview: Sliders,
    content: Sliders,
    onboarding: FolderOpen,
    // V2 categories
    rag: Brain,
    studio: Wand,
    participation: Bot,
    checklist: FileCheck,
    assessment: ClipboardCheck,
    col_calculator: Calculator,
    scholarship_finder: GraduationCap,
    remittance: Banknote,
    referral: Gift,
    ads: Megaphone,
    newsletter: Mail,
    jobs: Briefcase,
    gamification: Trophy,
    import: Download,
    analytics: BarChart3,
    travel: Plane,
    push: Bell,
    tools: Wrench,
    japabot: Sparkles,
};

export const categoryNames: Record<string, string> = {
    cors: 'CORS',
    media: 'Media Storage',
    pricing: 'Pricing Plans',
    university: 'Japa University',
    geo_personalization: 'Geo Personalization',
    linkpreview: 'Link Preview',
    content: 'Content',
    onboarding: 'Onboarding',
    // V2 categories
    rag: 'RAG / JapaBot Knowledge',
    studio: 'AI Content Studio',
    participation: 'Participation Orchestration',
    checklist: 'Visa Document Checklist',
    assessment: 'Japa Readiness Assessment',
    col_calculator: 'Cost of Living Calculator',
    scholarship_finder: 'Scholarship Finder',
    remittance: 'Remittance Rate Tracker',
    referral: 'Referral System',
    ads: 'Self-Serve Advertising',
    newsletter: 'Newsletter & Digest',
    jobs: 'Jobs Board',
    gamification: 'Gamification',
    import: 'Content Import',
    analytics: 'Advanced Analytics',
    travel: 'Travel Search',
    push: 'Mobile Push',
    tools: 'Tools Web Surface',
    japabot: 'JapaBot',
};

// Sensitive keys that should be masked in the UI (control-panel settings only).
export const sensitiveKeys: string[] = [
    'media.storage.access_key',
    'media.storage.secret_key',
    // V2 push notification secrets (§77, ADR-074)
    'push.fcm_server_key',
    'push.apns_key_id',
    'push.apns_team_id',
    'push.apns_private_key',
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
