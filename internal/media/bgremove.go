package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"math"
	"path"
	"path/filepath"
	"strings"

	webpenc "github.com/gen2brain/webp"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"golang.org/x/image/draw"
)

// Edge-background knockout constants. Badge/challenge artwork routinely
// arrives on a solid matte (white PNG exports, stock previews) that reads
// as an ugly square inside the circular badge shield. KnockoutEdgeTolerance
// is the max RGB distance (per channel, Euclidean) a pixel may sit from the
// sampled corner color and still count as background. The corner-agreement
// gate (knockoutMinCorners) is what actually distinguishes a matte from
// artwork that bleeds to the edge; the fraction cap only guards blanks.
const (
	// knockoutEdgeTolerance mirrors ImageMagick -fuzz ~12%.
	knockoutEdgeTolerance = 55.0
	// knockoutMaxFraction caps the removable share of the canvas — a flood
	// covering ~everything means the upload was blank, not matted.
	knockoutMaxFraction = 0.98
	// knockoutMinCorners is how many corner patches must agree on the
	// background color before anything is erased — a real matte is uniform
	// across edges; artwork bleeding to the edge is not.
	knockoutMinCorners = 3
	// knockoutCornerPatch is the corner patch side averaged for each seed.
	knockoutCornerPatch = 6
	// knockoutWebPQuality is the WebP encode quality for re-encoded art.
	knockoutWebPQuality = 90
)

// knockoutMIME is the MIME type the knockout emits — WebP keeps alpha and
// is the same output the optimizer emits.
const knockoutMIME = OptimizeMIME

// RemoveEdgeBackground knocks out a solid edge background on a stored media
// object and downscales it to maxDim pixels on its longest edge so the
// artwork fits the UI frame it attaches to (badge shield, challenge hero):
// reads the raster, resizes, flood-fills alpha=0 from each corner through
// pixels matching that corner's sampled color, re-encodes as WebP, rewrites
// storage (path extension follows the format change), and updates the media
// row. Idempotent — already-transparent right-sized artwork and non-raster
// rows (gif/svg/video) return nil without touching storage. maxDim <= 0
// skips the resize.
//
// Invoked when artwork attaches to an entity (badge icon_media_id,
// challenge image_media_id) — see gamification.knockoutArtwork.
func (s *Service) RemoveEdgeBackground(ctx context.Context, mediaID pgtype.UUID, maxDim int) error {
	m, err := s.store.GetMediaByID(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("loading media %s: %w", mediaID.String(), err)
	}

	canonical := CanonicalMIMEType(m.MimeType)
	if !strings.HasPrefix(canonical, "image/") ||
		canonical == "image/gif" ||
		canonical == "image/svg+xml" {
		return nil // nothing knockable — skip the object fetch entirely
	}

	if m.QuarantinedAt.Valid || m.RejectedAt.Valid {
		return nil
	}

	data, err := s.fetchObjectBytes(ctx, m.StorageBucket, m.StoragePath, MaxMediaUploadBody)
	if err != nil {
		if errors.Is(err, errObjectMissing) {
			slog.Warn("media: background knockout skipped — stored object missing",
				"media_id", mediaID.String(),
				"storage_path", m.StoragePath)

			return nil
		}

		return fmt.Errorf("reading stored object %s/%s: %w", m.StorageBucket, m.StoragePath, err)
	}

	knocked, outMIME := removeEdgeBackground(data, m.MimeType, maxDim)
	if outMIME == m.MimeType && bytes.Equal(knocked, data) {
		return nil // already transparent, no solid edge matte, or un-decodable
	}

	// Rewrite the stored object under a content-hashed name — ServeMedia
	// caches a path for a day on the assumption that object bytes are
	// immutable, so every content change must land under a new path or
	// browsers keep serving the stale (pre-knockout) file.
	newPath := transformedStoragePath(m.StoragePath, knocked, outMIME)

	stored := false

	if s.s3 != nil {
		if err := s.s3.putObject(ctx, m.StorageBucket, newPath, outMIME, knocked); err != nil {
			slog.Warn("media: knockout s3 put failed, trying local fallback",
				"media_id", mediaID.String(), "storage_path", newPath, "error", err)
		} else {
			stored = true
		}
	}

	if !stored {
		if err := storeLocalMediaFile(m.StorageBucket, newPath, knocked); err != nil {
			return fmt.Errorf("storing knocked-out object: %w", err)
		}
	}

	params := gen.UpdateMediaStorageParams{
		ID:            mediaID,
		StoragePath:   newPath,
		MimeType:      outMIME,
		FileSizeBytes: int64(len(knocked)),
	}
	// Keep width_px/height_px in sync with the rewritten object — the
	// downscale may have shrunk the canvas.
	if ocfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(knocked)); cfgErr == nil {
		w, h := int32(ocfg.Width), int32(ocfg.Height) // #nosec G115 -- bounded: pagination clamp or config/settings value, cannot reach int32 overflow
		params.WidthPx, params.HeightPx = &w, &h
	}

	if err := s.store.UpdateMediaStorage(ctx, params); err != nil {
		return fmt.Errorf("updating media row: %w", err)
	}

	slog.Info("media: knocked out edge background",
		"media_id", mediaID.String(),
		"old_bytes", len(data),
		"new_bytes", len(knocked),
		"old_mime", m.MimeType,
		"new_mime", outMIME,
	)

	if newPath != m.StoragePath {
		_ = s.deleteObject(ctx, m.StorageBucket, m.StoragePath)
	}

	return nil
}

