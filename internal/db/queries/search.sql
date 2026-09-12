-- name: LogSearchQuery :exec
INSERT INTO search_analytics (query, results_count, user_id, content_type) VALUES ($1, $2, $3, $4)
;

-- name: GetSearchAnalyticsSummary :many
SELECT
 query,
 COUNT(*) AS search_count,
 AVG(results_count)::int AS avg_results,
 COUNT(*) FILTER (WHERE results_count = 0) AS zero_result_count
FROM search_analytics
WHERE ($1::timestamptz IS NULL OR created_at >= $1)
 AND ($2::timestamptz IS NULL OR created_at <= $2)
GROUP BY query
ORDER BY search_count DESC
LIMIT $3 OFFSET $4;

-- name: GetZeroResultQueries :many
SELECT query, COUNT(*) AS search_count
FROM search_analytics
WHERE results_count = 0
 AND ($1::timestamptz IS NULL OR created_at >= $1)
 AND ($2::timestamptz IS NULL OR created_at <= $2)
GROUP BY query
ORDER BY search_count DESC
LIMIT $3 OFFSET $4;

-- name: GetSearchVolumeByDay :many
SELECT
 DATE(created_at) AS day,
 COUNT(*) AS total_searches,
 COUNT(*) FILTER (WHERE results_count = 0) AS zero_result_searches
FROM search_analytics
WHERE ($1::timestamptz IS NULL OR created_at >= $1)
 AND ($2::timestamptz IS NULL OR created_at <= $2)
GROUP BY DATE(created_at)
ORDER BY day DESC;
