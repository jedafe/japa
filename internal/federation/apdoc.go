package federation

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
)

// AP document builders (§126). Documents are map[string]any so optional
// fields are simply absent rather than serialized as nulls — several
// remote implementations reject unexpected nulls.

// webfingerJRD builds the WebFinger JRD for acct:{username}@{domain}.
// The OStatus subscribe template is deliberately omitted until the apex
// has an /authorize_interaction page (M2+) — emitting a template that
// resolves to 404 is worse than none; remote follow still works via the
// follower's own instance UI.
func webfingerJRD(username, domain, actorURL, profileURL string) map[string]any {
	return map[string]any{
		"subject": fmt.Sprintf("acct:%s@%s", username, domain),
		"aliases": []string{actorURL, profileURL},
		"links": []any{
			map[string]any{
				"rel":  "self",
				"type": "application/activity+json",
				"href": actorURL,
			},
			map[string]any{
				"rel":  "http://webfinger.net/rel/profile-page",
				"type": "text/html",
				"href": profileURL,
			},
		},
	}
}

// actorDocument builds the Person document from the actor+profile row.
func actorDocument(p gen.GetActorProfileByUsernameRow, siteBase string) map[string]any {
	doc := map[string]any{
		"@context":                  asContext,
		"id":                        p.ActorUrl,
		"type":                      "Person",
		"preferredUsername":         p.Username,
		"url":                       siteBase + "/u/" + p.Username,
		"manuallyApprovesFollowers": p.ManuallyApprovesFollowers,
		"discoverable":              p.Discoverable,
		"inbox":                     p.InboxUrl,
		"outbox":                    p.OutboxUrl,
		"followers":                 p.FollowersUrl,
		"following":                 p.FollowingUrl,
		"publicKey": map[string]any{
			"id":           p.PublicKeyID,
			"owner":        p.ActorUrl,
			"publicKeyPem": p.PublicKeyPem,
		},
		"endpoints": map[string]any{
			"sharedInbox": sharedInboxURL(p.ActorUrl),
		},
		"alsoKnownAs": []string{siteBase + "/u/" + p.Username},
	}

	if p.DisplayName != nil && *p.DisplayName != "" {
		doc["name"] = *p.DisplayName
	}
	if p.Bio != nil && *p.Bio != "" {
		// §126: summary is HTML-escaped — the bio is plain text, and remote
		// renderers that don't sanitize must not receive raw markup.
		doc["summary"] = html.EscapeString(*p.Bio)
	}
	if p.UserCreatedAt.Valid {
		doc["published"] = p.UserCreatedAt.Time.UTC().Format(time.RFC3339)
	}
	if p.AvatarUrl != nil && *p.AvatarUrl != "" {
		doc["icon"] = map[string]any{"type": "Image", "url": absoluteSiteURL(siteBase, *p.AvatarUrl)}
	}
	if p.CoverUrl != nil && *p.CoverUrl != "" {
		doc["image"] = map[string]any{"type": "Image", "url": absoluteSiteURL(siteBase, *p.CoverUrl)}
	}
	if u := httpURL(siteBase, p.WebsiteUrl); u != "" {
		// The member's website is emitted as a PropertyValue link — only
		// http(s) URLs may ride into remote renderers; a javascript:/data:
		// scheme in the href would execute in their sanitized context.
		doc["attachment"] = []any{map[string]any{
			"type":  "PropertyValue",
			"name":  "Website",
			"value": fmt.Sprintf(`<a href="%s" rel="me nofollow noopener noreferrer" target="_blank">%s</a>`, html.EscapeString(u), html.EscapeString(u)),
		}}
	}
	return doc
}

// absoluteSiteURL makes a stored URL absolute for AP consumers: absolute
// http(s) passes through, site-relative paths ("/brand/x.jpg") anchor to
// the site origin. Remote servers can't resolve relative references.
func absoluteSiteURL(siteBase, u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return siteBase + "/" + strings.TrimPrefix(u, "/")
}

// httpURL returns the absolute http(s) form of an optional URL, or "" for
// absent/unsafe values — javascript:/data:/other schemes are dropped
// rather than propagated into remote timelines.
func httpURL(siteBase string, u *string) string {
	if u == nil || *u == "" {
		return ""
	}
	if strings.HasPrefix(*u, "http://") || strings.HasPrefix(*u, "https://") {
		return *u
	}
	if strings.HasPrefix(*u, "/") {
		return absoluteSiteURL(siteBase, *u)
	}
	return ""
}

// sharedInboxURL derives the instance sharedInbox endpoint from an actor
// URL ({api}/federation/actors/{u} → {api}/federation/inbox).
func sharedInboxURL(actorURL string) string {
	if i := strings.Index(actorURL, "/federation/actors/"); i >= 0 {
		return actorURL[:i] + "/federation/inbox"
	}
	return actorURL + "/inbox"
}

