# ADR-039: Space Types & University System

**Status:** Accepted

**Context:** Different Space categories needed different default fields and permission presets. Universities needed first-class treatment as a primary diaspora decision entity.

**Decision:** space_types table links to a custom_field_group (Section 06) and default permission preset. Seven seeded system types: country, state, city, university, community, professional, interest. University type has university_fields group (accreditation, ranking, tuition, programs, etc.). Admin seeds universities via CSV import or manual CRUD. University discovery at /universities; single university pages with tabs (Overview, Posts, Forums, Courses, Members, Reviews, Compare). Compare button pre-selects university as Item 1 in Section 30.

**Rationale:** Space Types enable new categories without code changes. Universities are primary in diaspora decision-making. CSV bulk import enables launch-day population.

**Consequences:** Every new Space must select a Space Type. Space type deletion blocked if spaces of that type exist. University pages need EducationalOrganization + AggregateRating JSON-LD.
