# Learn App - Backend Integration Complete

## Overview
The JapaConnect Learn app has been fully connected to the backend API, replacing all hardcoded data with dynamic API calls.

## Files Created/Modified

### Core Infrastructure
- **`src/lib/api.ts`** - Client-side typed API client with JWT token management
  - All learning endpoints: tracks, lessons, progress, certificates
  - Automatic auth header injection
  - Full TypeScript types for all entities

- **`src/lib/server/api.ts`** - Server-side API helper
  - Uses `PRIVATE_API_BASE_URL` env variable
  - Server-only (never bundled to client)

- **`src/hooks.server.ts`** - Authentication middleware
  - Reads `learn_token` or `jc_token` cookie
  - Validates user via `/users/me` endpoint
  - Protects `/certificates` route
  - Populates `locals.user`

- **`src/app.d.ts`** - TypeScript declarations
  - `App.Locals` interface with user type
  - `App.Error` interface with code/message

### Authentication
- **`src/routes/+layout.server.ts`** - Server load function
  - Passes user, token, locale, translations to client
  - Reads locale from cookie

- **`src/routes/+layout.svelte`** - Root layout
  - Sets JWT token in API client
  - Initializes i18n with server-provided translations
  - Auth-aware header with user avatar/menu
  - Mobile-responsive navigation
  - Logout functionality

- **`src/routes/login/+page.svelte`** - Login page
  - Email/password form
  - Calls `/auth/login` endpoint
  - Sets `learn_token` cookie on success
  - Redirects to home page

### Routes

#### Track Listing (`/`)
- **`src/routes/+page.svelte`**
  - Fetches tracks from `GET /learning/tracks`
  - Hero section with "Learn & Grow" heading
  - Grid of track cards with:
    - Thumbnail, title, description
    - Difficulty badge (color-coded)
    - Module/lesson count, estimated hours
  - Loading skeleton state
  - Empty state if no tracks
  - Error state with retry button

#### Track Detail (`/tracks/[slug]`)
- **`src/routes/tracks/[slug]/+page.ts`** - Load function
  - Fetches track detail with modules
  - Error handling for 404/load failures

- **`src/routes/tracks/[slug]/+page.svelte`** - Track detail page
  - Track header with metadata
  - Progress bar (if enrolled)
  - Module accordion with lessons
  - Completion checkmarks
  - "Start Learning" / "Continue Learning" button
  - Links to first incomplete lesson
  - Certificate link if 100% complete

#### Lesson View (`/tracks/[trackSlug]/modules/[moduleSlug]/lessons/[lessonSlug]`)
- **`src/routes/tracks/[trackSlug]/modules/[moduleSlug]/lessons/[lessonSlug]/+page.ts`**
  - Fetches lesson content, track detail
  - Builds navigation (prev/next lessons)
  - Finds current module

- **`src/routes/tracks/[trackSlug]/modules/[moduleSlug]/lessons/[lessonSlug]/+page.svelte`**
  - Progress bar at top
  - Lesson content (rendered HTML)
  - "Mark as Complete" button
  - Previous/Next lesson navigation
  - Sidebar with module lessons list
  - Completion status for each lesson

#### Certificates (`/certificates`)
- **`src/routes/certificates/+page.svelte`**
  - Protected route (requires auth)
  - Fetches certificates from `GET /learning/certificates`
  - List of earned certificates
  - Each shows: track title, issue date, download/view buttons
  - Empty state with link to tracks
  - Loading and error states

### Localization
- **`src/lib/locales/en.json`** - English translations (expanded)
- **`src/lib/locales/fr.json`** - French translations (expanded)
  - All new keys for tracks, lessons, certificates, navigation

### Environment
- **`.env`** - Environment variables
  - `VITE_API_URL` - Client-side API base URL
  - `PRIVATE_API_BASE_URL` - Server-side API base URL

## API Endpoints Used

### Client-side (via `$lib/api.ts`)
- `GET /learning/tracks` - List published tracks
- `GET /learning/tracks/{slug}` - Get track detail with modules
- `GET /learning/tracks/{trackSlug}/modules/{moduleSlug}/lessons/{lessonSlug}` - Get lesson content
- `POST /learning/lessons/{id}/progress` - Update lesson progress
- `GET /learning/progress/{trackId}` - Get user's progress for a track
- `GET /learning/certificates` - Get user's certificates

### Server-side (via `$lib/server/api.ts`)
- `GET /users/me` - Validate JWT token and get user info

## Features Implemented

✅ Full backend API integration
✅ JWT authentication with cookie-based sessions
✅ Protected routes (certificates)
✅ Progress tracking per track
✅ Lesson completion marking
✅ Previous/Next lesson navigation
✅ Module accordion with completion status
✅ Certificate listing with download links
✅ Loading skeletons for all pages
✅ Error states with retry buttons
✅ Empty states with CTAs
✅ Mobile-responsive design
✅ Dark mode support (CSS custom properties)
✅ i18n support (English/French)
✅ TypeScript types for all entities
✅ Svelte 5 runes ($state, $derived, $effect)
✅ Tailwind v4 styling

## Verification

```bash
# Type check
bun run check  # ✅ 0 errors, 0 warnings

# Build
bun run build  # ✅ Success

# Tests
bun run test   # ✅ 1 test passed
```

## Usage

### Development
```bash
cd apps/learn
bun run dev  # Runs on http://localhost:3003
```

### Environment Variables
Set in `.env`:
```
VITE_API_URL=http://localhost:8080/api/v1
PRIVATE_API_BASE_URL=http://localhost:8080
```

### Authentication Flow
1. User visits `/login`
2. Enters email/password
3. Client calls `POST /auth/login`
4. Backend returns JWT token
5. Client sets `learn_token` cookie
6. Subsequent requests include token in `Authorization` header
7. Server validates token on protected routes

## Notes

- The app uses the same CSS custom properties as the main app for consistent theming
- All API calls go through the typed client for consistency
- Server-side rendering works for all public pages
- Client-side hydration handles auth-dependent UI
- Progress is fetched client-side after mount (requires auth)
- Lesson content is rendered as HTML (supports rich text from backend)
