# Section 39 — Space Types & University System

**Status:** ✅ Locked

## Space Type System

Space Types are admin-configurable templates that define the custom fields, permission preset, and icon for a class of Spaces. When a user (or admin) creates a new Space, they choose a Space Type.

System-seeded Space Types:
| Type | Icon | Field Group | Default Preset |
|---|---|---|---|
| `country` | 🌍 | `space_fields` | `geo_space` |
| `state` | 📍 | `space_fields` | `geo_space` |
| `city` | 🏙️ | `space_fields` | `geo_space` |
| `university` | 🎓 | `university_fields` | `curated` |
| `community` | 👥 | `space_fields` | `open_community` |
| `professional` | 💼 | `organization_fields` | `curated` |
| `interest` | ⭐ | `space_fields` | `open_community` |

Admin can create additional types and assign any existing Custom Field Group to them.

## University System

Universities are a first-class space type, seeded by admin in the Admin Panel, with a dedicated discovery page and single University page. They are a special case of `space_type = university` and inherit all Space functionality (posts, forums, members, moderation, follow/join).

### University-specific Fields (university_fields group, seeded)

```
accreditation_body    — text
country_ranking       — number (filterable, sortable)
world_ranking         — number (filterable, sortable)
founded_year          — number
programs_offered      — multiselect (admin-configurable options)
tuition_range         — select ('free'|'<$5k'|'$5k-$15k'|'$15k-$30k'|'>$30k')
student_population    — number
acceptance_rate       — number (0-100)
international_support — boolean
scholarship_available — boolean
official_website_url  — url
application_url       — url
```

### Admin CRUD for Universities

```
Admin Panel → Content → Universities
  List: filterable by country, ranking, tuition range; sortable; paginated
  Create/Edit: all standard Space fields + university_fields custom fields
  Bulk import: CSV upload with field mapping (for initial population)
  Status: published / draft / archived
```

### University Discovery Page (`/universities`)

```
Frontend list page at /universities
  Filters: country (Geo Space selector), ranking tier, tuition range, programs, scholarship
  Sort: world ranking | country ranking | name | member count | follower count
  Layout: grid (cards with logo, name, country, ranking, top programs) or list
  Each card: Follow button, View button
  SEO: JSON-LD EducationalOrganization, AggregateRating from Section 29
```

### Single University Page (`/universities/[slug]`)

```
Header: logo, name, country flag, rankings, quick stats bar
Tabs:
  Overview    — rich wiki content (TipTap editor for admins/moderators)
  Posts       — all content tagged or posted in this Space (filterable by content type)
  Forums      — forum threads in this Space
  Courses     — linked learning tracks from Section 36 (if any)
  Members     — joiners, sortable by join date / reputation
  Reviews     — Ratings & Reviews (Section 29) for this university
  Compare     — quick link to comparison page; defaults to this university as first item
Sidebar: key custom field values (ranking, tuition, programs, etc.)
```

### Comparison Integration

On any University page, a "Compare" button opens the Comparison System (Section 30) with this university pre-selected as Item 1, and a search/select to add Item 2+. Admin can also create curated comparison pages ("Top 5 Universities for Nigerian Students in Canada") using the `comparison` content type with `university` items.

## See Also

Section 05 (Spaces foundation), Section 06 (custom_field_groups), Section 29 (Ratings & Reviews on universities), Section 30 (Comparison System with university items). ADR-039.
