# Section 23 — Main Site Frontend

**Status:** ✅ Locked

## Decided
Navigation: code-defined fixed slots (`primary_nav`, `mobile_bottom_tab`, `footer`, `user_dropdown`) populated by the admin-configurable Navigation Manager — structure is code, content within it is admin-managed. Bottom tab bar on mobile (Home/Feed, Spaces, Search, Notifications, Profile) over a hamburger-only pattern, given expected mobile-heavy usage. Visible Primary Space switcher in the header given relocation changes it.

Notification centre: dropdown (desktop) / full-screen (mobile), live updates via the WS `notification` event, real-time unread badge. Profile settings catalog: Account, Privacy, Notification preferences, Language, Spaces, Billing, Data & Privacy, Organization management. Shared error/empty-state component (404/403/500/empty-list), visually distinct from loading skeletons.

**UI component stack:** Tailwind CSS v4 for styling, **Bits UI** (headless Svelte primitives: Dialog, Dropdown, Tooltip, Select, Combobox, Popover, etc.) for all interactive component foundations, **Lucide Svelte** for icons. Bespoke platform components (Entity Card, Feed Card, Comparison Matrix, Ad Slot, Verification Badge indicator, Reaction Bar, Bookmark Button, etc.) are built on top of Bits UI primitives rather than a second headless library. All three SvelteKit apps share the same Bits UI version.

## Dark Mode (restored post-audit, see ADR-029)

System-default on first visit; user-selectable Light / Dark / System toggle stored in profile preferences (server-side, so it follows the user across devices rather than being a per-device CSS cookie). Implemented via CSS custom properties / Tailwind v4 dark-mode class strategy — no separate stylesheet download; theme switches without a page reload. The toggle lives in the user dropdown header area.

## Timezone-Aware Display (restored post-audit, see ADR-029)

All timestamps stored as UTC; all displayed times converted to the user's configured timezone (stored in profile preferences, defaulting to browser-detected timezone on first visit). Relative time ("2 hours ago") uses the user's timezone as the reference point. Content with explicit times — Events (V2 Content Extension), scheduled posts — show both the local time in the user's timezone and the original timezone of the content in a tooltip, since a "London event at 6pm" means different things to a viewer in Lagos vs Toronto. Timezone preference lives in the same profile settings catalog as Language.
