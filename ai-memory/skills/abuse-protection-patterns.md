# Skill: Abuse Protection Patterns (Rate Limiting, Brute-Force, AI Quota)

See ADR-022 for the why. This file is the how.

## Nginx coarse limiting (first line, per-IP)
```nginx
limit_req_zone $binary_remote_addr zone=general:10m rate=10r/s;
location /api/ {
    limit_req zone=general burst=20 nodelay;
    proxy_pass http://backend;
}
```

## Application-level limiter (Valkey-backed, per-user/per-feature)
```go
func RateLimit(ctx context.Context, rdb RedisClient, key string, limit int, window time.Duration) (bool, error) {
    count, err := rdb.Incr(ctx, key).Result()
    if err != nil {
        return false, err
    }
    if count == 1 {
        rdb.Expire(ctx, key, window)
    }
    return count <= int64(limit), nil
}

// Usage — tighter limits on expensive/sensitive paths than on simple reads
func AIGatewayMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID := getUserID(r)
        key := fmt.Sprintf("jc:ratelimit:ai_gateway:%s", userID)
        ok, _ := RateLimit(r.Context(), rdb, key, dailyAIQuotaFor(userID), 24*time.Hour)
        if !ok {
            http.Error(w, "AI usage quota exceeded for today", http.StatusTooManyRequests)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

## Login brute-force protection (exponential backoff)
```go
func CheckLoginAttempt(ctx context.Context, rdb RedisClient, identifier string) (allowed bool, retryAfter time.Duration) {
    key := "jc:ratelimit:login_fail:" + identifier
    failCount, _ := rdb.Get(ctx, key).Int()
    if failCount >= 5 {
        backoff := time.Duration(math.Pow(2, float64(failCount-4))) * time.Second
        return false, backoff
    }
    return true, 0
}

func RecordFailedLogin(ctx context.Context, rdb RedisClient, identifier string) {
    key := "jc:ratelimit:login_fail:" + identifier
    rdb.Incr(ctx, key)
    rdb.Expire(ctx, key, 15*time.Minute) // window resets after 15 min of no further failures
}
```

## Rule of thumb for new endpoints
Every new endpoint gets an explicit tier at design time, not the application default by accident:
- **Tight** (password reset, AI Gateway calls, login): per-user + per-IP, short window.
- **Moderate** (search, content creation): per-user, longer window.
- **Loose** (read-only public pages): Nginx-level only, no application-level check needed.
