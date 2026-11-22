-- Revert 001226: restore the logo-img + SiteName h1 header pair.

UPDATE email_templates
SET html_body = replace(
    html_body,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto; border: 0; outline: none; text-decoration: none;" />{{else}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>{{end}}$$,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto 8px; border: 0; outline: none; text-decoration: none;" /><br />{{end}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">{{.SiteName}}</h1>$$
);

UPDATE email_templates
SET html_body = replace(
    html_body,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto; border: 0; outline: none; text-decoration: none;" />{{else}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">Welcome to {{.SiteName}}</h1>{{end}}$$,
    $$<img src="{{.LogoURL}}" alt="{{.SiteName}}" height="44" style="display: inline-block; max-height: 44px; margin: 0 auto 8px; border: 0; outline: none; text-decoration: none;" /><br />{{end}}<h1 style="margin: 0; color: #ffffff; font-size: 22px; font-weight: 600; letter-spacing: -0.02em;">Welcome to {{.SiteName}}</h1>$$
);

