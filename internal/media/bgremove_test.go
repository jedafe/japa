package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// whiteMatte renders an opaque white canvas with a colored disc in the
// middle — the classic badge-art upload shape.
func whiteMatte(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, 255})
		}
	}
	cx, cy, r := w/2, h/2, w/5
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				img.SetNRGBA(x, y, color.NRGBA{212, 175, 55, 255})
			}
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func decodeNRGBA(t *testing.T, data []byte) *image.NRGBA {
	t.Helper()
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return toNRGBA(src)
}

func TestRemoveEdgeBackgroundKnocksOutWhiteMatteAndDownscales(t *testing.T) {
	in := encodePNG(t, whiteMatte(1024, 1024))

	out, outMIME := removeEdgeBackground(in, "image/png", 512)
	if outMIME != knockoutMIME {
		t.Fatalf("mime = %q, want %q", outMIME, knockoutMIME)
	}

	img := decodeNRGBA(t, out)
	if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w > 512 || h > 512 {
		t.Fatalf("dims %dx%d exceed maxDim 512", w, h)
	}

	b := img.Bounds()
	for _, p := range [4]image.Point{{0, 0}, {b.Max.X - 1, 0}, {0, b.Max.Y - 1}, {b.Max.X - 1, b.Max.Y - 1}} {
		if _, _, _, a := img.At(p.X, p.Y).RGBA(); a != 0 {
			t.Errorf("corner %v alpha = %d, want 0", p, a)
		}
	}
	if _, _, _, a := img.At(b.Max.X/2, b.Max.Y/2).RGBA(); a != 0xffff {
		t.Error("center pixel lost its alpha — flood ate the artwork")
	}
}

func TestRemoveEdgeBackgroundSkipsTransparent(t *testing.T) {
	img := whiteMatte(200, 200)
	img.SetNRGBA(0, 0, color.NRGBA{255, 255, 255, 0})
	in := encodePNG(t, img)

	out, outMIME := removeEdgeBackground(in, "image/png", 512)
	if outMIME != "image/png" || !bytes.Equal(out, in) {
		t.Error("already-transparent input was rewritten")
	}
}

func TestRemoveEdgeBackgroundAbortsOnFullBleedArt(t *testing.T) {
	// A canvas that is ~all "background color" — the flood would erase
	// almost everything, so the transform must bail out.
	img := image.NewNRGBA(image.Rect(0, 0, 400, 400))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	in := encodePNG(t, img)

	out, outMIME := removeEdgeBackground(in, "image/png", 0)
	if outMIME != "image/png" || !bytes.Equal(out, in) {
		t.Error("full-bleed canvas was rewritten — flood cap failed")
	}
}

func TestRemoveEdgeBackgroundLeavesOpaqueNoMatte(t *testing.T) {
	// Opaque art that reaches every edge in a different color: no corner
	// cluster can form, so nothing may be erased.
	img := image.NewNRGBA(image.Rect(0, 0, 300, 300))
	quads := [4]color.NRGBA{
		{200, 30, 30, 255},
		{30, 200, 30, 255},
		{30, 30, 200, 255},
		{200, 200, 30, 255},
	}
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			i := 0
			if x >= 150 {
				i++
			}
			if y >= 150 {
				i += 2
			}
			img.SetNRGBA(x, y, quads[i])
		}
	}
	in := encodePNG(t, img)

	out, _ := removeEdgeBackground(in, "image/png", 0)
	decoded := decodeNRGBA(t, out)
	if hasAlphaTransparency(decoded) {
		t.Error("four-corner-distinct art gained transparent pixels")
	}
}
