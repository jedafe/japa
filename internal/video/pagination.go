package video

import (
	"net/http"
	"strconv"
)

// parsePage reads limit/offset-style pagination from query parameters.
func parsePage(r *http.Request, limitKey, offsetKey string) (limit, offset int32) {
	limit = defaultPageSize

	l := r.URL.Query().Get(limitKey)
	if l != "" {
		n, err := strconv.ParseInt(l, 10, 32)
		if err == nil && n > 0 {
			if n > maxPageSize {
				n = maxPageSize
			}

			limit = int32(n)
		}
	}

	o := r.URL.Query().Get(offsetKey)
	if o != "" {
		n, err := strconv.ParseInt(o, 10, 32)
		if err == nil && n >= 0 {
			offset = int32(n)
		}
	}

	return limit, offset
}
