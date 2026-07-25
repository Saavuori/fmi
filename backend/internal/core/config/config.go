package config

import (
	"bufio"
	"flag"
	"log"
	"os"
	"strconv"
	"strings"
)

// FMIUserAgent identifies this app to the Finnish Meteorological Institute's
// open-data services. FMI does not mandate a header the way Digitraffic does,
// but sending a descriptive agent with a contact URL is the same courtesy: it
// lets them tell who is generating load and reach us before throttling it.
const FMIUserAgent = "Saavuori/Tutka (+https://github.com/Saavuori/fmi)"

type Config struct {
	RedisURL string
	Port     string
	NoRedis  bool

	// Radar frame archive. Frames are files under FramesDir, not rows in a
	// database: they are ~100 KB blobs and pruning them must not need scratch
	// space on a nearly-full disk. Recording is disabled when FramesDir is empty.
	FramesDir string

	// The canonical grid every radar frame is fetched, stored, rendered and
	// sampled on, in EPSG:3857. GridResolution is mercator metres per pixel;
	// ground resolution is that times cos(latitude), so 2000 is ~1 km at 60°N.
	GridBBox       [4]float64
	GridResolution float64

	// Per-product retention in hours, keyed by product id, overriding the
	// defaults in the tutka product table. Set via TUTKA_RETENTION_HOURS as a
	// comma-separated "dbz=168,rr=48" list.
	RetentionHours map[string]int
}

// envInt reads an integer env var, falling back to def when unset or unparseable.
func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("Invalid %s=%q, using default %d\n", key, v, def)
	}
	return def
}

// envFloat reads a float env var, falling back to def when unset or unparseable.
func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
		log.Printf("Invalid %s=%q, using default %g\n", key, v, def)
	}
	return def
}

// envBBox parses a "minX,minY,maxX,maxY" env var, falling back to def when
// unset, malformed, or degenerate.
func envBBox(key string, def [4]float64) [4]float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	if len(parts) != 4 {
		log.Printf("Invalid %s=%q (want minX,minY,maxX,maxY), using default %v\n", key, v, def)
		return def
	}
	var out [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			log.Printf("Invalid %s=%q, using default %v\n", key, v, def)
			return def
		}
		out[i] = f
	}
	if out[2] <= out[0] || out[3] <= out[1] {
		log.Printf("Invalid %s=%q (max must exceed min), using default %v\n", key, v, def)
		return def
	}
	return out
}

// envRetention parses a "dbz=168,rr=48" env var into a per-product hour map.
// Unparseable pairs are logged and skipped rather than failing the whole map,
// so one typo cannot wipe out the other overrides.
func envRetention(key string) map[string]int {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	out := make(map[string]int)
	for _, pair := range strings.Split(v, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, hours, ok := strings.Cut(pair, "=")
		if !ok {
			log.Printf("Invalid %s entry %q (want product=hours), skipping\n", key, pair)
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(hours))
		if err != nil || n < 0 {
			log.Printf("Invalid %s entry %q (want product=hours), skipping\n", key, pair)
			continue
		}
		out[strings.TrimSpace(name)] = n
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// loadDotEnv tries to find and parse a .env file from common locations and sets env vars
func loadDotEnv() {
	paths := []string{".env", "../.env", "backend/.env", "../backend/.env"}
	var file *os.File
	for _, path := range paths {
		f, err := os.Open(path)
		if err == nil {
			file = f
			break
		}
	}
	if file == nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
			log.Printf("Set env: %s\n", key)
		}
	}
}

// DefaultGridBBox covers Finland in EPSG:3857 with room for the radar
// composite's sea coverage: Märket (19.1°E) to Virmajärvi (31.6°E) and Bogskär
// (59.5°N) to Nuorgam (70.1°N).
var DefaultGridBBox = [4]float64{2_000_000, 8_250_000, 3_600_000, 11_200_000}

// DefaultGridResolution is mercator metres per pixel: 2000 gives an 800×1475
// grid, ~1.0 km on the ground at 60°N and ~0.68 km at 70°N.
const DefaultGridResolution = 2000

func LoadConfig() *Config {
	loadDotEnv()

	cfg := &Config{
		RedisURL: os.Getenv("REDIS_URL"),
		Port:     os.Getenv("PORT"),
	}

	if cfg.RedisURL == "" {
		cfg.RedisURL = "redis://fmi-cache:6379"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	// Frames live on a mounted volume. Set FRAMES_DIR="" to disable the archive
	// entirely, which leaves the radar mode serving only what it holds in memory.
	cfg.FramesDir = "/data/frames"
	if v, ok := os.LookupEnv("FRAMES_DIR"); ok {
		cfg.FramesDir = v
	}

	cfg.GridBBox = envBBox("GRID_BBOX", DefaultGridBBox)
	cfg.GridResolution = envFloat("GRID_RESOLUTION", DefaultGridResolution)
	if cfg.GridResolution <= 0 {
		log.Printf("Invalid GRID_RESOLUTION %g, using default %g\n", cfg.GridResolution, float64(DefaultGridResolution))
		cfg.GridResolution = DefaultGridResolution
	}
	cfg.RetentionHours = envRetention("TUTKA_RETENTION_HOURS")

	fs := flag.NewFlagSet("fmi", flag.ContinueOnError)
	noRedisFlag := fs.Bool("no-redis", false, "Use in-memory map instead of Redis")

	var args []string
	for _, arg := range os.Args[1:] {
		if len(arg) < 6 || arg[:6] != "-test." {
			args = append(args, arg)
		}
	}
	_ = fs.Parse(args)

	cfg.NoRedis = *noRedisFlag
	if os.Getenv("NO_REDIS") == "true" {
		cfg.NoRedis = true
	}

	return cfg
}
