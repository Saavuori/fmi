package tutka

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"fmi/internal/core/cache"
)

// Layout on disk: <dir>/<product>/meta.json plus <dir>/<product>/<YYYYMMDD>/<HHMM>.png
//
// Frames are files rather than rows in a database, which is the one place this
// app deliberately parts company with its Fintraffic sibling's SQLite trail
// store. A few thousand ~100 KB blobs in SQLite means pruning either fragments
// the file or needs a VACUUM, and VACUUM wants scratch space equal to the
// database — on a host whose root volume runs near full that is the wrong
// trade. As files, pruning is unlink, and `du` tells the truth about usage.
const (
	metaFileName = "meta.json"
	dayLayout    = "20060102"
	minuteLayout = "1504"
)

// decodedCacheSize and renderedCacheSize bound the in-memory caches. A decoded
// frame is ~2.4 MB at the default grid, so the archive cannot live in RAM: 24
// frames covers the animation's recent window and the nowcast's needs, and the
// rest is read back from disk on demand.
const (
	decodedCacheSize  = 24
	renderedCacheSize = 64
)

// productMeta is the per-product sidecar. It records the grid the frames were
// fetched on, which matters more than it looks: if the operator changes
// GRID_BBOX or GRID_RESOLUTION, every archived frame becomes geometrically wrong
// for the new configuration, and silently serving them would put the weather in
// the wrong place. Open() detects the mismatch and discards the archive instead.
type productMeta struct {
	Grid   Grid    `json:"grid"`
	Bits   int     `json:"bits"`
	Gain   float64 `json:"gain"`
	Offset float64 `json:"offset"`
}

// Store is the frame archive: pollers write here, handlers read here, and no
// visitor request ever reaches FMI.
type Store struct {
	dir   string // "" disables the on-disk archive
	grid  Grid
	cache cache.Cache

	mu    sync.RWMutex
	index map[string]map[int64]bool // product -> unix seconds -> present
	meta  map[string]productMeta

	decoded  *lru[string, *Frame]
	rendered *lru[string, []byte]
	sf       singleflight.Group
}

// NewStore prepares the archive. A non-empty dir is created if missing; if that
// fails the archive is disabled and the mode carries on serving whatever it holds
// in memory, because a read-only volume should degrade the app rather than stop
// it.
func NewStore(dir string, grid Grid, c cache.Cache) *Store {
	s := &Store{
		dir:      dir,
		grid:     grid,
		cache:    c,
		index:    make(map[string]map[int64]bool),
		meta:     make(map[string]productMeta),
		decoded:  newLRU[string, *Frame](decodedCacheSize),
		rendered: newLRU[string, []byte](renderedCacheSize),
	}
	if dir == "" {
		log.Println("Tutka: FRAMES_DIR is empty, archive disabled (frames held in memory only)")
		return s
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("Tutka: cannot create %s (%v), archive disabled", dir, err)
		s.dir = ""
	}
	return s
}

// ArchiveEnabled reports whether frames are being persisted.
func (s *Store) ArchiveEnabled() bool { return s.dir != "" }

// Open scans the archive and rebuilds the in-memory index. At a few thousand
// files this is a handful of milliseconds, which is why the index is not itself
// persisted — a derived structure that can be rebuilt cheaply is one less thing
// that can go stale or corrupt.
//
// Products whose stored grid no longer matches the configured one are cleared:
// see productMeta.
func (s *Store) Open() {
	if s.dir == "" {
		return
	}
	for _, p := range Products {
		productDir := filepath.Join(s.dir, p.ID)
		if _, err := os.Stat(productDir); errors.Is(err, fs.ErrNotExist) {
			continue
		}

		meta, err := s.readMeta(p.ID)
		if err != nil {
			log.Printf("Tutka: %s has no readable meta.json (%v), discarding its archive", p.ID, err)
			s.clearProduct(p.ID)
			continue
		}
		if !sameGrid(meta.Grid, s.grid) {
			log.Printf("Tutka: %s was archived on a %dx%d grid but the configured grid is %dx%d — discarding %s",
				p.ID, meta.Grid.Width, meta.Grid.Height, s.grid.Width, s.grid.Height, productDir)
			s.clearProduct(p.ID)
			continue
		}

		times := s.scanProduct(p.ID)
		s.mu.Lock()
		s.meta[p.ID] = meta
		set := make(map[int64]bool, len(times))
		for _, t := range times {
			set[t] = true
		}
		s.index[p.ID] = set
		s.mu.Unlock()
		if len(times) > 0 {
			log.Printf("Tutka: %s archive has %d frames on disk", p.ID, len(times))
		}
	}
}

