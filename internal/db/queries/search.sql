-- name: LogSearchQuery :exec
INSERT INTO search_analytics (tenant_id, query, results_count, user_id, content_type)
VALUES ($1, $2, $3, $4, $5);

-- name: GetSearchAnalyticsSummary :many
SELECT
    query,
    COUNT(*) AS search_count,
    AVG(results_count)::int AS avg_results,
    COUNT(*) FILTER (WHERE results_count = 0) AS zero_result_count
FROM search_analytics
WHERE tenant_id = $1
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
GROUP BY query
ORDER BY search_count DESC
LIMIT $4 OFFSET $5;

-- name: GetZeroResultQueries :many
SELECT query, COUNT(*) AS search_count
FROM search_analytics
WHERE tenant_id = $1
  AND results_count = 0
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
GROUP BY query
ORDER BY search_count DESC
LIMIT $4 OFFSET $5;

-- name: GetSearchVolumeByDay :many
SELECT
    DATE(created_at) AS day,
    COUNT(*) AS total_searches,
    COUNT(*) FILTER (WHERE results_count = 0) AS zero_result_searches
FROM search_analytics
WHERE tenant_id = $1
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3)
GROUP BY DATE(created_at)
ORDER BY day DESC;
