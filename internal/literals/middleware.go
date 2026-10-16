package literals

import (
	"strconv"
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

// maxResponseBuffer caps how much of a JSON response is buffered for literal
// resolution. Responses larger than this are flushed through unmodified —
// resolution is a best-effort boundary, never a reason to truncate.
const maxResponseBuffer = 4 << 20 // 4 MiB

// Middleware resolves {{...}} literals in JSON response bodies at the router
// boundary. It is defense-in-depth: handlers that call a resolver explicitly
// keep working unchanged (resolution is idempotent — resolved output contains
// no "{{"), while endpoints that forget get coverage for free.
//
// Behaviour:
//   - ?raw=1 requests and websocket upgrades pass through untouched.
//   - Only JSON responses (Content-Type containing "json") are buffered;
//     HTML/RSS/files/SSE stream straight through.
//   - A mid-handler Flush or oversized body switches to passthrough so
//     streaming and large payloads are unaffected.
//   - Resolved bodies get a corrected Content-Length; status/headers are
//     otherwise preserved.
func Middleware(cat *Catalog) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cat == nil || IsRawRequest(r) || isUpgrade(r) {
				next.ServeHTTP(w, r)

				return
			}

			lw := &literalResponseWriter{
				ResponseWriter: w,
				header:         make(http.Header),
			}

			next.ServeHTTP(lw, r)
			lw.finish(cat, r)
		})
	}
}

func isUpgrade(r *http.Request) bool {
	return r.Header.Get("Upgrade") != "" ||
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// literalResponseWriter buffers JSON responses so literals can be resolved
// before the bytes hit the wire. Headers are captured in a private map and
// forwarded at finish/flush time.
type literalResponseWriter struct {
	http.ResponseWriter
	header      http.Header
	status      int
	buf         bytes.Buffer
	passthrough bool
}

func (w *literalResponseWriter) Header() http.Header { return w.header }

func (w *literalResponseWriter) WriteHeader(code int) {
	if w.passthrough {
		w.ResponseWriter.WriteHeader(code)

		return
	}

	if w.status != 0 {
		return // first WriteHeader wins, mirroring net/http semantics
	}

	w.status = code

	if !isJSONHeader(w.header) {
		// Non-JSON (HTML, RSS, downloads): start streaming immediately.
		w.flushHead()
	}
}

func (w *literalResponseWriter) Write(p []byte) (int, error) {
	if w.passthrough {
		return w.ResponseWriter.Write(p)
	}

	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	if w.passthrough {
		return w.ResponseWriter.Write(p)
	}

	if w.buf.Len()+len(p) > maxResponseBuffer {
		// Oversized JSON — flush head + buffered bytes and stream the rest.
		w.flushHead()

		if _, err := w.buf.WriteTo(w.ResponseWriter); err != nil {
			return 0, err
		}

		return w.ResponseWriter.Write(p)
	}

	return w.buf.Write(p)
}

// Flush forces any buffered head/body to the wire and switches to passthrough.
func (w *literalResponseWriter) Flush() {
	if !w.passthrough {
		if w.status == 0 {
			w.status = http.StatusOK
		}

		w.flushHead()
		_, _ = w.buf.WriteTo(w.ResponseWriter)
	}

	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack delegates to the underlying writer for protocol upgrades.
func (w *literalResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("underlying writer does not support hijacking")
	}

	return h.Hijack()
}

// Push delegates HTTP/2 server-push support when present.
func (w *literalResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}

	return http.ErrNotSupported
}

// ReadFrom buffers into the response body (satisfies io.ReaderFrom). The
// content-type classification must happen first — handlers that stream via
// io.Copy/ServeContent may never call Write explicitly.
func (w *literalResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	if w.passthrough {
		return io.Copy(w.ResponseWriter, r)
	}

	n, err := w.buf.ReadFrom(io.LimitReader(r, maxResponseBuffer+1))

	if w.buf.Len() > maxResponseBuffer {
		w.flushHead()

		_, _ = w.buf.WriteTo(w.ResponseWriter)
		if err == nil {
			_, err = io.Copy(w.ResponseWriter, r)
		}
	}

	return n, err
}

// flushHead writes captured headers + status to the real writer and switches
// to passthrough mode. Callers must have set w.status already.
func (w *literalResponseWriter) flushHead() {
	copyHeaders(w.ResponseWriter.Header(), w.header)
	w.ResponseWriter.WriteHeader(w.status)
	w.passthrough = true
}

// finish is called after the handler returns. Buffered JSON bodies get
// resolved; everything else has already streamed through.
func (w *literalResponseWriter) finish(cat *Catalog, r *http.Request) {
	if w.passthrough {
		return // already streamed (non-JSON, flushed, or oversized)
	}

	body := w.buf.Bytes()

	if w.status == 0 {
		w.status = http.StatusOK
	}

	if w.status != http.StatusNoContent && len(body) > 0 && bytes.Contains(body, []byte("{{")) {
		body = cat.ResolveJSON(r.Context(), body, NewRequestContext(r, ""))
	}

	w.header.Del("Content-Length")

	if len(body) > 0 {
		w.header.Set("Content-Length", strconv.Itoa(len(body)))
	}

	copyHeaders(w.ResponseWriter.Header(), w.header)
	w.ResponseWriter.WriteHeader(w.status)

	if len(body) > 0 && w.status != http.StatusNoContent {
		_, _ = w.ResponseWriter.Write(body)
	}
}

func isJSONHeader(h http.Header) bool {
	return strings.Contains(strings.ToLower(h.Get("Content-Type")), "json")
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