// scanProduct walks one product's day directories and returns the unix seconds
// of every frame it finds.
func (s *Store) scanProduct(product string) []int64 {
	var out []int64
	root := filepath.Join(s.dir, product)
	days, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, day := range days {
		if !day.IsDir() {
			continue
		}
		dayTime, err := time.ParseInLocation(dayLayout, day.Name(), time.UTC)
		if err != nil {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, day.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			name := f.Name()
			if filepath.Ext(name) != ".png" {
				continue
			}
			hm, err := time.ParseInLocation(minuteLayout, name[:len(name)-4], time.UTC)
			if err != nil {
				continue
			}
			ts := dayTime.Add(time.Duration(hm.Hour())*time.Hour + time.Duration(hm.Minute())*time.Minute)
			out = append(out, ts.Unix())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Store) clearProduct(product string) {
	if s.dir == "" {
		return
	}
	if err := os.RemoveAll(filepath.Join(s.dir, product)); err != nil {
		log.Printf("Tutka: failed to clear %s archive: %v", product, err)
	}
	s.mu.Lock()
	delete(s.index, product)
	delete(s.meta, product)
	s.mu.Unlock()
}

func (s *Store) framePath(product string, ts time.Time) string {
	ts = ts.UTC()
	return filepath.Join(s.dir, product, ts.Format(dayLayout), ts.Format(minuteLayout)+".png")
}

func (s *Store) readMeta(product string) (productMeta, error) {
	var m productMeta
	body, err := os.ReadFile(filepath.Join(s.dir, product, metaFileName))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return m, err
	}
	if m.Bits != 8 && m.Bits != 16 {
		return m, fmt.Errorf("meta has bits=%d", m.Bits)
	}
	return m, nil
}

func (s *Store) writeMeta(product string, m productMeta) error {
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(s.dir, product)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, metaFileName), body, 0o644)
}

// Has reports whether a frame is already archived, so the poller can skip
// fetching it. This is what keeps upstream traffic proportional to *new* data
// rather than to how often we poll.
func (s *Store) Has(product string, ts time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.index[product][ts.UTC().Unix()]
}

// Times returns the archived timestamps of a product in ascending order,
// optionally bounded. A zero from/to means unbounded on that side.
func (s *Store) Times(product string, from, to time.Time) []time.Time {
	s.mu.RLock()
	set := s.index[product]
	out := make([]int64, 0, len(set))
	for ts := range set {
		out = append(out, ts)
	}
	s.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	times := make([]time.Time, 0, len(out))
	for _, unix := range out {
		t := time.Unix(unix, 0).UTC()
		if !from.IsZero() && t.Before(from) {
			continue
		}
		if !to.IsZero() && t.After(to) {
			continue
		}
		times = append(times, t)
	}
	return times
}

// Latest returns the most recent archived timestamp of a product.
func (s *Store) Latest(product string) (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var newest int64
	for ts := range s.index[product] {
		if ts > newest {
			newest = ts
		}
	}
	if newest == 0 {
		return time.Time{}, false
	}
	return time.Unix(newest, 0).UTC(), true
}

// Count returns how many frames of a product are archived.
func (s *Store) Count(product string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.index[product])
}