// transformedStoragePath names a rewritten object after its content —
// stem-<hash8>.<ext>. ServeMedia treats object paths as immutable
// (day-long cache), so a rewrite that changes bytes must also change the
// path or clients hold the stale object. Deterministic naming also makes
// rewrites naturally idempotent: identical output lands at the same path.
func transformedStoragePath(oldPath string, data []byte, mime string) string {
	dir, file := path.Split(oldPath)
	stem := strings.TrimSuffix(file, filepath.Ext(file))
	ext := extensionFromMIME(mime)
	if ext == "" {
		ext = filepath.Ext(oldPath)
	}
	sum := sha256.Sum256(data)
	return dir + stem + "-" + hex.EncodeToString(sum[:4]) + ext
}

// removeEdgeBackground decodes a raster image, downscales it to maxDim on
// its longest edge, and returns it with its solid edge background made
// transparent, re-encoded as WebP. Returns the input unchanged when the
// canvas is too large to decode safely or nothing changed (right-sized
// transparent art, or opaque art with no matte). Unlike the optimizer,
// byte-size inflation does not veto the transform — transparency and
// frame-fit are the point, not size. maxDim <= 0 skips the resize.
func removeEdgeBackground(data []byte, mimeType string, maxDim int) ([]byte, string) {
	canonical := CanonicalMIMEType(mimeType)
	if !strings.HasPrefix(canonical, "image/") ||
		canonical == "image/gif" ||
		canonical == "image/svg+xml" {
		return data, mimeType
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || int64(cfg.Width)*int64(cfg.Height) > optimizeMaxPixels {
		return data, mimeType
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, mimeType
	}

	img := toNRGBA(src)
	changed := false

	// Fit the artwork to its frame — badge shields and hero strips never
	// need the multi-thousand-pixel canvas stock art ships at.
	if maxDim > 0 {
		if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w > maxDim || h > maxDim {
			img = downscaleNRGBA(img, maxDim)
			changed = true
		}
	}

	// Already-transparent artwork skips the flood — only the resize may
	// still rewrite it.
	if !hasAlphaTransparency(img) {
		if floodCorners(img) > 0 {
			changed = true
		}
	}

	if !changed {
		return data, mimeType
	}

	var buf bytes.Buffer
	if err := webpenc.Encode(&buf, img, webpenc.Options{Quality: knockoutWebPQuality}); err != nil {
		return data, mimeType
	}

	return buf.Bytes(), knockoutMIME
}

// toNRGBA converts any decoded image to NRGBA.
func toNRGBA(src image.Image) *image.NRGBA {
	if n, ok := src.(*image.NRGBA); ok {
		return n
	}
	b := src.Bounds()
	img := image.NewNRGBA(b)
	draw.Draw(img, b, src, b.Min, draw.Src)
	return img
}

// downscaleNRGBA resizes img so its longest edge is maxDim pixels, using
// CatmullRom (same scaler as the storage optimizer).
func downscaleNRGBA(img *image.NRGBA, maxDim int) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := float64(maxDim) / float64(max(w, h))
	nw := max(1, int(float64(w)*scale+0.5))
	nh := max(1, int(float64(h)*scale+0.5))
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

// hasAlphaTransparency reports whether the image already contains any
// non-opaque pixel.
func hasAlphaTransparency(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] < 255 {
			return true
		}
	}
	return false
}

// floodCorners clears alpha on every pixel reachable from a corner through
// the sampled background color, but only when at least knockoutMinCorners
// corner patches agree on that color — the signature of a real matte.
// Returns the number of pixels cleared; 0 or a share above
// knockoutMaxFraction leaves the image untouched (the edge is the artwork
// or the upload was blank).
func floodCorners(img *image.NRGBA) int {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	total := w * h

	if total == 0 {
		return 0
	}

	corners := [4]image.Point{
		{X: b.Min.X, Y: b.Min.Y},
		{X: b.Max.X - 1, Y: b.Min.Y},
		{X: b.Min.X, Y: b.Max.Y - 1},
		{X: b.Max.X - 1, Y: b.Max.Y - 1},
	}

	var seeds [4][3]float64
	for i, c := range corners {
		seeds[i] = patchMean(img, c.X, c.Y, knockoutCornerPatch)
	}

	cluster := largestSimilarCluster(seeds, knockoutEdgeTolerance)
	if len(cluster) < knockoutMinCorners {
		return 0
	}

	var seed [3]float64
	for _, i := range cluster {
		for k := 0; k < 3; k++ {
			seed[k] += seeds[i][k]
		}
	}
	for k := 0; k < 3; k++ {
		seed[k] /= float64(len(cluster))
	}

	mask := make([]bool, total)
	cleared := 0

	for _, i := range cluster {
		cleared += floodFill(img, corners[i].X, corners[i].Y, seed, mask)
	}

	// Border-survivor gate — if any opaque pixel still touches the edge,
	// the artwork bleeds to the border; what flooded wasn't a matte (e.g.
	// a photo whose sky shared a corner color). Restore and bail.
	if cleared > 0 && borderHasOpaque(img) {
		restoreAlpha(img)
		return 0
	}

	if cleared == 0 || float64(cleared) > float64(total)*knockoutMaxFraction {
		restoreAlpha(img)
		return 0
	}

	return cleared
}

