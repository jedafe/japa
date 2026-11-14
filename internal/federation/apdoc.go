package federation

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/jedafe/japa/internal/db/gen"
)

// AP document builders (§126). Documents are map[string]any so optional
// fields are simply absent rather than serialized as nulls — several
// remote implementations reject unexpected nulls.

// webfingerJRD builds the WebFinger JRD for acct:{username}@{domain}.
func webfingerJRD(username, domain, actorURL, profileURL, subscribeTemplate string) map[string]any {
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
			map[string]any{
				"rel":      "http://ostatus.org/schema/1.0/subscribe",
				"template": subscribeTemplate,
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
		"discoverable":              true,
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
		doc["summary"] = *p.Bio
	}
	if p.UserCreatedAt.Valid {
		doc["published"] = p.UserCreatedAt.Time.UTC().Format(time.RFC3339)
	}
	if p.AvatarUrl != nil && *p.AvatarUrl != "" {
		doc["icon"] = map[string]any{"type": "Image", "url": *p.AvatarUrl}
	}
	if p.CoverUrl != nil && *p.CoverUrl != "" {
		doc["image"] = map[string]any{"type": "Image", "url": *p.CoverUrl}
	}
	if p.WebsiteUrl != nil && *p.WebsiteUrl != "" {
		doc["attachment"] = []any{map[string]any{
			"type":  "PropertyValue",
			"name":  "Website",
			"value": fmt.Sprintf(`<a href="%s" rel="me nofollow noopener noreferrer" target="_blank">%s</a>`, html.EscapeString(*p.WebsiteUrl), html.EscapeString(*p.WebsiteUrl)),
		}}
	}
	return doc
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
		"summary":      nil,
		"inReplyTo":    nil,
		"url":          pageURL,
		"to":           []string{asPublic},
		"cc":           []string{followersURL},
		"sensitive":    false,
	}
	if objType == "Article" && c.Title != "" {
		doc["name"] = c.Title
	}
	if c.PublishedAt.Valid {
		doc["published"] = c.PublishedAt.Time.UTC().Format(time.RFC3339)
	}
	if c.UpdatedAt.Valid {
		doc["updated"] = c.UpdatedAt.Time.UTC().Format(time.RFC3339)
	}
	return doc
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
func createActivity(actorURL, followersURL string, object map[string]any) map[string]any {
	objID, _ := object["id"].(string)
	return map[string]any{
		"id":        objID + "#create",
		"type":      "Create",
		"actor":     actorURL,
		"to":        []string{asPublic},
		"cc":        []string{followersURL},
		"published": object["published"],
		"object":    object,
	}
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
func nodeInfoDocument(version, apiBase string, usage gen.GetNodeInfoUsageRow, openRegistrations bool) map[string]any {
	return map[string]any{
		"version": "2.1",
		"software": map[string]any{
			"name":       "japaconnect",
			"version":    version,
			"repository": "https://github.com/jedafe/japa",
			"homepage":   apiBase,
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
