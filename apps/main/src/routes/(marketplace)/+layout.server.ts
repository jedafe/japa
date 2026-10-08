import type { LayoutServerLoad } from './$types';

// Pass-through: the root layout already loads everything the marketplace
// chrome needs (user, siteSettings, menu, footerMenu, features,
// unreadNotifications, currencies). This file documents the ADR-058
// exception and reserves the layout for marketplace-scoped data later;
// returning {} keeps child pages receiving parent() data.
export const load: LayoutServerLoad = async () => ({});
