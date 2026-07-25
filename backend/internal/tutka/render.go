package tutka

import (
	"bytes"
	"image"
	"image/png"
	"time"
)

// RenderPNG colours a frame with a palette and encodes it as a transparent PNG
// ready for a MapLibre image source.
//
// The work is a lookup-table index per pixel — see Palette.LUT — so the cost is
// dominated by PNG encoding rather than by the colour maths. Compression is left
// at the default rather than pushed to best: the result is cached in memory and
// re-derivable from the archived frame, so trading a few percent of size for a
// noticeably faster first response is the right way round.
func RenderPNG(f *Frame, pal Palette) ([]byte, error) {
	lut := pal.LUT(f)
	img := image.NewNRGBA(image.Rect(0, 0, f.Grid.Width, f.Grid.Height))

	for i, raw := range f.Values {
		c := lut[raw]
		if c.A == 0 {
			continue // leave the pixel fully transparent
		}
		o := i * 4
		img.Pix[o] = c.R
		img.Pix[o+1] = c.G
		img.Pix[o+2] = c.B
		img.Pix[o+3] = c.A
	}

	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderedFrame returns the coloured PNG for one frame, caching the result.
//
// Concurrent requests for the same frame and palette collapse onto a single
// render. That is not a micro-optimisation: a client starting the animation asks
// for a dozen frames at once, and several clients doing so together would
// otherwise each decode and encode the same megapixel rasters.
func (s *Store) RenderedFrame(product string, ts time.Time, pal Palette) ([]byte, error) {
	key := frameKey(product, ts) + "@" + pal.ID
	if body, ok := s.rendered.get(key); ok {
		return body, nil
	}

	body, err, _ := s.sf.Do(key, func() (any, error) {
		// Re-check inside the flight: a request that queued behind the leader
		// should take its result rather than redo the work.
		if cached, ok := s.rendered.get(key); ok {
			return cached, nil
		}
		f, err := s.Get(product, ts)
		if err != nil {
			return nil, err
		}
		rendered, err := RenderPNG(f, pal)
		if err != nil {
			return nil, err
		}
		s.rendered.put(key, rendered)
		return rendered, nil
	})
	if err != nil {
		return nil, err
	}
	return body.([]byte), nil
}
