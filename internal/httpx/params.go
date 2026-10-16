// Package httpx provides small HTTP request helpers shared across handlers.
package httpx

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
)

// URLParamDecoded returns a chi URL parameter percent-decoded. Chi matches
// routes on the escaped path, so a request for /users/youtube%3Arick-astley
// captures "youtube%3Arick-astley" while the literal-colon form captures
// "youtube:rick-astley" — two spellings of the same resource that resolve
// differently. Decoding here makes both forms equivalent (synthetic video
// channel usernames legitimately contain ':'). Malformed escapes fall back
// to the raw segment.
func URLParamDecoded(r *http.Request, key string) string {
	raw := chi.URLParam(r, key)
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}

	return decoded
}
