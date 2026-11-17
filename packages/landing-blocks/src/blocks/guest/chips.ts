// Ref: japaconnect-engineering/sections/16-page-builder.md
// Theme-aware chip classes — `.chip-tint` + a hue modifier, defined in
// guest-legacy-styles.css. They replace the fixed pastel utility pairs
// (bg-*-100 text-*-600) that washed out in dark mode, and the bare
// `.chip-tint` default follows the theme/block accent.

const CHIP_HUES = ['green', 'blue', 'purple', 'orange', 'teal', 'sky', 'muted'] as const;
export type ChipHue = (typeof CHIP_HUES)[number];

export function chipClass(hue: string | undefined): string {
  return (CHIP_HUES as readonly string[]).includes(hue ?? '')
    ? `chip-tint chip-tint-${hue}`
    : 'chip-tint';
}

// Opportunity type → chip hue for the type pill/tag.
const OPPORTUNITY_TYPE_HUE: Record<string, ChipHue> = {
  job: 'green',
  scholarship: 'blue',
  visa: 'orange',
  business: 'teal',
  internship: 'purple',
};
export function opportunityTypeChip(type: string): string {
  return chipClass(OPPORTUNITY_TYPE_HUE[type] ?? 'muted');
}

// Opportunity type → chip hue for the org-logo initials fallback.
const OPPORTUNITY_LOGO_HUE: Record<string, ChipHue> = {
  job: 'sky',
  scholarship: 'blue',
  visa: 'orange',
  business: 'green',
  internship: 'purple',
};
export function opportunityLogoChip(type: string): string {
  return chipClass(OPPORTUNITY_LOGO_HUE[type] ?? 'muted');
}
