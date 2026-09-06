<script lang="ts">
  // Abstract "global move" illustration for the split-layout guest hero.
  // Uses theme-aware CSS variables so it adapts to light/dark mode.
</script>

<svg
  viewBox="0 0 640 420"
  class="h-full w-full"
  role="img"
  aria-label="A person holding a passport, with a city skyline, an airplane, and a sweeping global wave"
  xmlns="http://www.w3.org/2000/svg"
>
  <title>Your global move, simplified</title>

  <!-- Water / horizon base -->
  <rect x="0" y="300" width="640" height="120" fill="var(--color-primary)" opacity="0.08" />

  <!-- Green/blue sweeping waves -->
  <path
    d="M -20 380 C 120 340 240 300 640 240 L 640 420 L -20 420 Z"
    fill="var(--color-primary)"
    opacity="0.18"
  />
  <path
    d="M -20 400 C 160 360 320 320 640 280 L 640 420 L -20 420 Z"
    fill="var(--color-success)"
    opacity="0.22"
  />

  <!-- City skyline (right side) -->
  <g opacity="0.28" fill="var(--color-text)">
    <!-- generic buildings -->
    <rect x="360" y="220" width="24" height="80" rx="2" />
    <rect x="390" y="200" width="32" height="100" rx="2" />
    <rect x="430" y="240" width="20" height="60" rx="2" />
    <rect x="460" y="180" width="28" height="120" rx="2" />
    <rect x="495" y="230" width="22" height="70" rx="2" />
    <rect x="525" y="210" width="30" height="90" rx="2" />
    <rect x="565" y="250" width="18" height="50" rx="2" />

    <!-- CN Tower-ish needle -->
    <rect x="600" y="80" width="6" height="220" rx="2" />
    <circle cx="603" cy="76" r="9" />

    <!-- Sydney Opera House-ish sails -->
    <path d="M 240 300 Q 260 230 280 300 Z" />
    <path d="M 270 300 Q 295 210 320 300 Z" />
    <path d="M 300 300 Q 330 190 360 300 Z" />
  </g>

  <!-- Person (left of center) -->
  <g opacity="0.32" fill="var(--color-text)">
    <!-- head -->
    <circle cx="235" cy="145" r="34" />
    <!-- body/shoulders -->
    <path d="M 185 210 Q 235 185 285 210 L 305 330 L 165 330 Z" />
  </g>

  <!-- Arm / hand holding a passport -->
  <path
    d="M 275 240 Q 330 250 360 290"
    stroke="var(--color-text)"
    stroke-width="12"
    stroke-linecap="round"
    fill="none"
    opacity="0.35"
  />

  <!-- Passport -->
  <rect x="345" y="235" width="90" height="120" rx="6" fill="var(--color-primary)" opacity="0.9" />
  <text x="360" y="260" font-size="10" fill="white" opacity="0.9">PASSPORT</text>
  <circle cx="390" cy="295" r="22" stroke="white" stroke-width="2" fill="none" />
  <path
    d="M 368 295 h 44 M 390 273 a 22 22 0 0 1 0 44"
    stroke="white"
    stroke-width="1.5"
    fill="none"
  />

  <!-- Airplane with dashed flight arc -->
  <g transform="translate(480, 70) rotate(-12)">
    <path
      d="M 0 0 L 42 -10 L 38 6 L 74 0 L 76 10 L 38 16 L 34 30 Z"
      fill="var(--color-primary)"
    />
  </g>
  <path
    d="M 520 80 Q 420 110 260 130"
    stroke="var(--color-success)"
    stroke-width="2.5"
    fill="none"
    stroke-dasharray="7 6"
    opacity="0.8"
  />

  <!-- Subtle sun/moon accent -->
  <circle cx="540" cy="120" r="22" fill="var(--color-success)" opacity="0.12" />
</svg>