// objectDocument builds the Note/Article for a federatable contents row.
func objectDocument(c gen.GetFederatableContentByIDRow, actorURL, followersURL, siteBase, pageURL string) map[string]any {
	objType := apObjectType(c.ContentType)
	objID := objectURLFromActor(actorURL, c.ID.String())

	doc := map[string]any{
		"id":           objID,
		"type":         objType,
		"attributedTo": actorURL,
		"content":      c.BodyHtml,
		"url":          pageURL,
		"to":           []string{asPublic},
		"cc":           []string{followersURL},
		"sensitive":    false,
	}
	if objType == "Article" && c.Title != "" {
		doc["name"] = c.Title
	}
	// §126: the featured (else header) image rides as an attachment so
	// remote timelines render a preview card. URLs go through the media
	// helpers — imgproxy-signed for images, the serve endpoint for other
	// binaries — never assembled here. A non-image asset is typed
	// Document rather than mislabelled Image.
	if bucket, path, mime := attachmentMedia(c); bucket != nil && path != nil {
		att := map[string]any{"type": "Image"}
		if mime != nil && !strings.HasPrefix(*mime, "image/") {
			att["type"] = "Document"
			att["mediaType"] = *mime
			att["url"] = media.ServeURL(*bucket, *path)
		} else {
			if mime != nil {
				att["mediaType"] = *mime
			}
			if u := media.URLFromParts(bucket, path, media.ImageProxyOpts{}); u != nil {
				att["url"] = *u
			}
		}
		if _, ok := att["url"]; ok {
			doc["attachment"] = []any{att}
		}
	}
	if c.PublishedAt.Valid {
		doc["published"] = c.PublishedAt.Time.UTC().Format(time.RFC3339)
	}
	if c.UpdatedAt.Valid {
		doc["updated"] = c.UpdatedAt.Time.UTC().Format(time.RFC3339)
	}
	return doc
}

// attachmentMedia picks the featured media row, falling back to the
// header row — bucket/path/mime always come from the same row so url and
// mediaType can't describe different assets.
func attachmentMedia(c gen.GetFederatableContentByIDRow) (bucket, path, mime *string) {
	if c.FeaturedBucket != nil && c.FeaturedPath != nil {
		return c.FeaturedBucket, c.FeaturedPath, c.FeaturedMime
	}
	return c.HeaderBucket, c.HeaderPath, c.HeaderMime
}

// objectURLFromActor derives the canonical object @id from the actor URL.
// ({api}/federation/actors/{u} → {api}/federation/objects/contents/{id})
func objectURLFromActor(actorURL, contentID string) string {
	if i := strings.Index(actorURL, "/federation/actors/"); i >= 0 {
		return actorURL[:i] + "/federation/objects/contents/" + contentID
	}
	return actorURL + "/objects/" + contentID
}

// createActivity wraps an object in a Create activity for outbox items.
// published is copied only when the object carries a real timestamp —
// absent beats null under the package's no-nulls rule.
func createActivity(actorURL, followersURL string, object map[string]any) map[string]any {
	objID, _ := object["id"].(string)
	act := map[string]any{
		"id":     objID + "#create",
		"type":   "Create",
		"actor":  actorURL,
		"to":     []string{asPublic},
		"cc":     []string{followersURL},
		"object": object,
	}
	if published, ok := object["published"]; ok {
		act["published"] = published
	}
	return act
}

// orderedCollection builds the unpaginated OrderedCollection shell.
func orderedCollection(id string, total int64, firstPageURL string) map[string]any {
	return map[string]any{
		"@context":   asContext[0],
		"id":         id,
		"type":       "OrderedCollection",
		"totalItems": total,
		"first":      firstPageURL,
	}
}

// orderedCollectionPage builds one OrderedCollectionPage. next/prev are
// empty strings when the edge is reached (omitted from the document).
func orderedCollectionPage(pageID, collectionID string, items []any, total int64, hasNext, hasPrev bool, pageIDFmt func(page int) string, page int) map[string]any {
	doc := map[string]any{
		"@context":     asContext[0],
		"id":           pageID,
		"type":         "OrderedCollectionPage",
		"partOf":       collectionID,
		"totalItems":   total,
		"orderedItems": items,
	}
	if hasNext {
		doc["next"] = pageIDFmt(page + 1)
	}
	if hasPrev && page > 1 {
		doc["prev"] = pageIDFmt(page - 1)
	}
	return doc
}

// nodeInfoDocument builds the NodeInfo 2.1 stats document.
func nodeInfoDocument(version, homepage string, usage gen.GetNodeInfoUsageRow, openRegistrations bool) map[string]any {
	return map[string]any{
		"version": "2.1",
		"software": map[string]any{
			"name":       "japaconnect",
			"version":    version,
			"repository": "https://github.com/jedafe/japa",
			"homepage":   homepage,
		},
		"protocols": []string{"activitypub"},
		"services": map[string]any{
			"inbound":  []string{},
			"outbound": []string{"rss2.0"},
		},
		"openRegistrations": openRegistrations,
		"usage": map[string]any{
			"users":         map[string]any{"total": usage.TotalUsers},
			"localPosts":    usage.LocalPosts,
			"localComments": usage.LocalComments,
		},
		"metadata": map[string]any{},
	}
}

// wellKnownNodeInfo builds the /.well-known/nodeinfo discovery doc.
func wellKnownNodeInfo(href string) map[string]any {
	return map[string]any{
		"links": []any{map[string]any{
			"rel":  "http://nodeinfo.diaspora.software/ns/schema/2.1",
			"href": href,
		}},
	}
}

// tombstoneDocument is the 410-Gone body §126 serves for disabled or
// tombstoned actors — remotes read type Tombstone as terminal.
func tombstoneDocument(actorURL string) map[string]any {
	return map[string]any{
		"@context": "https://www.w3.org/ns/activitystreams",
		"id":       actorURL,
		"type":     "Tombstone",
	}
}
