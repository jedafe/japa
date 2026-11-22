-- 001226 — the brand logo replaces the SiteName title in every seeded
-- email_templates header instead of rendering both.
--
-- 001215 added a guarded {{.LogoURL}} <img> above the existing h1; showing
-- the mark AND the name is redundant. The h1 moves into an {{else}} branch
-- so it still renders when no logo URL resolves (unset site base) and for
-- clients that block remote images. Admin-customized bodies that no longer
-- contain the stock header string are untouched (replace() no-ops).

UPDATE email_templates
SET html_body = replace(
    html_body,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto 8px; border: 0; outline: none; text-decoration: none;" /><br />{{end}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>$$,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto; border: 0; outline: none; text-decoration: none;" />{{else}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>{{end}}$$
);

UPDATE email_templates
SET html_body = replace(
    html_body,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto 8px; border: 0; outline: none; text-decoration: none;" /><br />{{end}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">Welcome to {{.SiteName}}</h1>$$,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto; border: 0; outline: none; text-decoration: none;" />{{else}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">Welcome to {{.SiteName}}</h1>{{end}}$$
);

-- The 001215 notification/community_digest seeds wrote the img guard and h1
-- on separate lines — same pair, newline between.
UPDATE email_templates
SET html_body = replace(
    html_body,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto 8px; border: 0; outline: none; text-decoration: none;" /><br />{{end}}
            <h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>$$,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto; border: 0; outline: none; text-decoration: none;" />{{else}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>{{end}}$$
);
