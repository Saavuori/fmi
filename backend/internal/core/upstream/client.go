package upstream

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"fmi/internal/core/config"
)

// Client is a thin GET client for an FMI open-data host. It sets a descriptive
// User-Agent per FMI's etiquette and returns raw bodies, because the two things
// we fetch are XML (opendata.fmi.fi WFS) and binary rasters (openwms.fmi.fi
// GetMap) rather than JSON.
//
// FMI spans two hosts, so a mode may either construct a Client against one base
// URL and call Get with a path, or call GetURL with a fully-qualified URL.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient builds a client for baseURL. The timeout is generous because a
// full-grid GetMap is a megabyte of raster that GeoServer reprojects on demand.
func NewClient(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		baseURL:    baseURL,
	}
}

// Get fetches baseURL+path and returns the raw response body.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	return c.GetURL(ctx, c.baseURL+path)
}

// GetURL fetches a fully-qualified URL and returns the raw response body.
//
// Note on compression: we deliberately never set Accept-Encoding ourselves.
// openwms.fmi.fi gzips GetMap responses regardless of what was asked for (the
// same trap tie.digitraffic.fi sets in the Fintraffic sibling), and Go's
// transport only decompresses transparently for the header *it* added. Leaving
// it alone keeps that automatic path; the explicit unwrap below is belt and
// braces for the case where a proxy passes gzip through unrequested.
func (c *Client) GetURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", config.FMIUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fmi GET %s: status %d", url, resp.StatusCode)
	}

	body := io.Reader(resp.Body)
	if resp.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("fmi GET %s: gzip: %w", url, err)
		}
		defer zr.Close()
		body = zr
	}

	return io.ReadAll(body)
}
