package seo

import (
	"fmt"
	"time"
)

// GenerateArticleJSONLD produces a schema.org Article structured data object.
// Use for blog posts, guides, and long-form content.
func GenerateArticleJSONLD(title, description, author, imageURL, publishedAt, updatedAt string) map[string]interface{} {
	article := map[string]interface{}{
		"@context":      "https://schema.org",
		"@type":         "Article",
		"title":         title,
		"description":   description,
		"author":        map[string]interface{}{"@type": "Person", "name": author},
		"datePublished": publishedAt,
	}

	if imageURL != "" {
		article["image"] = imageURL
	}

	if updatedAt != "" {
		article["dateModified"] = updatedAt
	}

	return article
}

// GenerateQAPageJSONLD produces a schema.org QAPage structured data object.
// For solved threads, set isSolved to true and provide a non-empty answerBody.
// dateCreated should be the thread's actual creation timestamp (RFC3339);
// when empty, the current time is used as a last resort.
func GenerateQAPageJSONLD(questionTitle, questionBody, answerBody, author, dateCreated string, isSolved bool) map[string]interface{} {
	created := dateCreated
	if created == "" {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	question := map[string]interface{}{
		"@type":       "Question",
		"name":        questionTitle,
		"text":        questionBody,
		"dateCreated": created,
		"author":      map[string]interface{}{"@type": "Person", "name": author},
		"answerCount": 0,
		"isAccepted":  false,
	}

	if isSolved && answerBody != "" {
		question["answerCount"] = 1
		question["acceptedAnswer"] = map[string]interface{}{
			"@type":  "Answer",
			"text":   answerBody,
			"author": map[string]interface{}{"@type": "Person", "name": author},
		}
	}

	return map[string]interface{}{
		"@context":   "https://schema.org",
		"@type":      "QAPage",
		"mainEntity": question,
	}
}

// GenerateReviewJSONLD produces a schema.org Review structured data object
// with an aggregate rating. entityType is the schema.org @type for the
// reviewed entity (e.g., "Service", "Person", "Organization"); it defaults
// to "Product" when empty for backwards compatibility. Use the entity's
// real type per Section 26's structured-data table — "Product" is wrong
// for mentors, services, and organizations.
func GenerateReviewJSONLD(itemName string, rating float64, reviewCount int, entityType string) map[string]interface{} {
	et := entityType
	if et == "" {
		et = "Product"
	}
	return map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    et,
		"name":     itemName,
		"aggregateRating": map[string]interface{}{
			"@type":       "AggregateRating",
			"ratingValue": fmt.Sprintf("%.1f", rating),
			"bestRating":  "5",
			"reviewCount": reviewCount,
		},
	}
}

// GenerateOrganizationJSONLD produces a schema.org Organization structured data object.
// Use on every page for brand identity.
func GenerateOrganizationJSONLD(name, logo, url string) map[string]interface{} {
	org := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "Organization",
		"name":     name,
	}
	if logo != "" {
		org["logo"] = logo
	}

	if url != "" {
		org["url"] = url
	}

	return org
}

// GenerateCourseJSONLD produces a schema.org Course structured data object.
// Use for learning section content.
func GenerateCourseJSONLD(name, description, provider, url string) map[string]interface{} {
	course := map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "Course",
		"name":        name,
		"description": description,
		"provider":    map[string]interface{}{"@type": "Organization", "name": provider},
	}
	if url != "" {
		course["url"] = url
	}

	return course
}

// BreadcrumbItem represents a single breadcrumb in a BreadcrumbList.
type BreadcrumbItem struct {
	Name string
	URL  string
}

// GenerateBreadcrumbListJSONLD produces a schema.org BreadcrumbList structured
// data object from an ordered list of crumbs (root → leaf). Used on all pages
// with hierarchy (blog posts, forum threads, space hubs, learning lessons)
// per Section 26's structured-data table.
// Ref: japaconnect-engineering/sections/26-seo-performance.md
func GenerateBreadcrumbListJSONLD(items []BreadcrumbItem) map[string]interface{} {
	elements := make([]map[string]interface{}, 0, len(items))
	for i, item := range items {
		entry := map[string]interface{}{
			"@type":    "ListItem",
			"position": i + 1,
			"name":     item.Name,
		}
		if item.URL != "" {
			entry["item"] = item.URL
		}
		elements = append(elements, entry)
	}
	return map[string]interface{}{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": elements,
	}
}
