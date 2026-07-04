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
func GenerateQAPageJSONLD(questionTitle, questionBody, answerBody, author string, isSolved bool) map[string]interface{} {
	question := map[string]interface{}{
		"@type":       "Question",
		"name":        questionTitle,
		"text":        questionBody,
		"dateCreated": time.Now().Format(time.RFC3339),
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
// with an aggregate rating. Use for any reviewable item (mentor, service, etc.).
func GenerateReviewJSONLD(itemName string, rating float64, reviewCount int) map[string]interface{} {
	return map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "Product",
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