// Put archives a frame and caches it decoded. Writes go through a temporary file
// and a rename so a crash mid-write cannot leave a truncated PNG in the archive
// that later reads would trip over.
func (s *Store) Put(f *Frame) error {
	s.decoded.put(frameKey(f.Product, f.Time), f)

	if s.dir == "" {
		return nil
	}

	path := s.framePath(f.Product, f.Time)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	body, err := encodeFrame(f)
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}

	s.mu.Lock()
	if s.index[f.Product] == nil {
		s.index[f.Product] = make(map[int64]bool)
	}
	s.index[f.Product][f.Time.UTC().Unix()] = true
	known, seen := s.meta[f.Product]
	s.mu.Unlock()

	next := productMeta{Grid: f.Grid, Bits: f.Bits, Gain: f.Gain, Offset: f.Offset}
	if !seen {
		if err := s.writeMeta(f.Product, next); err != nil {
			log.Printf("Tutka: failed to write %s meta: %v", f.Product, err)
		}
		s.mu.Lock()
		s.meta[f.Product] = next
		s.mu.Unlock()
		return nil
	}
	// FMI's linear transformation is a constant of each product, so a change is
	// either an upstream migration or a parsing bug. Either way it silently
	// rescales every value, so say so loudly rather than blending old and new.
	if known.Gain != f.Gain || known.Offset != f.Offset || known.Bits != f.Bits {
		log.Printf("Tutka: WARNING %s scaling changed (bits %d->%d, gain %g->%g, offset %g->%g); frames archived before now use the old scaling",
			f.Product, known.Bits, f.Bits, known.Gain, f.Gain, known.Offset, f.Offset)
		if err := s.writeMeta(f.Product, next); err != nil {
			log.Printf("Tutka: failed to update %s meta: %v", f.Product, err)
		}
		s.mu.Lock()
		s.meta[f.Product] = next
		s.mu.Unlock()
	}
	return nil
}

// Get returns a decoded frame, from the in-memory cache when possible and from
// disk otherwise.
func (s *Store) Get(product string, ts time.Time) (*Frame, error) {
	key := frameKey(product, ts)
	if f, ok := s.decoded.get(key); ok {
		return f, nil
	}
	if s.dir == "" {
		return nil, fs.ErrNotExist
	}

	s.mu.RLock()
	meta, ok := s.meta[product]
	s.mu.RUnlock()
	if !ok {
		return nil, fs.ErrNotExist
	}

	body, err := os.ReadFile(s.framePath(product, ts))
	if err != nil {
		return nil, err
	}
	f, err := decodeFrame(body, product, ts.UTC(), meta)
	if err != nil {
		return nil, err
	}
	s.decoded.put(key, f)
	return f, nil
}

// Prune deletes frames older than the cutoff and removes day directories left
// empty behind them. It returns how many frames went.
func (s *Store) Prune(product string, cutoff time.Time) int {
	if s.dir == "" {
		return 0
	}
	removed := 0
	for _, t := range s.Times(product, time.Time{}, cutoff.Add(-time.Second)) {
		if err := os.Remove(s.framePath(product, t)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("Tutka: failed to prune %s %s: %v", product, t.Format(time.RFC3339), err)
			continue
		}
		s.mu.Lock()
		delete(s.index[product], t.Unix())
		s.mu.Unlock()
		s.decoded.remove(frameKey(product, t))
		removed++
	}

	// Sweep now-empty day directories. os.Remove on a non-empty directory fails,
	// which is exactly the check we want.
	root := filepath.Join(s.dir, product)
	if days, err := os.ReadDir(root); err == nil {
		for _, day := range days {
			if day.IsDir() {
				os.Remove(filepath.Join(root, day.Name()))
			}
		}
	}
	return removed
}

// DiskBytes totals the archive on disk, so /api/health can show it growing
// before it becomes a problem.
func (s *Store) DiskBytes() int64 {
	if s.dir == "" {
		return 0
	}
	var total int64
	filepath.WalkDir(s.dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry should not abort the walk
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// MeanFrameBytes returns the average size of a product's archived frames, so the
// disk projection can be based on what this product actually costs at this grid
// rather than on a guessed compression ratio. It reports false when the product
// has nothing on disk to measure.
func (s *Store) MeanFrameBytes(product string) (float64, bool) {
	if s.dir == "" {
		return 0, false
	}
	var total int64
	var count int
	root := filepath.Join(s.dir, product)
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".png" {
			return nil //nolint:nilerr // an unreadable entry should not abort the walk
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
			count++
		}
		return nil
	})
	if count == 0 {
		return 0, false
	}
	return float64(total) / float64(count), true
}