// restoreAlpha resets every pixel opaque — valid because the flood only
// runs on inputs hasAlphaTransparency already proved fully opaque.
func restoreAlpha(img *image.NRGBA) {
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
}

// borderHasOpaque reports whether any non-transparent pixel remains on the
// image edge.
func borderHasOpaque(img *image.NRGBA) bool {
	b := img.Bounds()

	for x := b.Min.X; x < b.Max.X; x++ {
		if img.Pix[img.PixOffset(x, b.Min.Y)+3] != 0 ||
			img.Pix[img.PixOffset(x, b.Max.Y-1)+3] != 0 {
			return true
		}
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		if img.Pix[img.PixOffset(b.Min.X, y)+3] != 0 ||
			img.Pix[img.PixOffset(b.Max.X-1, y)+3] != 0 {
			return true
		}
	}

	return false
}

// largestSimilarCluster returns the biggest set of seeds all within tol of
// a single seed's color (star clustering — a matte's corners agree with at
// least one common background color).
func largestSimilarCluster(seeds [4][3]float64, tol float64) []int {
	best := []int{}

	for i := 0; i < len(seeds); i++ {
		var cl []int

		for j := 0; j < len(seeds); j++ {
			if i == j || colorDist(seeds[i], seeds[j]) <= tol {
				cl = append(cl, j)
			}
		}

		if len(cl) > len(best) {
			best = cl
		}
	}

	return best
}

// colorDist is the Euclidean distance between two RGB triples.
func colorDist(a, b [3]float64) float64 {
	dr := a[0] - b[0]
	dg := a[1] - b[1]
	db := a[2] - b[2]
	return math.Sqrt(dr*dr + dg*dg + db*db)
}

// patchMean averages the RGB of a small square patch anchored at (x,y),
// clamped inside the image — a more stable seed color than a single corner
// pixel, which may sit on a JPEG artifact.
func patchMean(img *image.NRGBA, x, y, size int) [3]float64 {
	b := img.Bounds()

	x0 := clamp(x-size/2, b.Min.X, b.Max.X-1)
	y0 := clamp(y-size/2, b.Min.Y, b.Max.Y-1)
	x1 := clamp(x0+size, b.Min.X, b.Max.X)
	y1 := clamp(y0+size, b.Min.Y, b.Max.Y)

	var sr, sg, sb, n float64

	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			i := img.PixOffset(xx, yy)
			sr += float64(img.Pix[i])
			sg += float64(img.Pix[i+1])
			sb += float64(img.Pix[i+2])
			n++
		}
	}

	if n == 0 {
		return [3]float64{}
	}

	return [3]float64{sr / n, sg / n, sb / n}
}

// floodFill clears alpha on every pixel reachable from (sx,sy) whose RGB
// sits within knockoutEdgeTolerance of seed, 8-connected. Cleared pixels are
// marked in mask so the four corner floods do not recount each other.
func floodFill(img *image.NRGBA, sx, sy int, seed [3]float64, mask []bool) int {
	b := img.Bounds()
	w := b.Dx()

	if sx < b.Min.X || sx >= b.Max.X || sy < b.Min.Y || sy >= b.Max.Y {
		return 0
	}

	stack := []image.Point{{X: sx, Y: sy}}
	cleared := 0

	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if p.X < b.Min.X || p.X >= b.Max.X || p.Y < b.Min.Y || p.Y >= b.Max.Y {
			continue
		}

		mi := (p.Y-b.Min.Y)*w + (p.X - b.Min.X)
		if mask[mi] {
			continue
		}
		mask[mi] = true // visited — never re-check, matched or not

		i := img.PixOffset(p.X, p.Y)
		dr := float64(img.Pix[i]) - seed[0]
		dg := float64(img.Pix[i+1]) - seed[1]
		db := float64(img.Pix[i+2]) - seed[2]

		if math.Sqrt(dr*dr+dg*dg+db*db) > knockoutEdgeTolerance {
			continue
		}

		img.Pix[i+3] = 0
		cleared++

		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx != 0 || dy != 0 {
					stack = append(stack, image.Point{X: p.X + dx, Y: p.Y + dy})
				}
			}
		}
	}

	return cleared
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
