package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	HTTPTimeout = 5 * time.Second
)

type VolumioClient struct {
	baseURL string
	http    *http.Client
}

func NewVolumioClient(base string) (*VolumioClient, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	if u.Host == "" {
		return nil, errors.New("URL must include a host")
	}
	return &VolumioClient{
		baseURL: u.String(),
		http: &http.Client{
			Timeout: HTTPTimeout,
		},
	}, nil
}

func (c *VolumioClient) cmd(ctx context.Context, command string) error {
	reqURL := fmt.Sprintf("%s/api/v1/commands/?cmd=%s", strings.TrimRight(c.baseURL, "/"), url.QueryEscape(command))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Volumio may respond 200 or 204 for commands; treat 2xx as success.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("command %q failed: status %d", command, resp.StatusCode)
	}
	return nil
}

func (c *VolumioClient) Play(ctx context.Context) error   { return c.cmd(ctx, "play") }
func (c *VolumioClient) Pause(ctx context.Context) error  { return c.cmd(ctx, "pause") }
func (c *VolumioClient) Stop(ctx context.Context) error   { return c.cmd(ctx, "stop") }
func (c *VolumioClient) Toggle(ctx context.Context) error { return c.cmd(ctx, "toggle") }

// SetVolume sets the absolute volume (0..100).
func (c *VolumioClient) SetVolume(ctx context.Context, vol int) error {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	// Build the query properly so &volume is not escaped into the cmd value.
	reqURL := fmt.Sprintf("%s/api/v1/commands/?cmd=volume&volume=%d", strings.TrimRight(c.baseURL, "/"), vol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("set volume failed: status %d", resp.StatusCode)
	}
	return nil
}

func (c *VolumioClient) GetState(ctx context.Context) (State, error) {
	var s State
	reqURL := strings.TrimRight(c.baseURL, "/") + "/api/v1/getState"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return s, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return s, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s, fmt.Errorf("getState failed: status %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&s); err != nil {
		return s, err
	}
	return s, nil
}

func (c *VolumioClient) ProbeHost() error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	ctx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel1()
	conn, err := d.DialContext(ctx1, "tcp", host)
	if err != nil {
		// Try common Volumio port if user omitted it
		host3000 := u.Hostname() + ":3000"
		ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel2()
		if c2, err2 := d.DialContext(ctx2, "tcp", host3000); err2 == nil {
			_ = c2.Close()
			return nil
		}
		return err
	}
	_ = conn.Close()
	return nil
}