// encodeFrame writes a frame as a single-channel PNG at its native depth. PNG is
// both the archive container and, after colouring, the wire format: it is
// lossless, its filters beat plain gzip on a smooth radar field, and the encoder
// is in the standard library. The no-coverage sentinel needs no special handling
// because it is simply the maximum value for the depth.
func encodeFrame(f *Frame) ([]byte, error) {
	w, h := f.Grid.Width, f.Grid.Height
	rect := image.Rect(0, 0, w, h)

	var src image.Image
	switch f.Bits {
	case 8:
		g := image.NewGray(rect)
		for i, v := range f.Values {
			g.Pix[i] = uint8(v)
		}
		src = g
	case 16:
		g := image.NewGray16(rect)
		for i, v := range f.Values {
			g.Pix[i*2] = uint8(v >> 8)
			g.Pix[i*2+1] = uint8(v)
		}
		src = g
	default:
		return nil, fmt.Errorf("unsupported depth %d bits", f.Bits)
	}

	// Written once, read for days: spending the extra compression time is worth
	// the bytes saved on a nearly-full disk.
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	var buf bytes.Buffer
	if err := enc.Encode(&buf, src); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeFrame reads a frame back off disk, applying the product's recorded
// scaling.
func decodeFrame(body []byte, product string, ts time.Time, meta productMeta) (*Frame, error) {
	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	if b.Dx() != meta.Grid.Width || b.Dy() != meta.Grid.Height {
		return nil, fmt.Errorf("archived frame is %dx%d, meta says %dx%d", b.Dx(), b.Dy(), meta.Grid.Width, meta.Grid.Height)
	}

	values := make([]uint16, meta.Grid.Width*meta.Grid.Height)
	switch src := img.(type) {
	case *image.Gray:
		for i := range values {
			values[i] = uint16(src.Pix[i])
		}
	case *image.Gray16:
		for i := range values {
			values[i] = uint16(src.Pix[i*2])<<8 | uint16(src.Pix[i*2+1])
		}
	default:
		return nil, fmt.Errorf("archived frame has unexpected type %T", img)
	}

	return &Frame{
		Product: product,
		Time:    ts,
		Grid:    meta.Grid,
		Bits:    meta.Bits,
		NoData:  uint16(1<<meta.Bits - 1),
		Gain:    meta.Gain,
		Offset:  meta.Offset,
		Values:  values,
	}, nil
}

func frameKey(product string, ts time.Time) string {
	return product + "/" + ts.UTC().Format(time.RFC3339)
}

// sameGrid compares grid geometry. Only the fields that affect where a pixel
// lands matter; float equality is right here because both sides derive from the
// same arithmetic on the same configured numbers.
func sameGrid(a, b Grid) bool {
	return a.Width == b.Width && a.Height == b.Height &&
		a.MinX == b.MinX && a.MinY == b.MinY &&
		a.MaxX == b.MaxX && a.MaxY == b.MaxY
}

// publishIndex mirrors the frame index into the shared cache. It is the one
// thing here that belongs in Redis: small, hot, and useful to have survive a
// process restart before the disk scan finishes.
func (s *Store) publishIndex(ctx context.Context, product string) {
	times := s.Times(product, time.Time{}, time.Time{})
	unix := make([]int64, len(times))
	for i, t := range times {
		unix[i] = t.Unix()
	}
	body, err := json.Marshal(unix)
	if err != nil {
		return
	}
	if err := s.cache.SetValue(ctx, "fmi:tutka:index:"+product, body, 30*time.Minute); err != nil {
		log.Printf("Tutka: cache write for %s index failed: %v (memory fallback holds)", product, err)
	}
}
