// Package camera talks to the GoPro's HTTP control API over the USB link.
package camera

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"time"
)

// maxBody bounds how much of a response gw reads; replies are small JSON.
const maxBody = 64 << 10

// Client sends requests to one camera. It uses its own transport: no proxy
// from the environment, no redirects, a source address on the GoPro link and
// a timeout on every request.
type Client struct {
	base string
	http *http.Client
}

// NewClient returns a client for the camera at cam, dialing from local, the
// host's address on the same link.
func NewClient(cam netip.AddrPort, local netip.Addr, timeout time.Duration) *Client {
	dialer := &net.Dialer{
		Timeout:   timeout,
		LocalAddr: &net.TCPAddr{IP: local.AsSlice()},
	}
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         dialer.DialContext,
		DisableCompression:  true,
		MaxIdleConns:        1,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: timeout,
	}
	return &Client{
		base: "http://" + cam.String(),
		http: &http.Client{
			Transport: tr,
			Timeout:   timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// StatusError is a non-2xx HTTP reply.
type StatusError struct {
	Path string
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d: %s", e.Path, e.Code, e.Body)
}

// get sends GET path?query and returns the body of a 2xx reply. query is
// built by the caller from validated values, in the order the camera
// expects; url.Values would sort the keys.
func (c *Client) get(ctx context.Context, path, query string) ([]byte, error) {
	u := c.base + path
	if query != "" {
		u += "?" + query
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("GET %s: read body: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &StatusError{Path: path, Code: resp.StatusCode, Body: string(body)}
	}
	return body, nil
}

// IsStatus reports whether err is an HTTP reply with the given status code.
func IsStatus(err error, code int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == code
}
