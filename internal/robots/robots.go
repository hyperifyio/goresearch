package robots

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hyperifyio/goresearch/internal/cache"
)

type Source int

const (
	SourceNetwork Source = iota
	SourceMemory
	SourceCache304
)

type Rules struct {
	Groups []Group
}

type Group struct {
	Agents     []string
	Allow      []string
	Disallow   []string
	CrawlDelay *time.Duration
}

type Manager struct {
	HTTPClient        *http.Client
	Cache             *cache.HTTPCache
	UserAgent         string
	EntryExpiry       time.Duration
	AllowPrivateHosts bool

	mu  sync.Mutex
	mem map[string]memEntry
	now func() time.Time
}

type memEntry struct {
	rules  Rules
	expiry time.Time
}

func (m *Manager) Get(ctx context.Context, robotsURL string) (Rules, Source, error) {
	if m.now == nil {
		m.now = time.Now
	}
	if m.mem == nil {
		m.mem = make(map[string]memEntry)
	}
	u, err := url.Parse(robotsURL)
	if err != nil {
		return Rules{}, SourceNetwork, fmt.Errorf("parse url: %w", err)
	}
	if u == nil || !isHTTPScheme(u) {
		return Rules{}, SourceNetwork, fmt.Errorf("unsupported url scheme: %q", robotsURL)
	}
	host := u.Hostname()
	if !m.AllowPrivateHosts && isLocalOrPrivateHost(host) {
		return Rules{}, SourceNetwork, fmt.Errorf("private host not allowed: %s", host)
	}

	m.mu.Lock()
	if ent, ok := m.mem[robotsURL]; ok && m.now().Before(ent.expiry) {
		r := ent.rules
		m.mu.Unlock()
		return r, SourceMemory, nil
	}
	m.mu.Unlock()

	var etag, lastMod string
	if m.Cache != nil {
		if meta, err := m.Cache.LoadMeta(ctx, robotsURL); err == nil && meta != nil {
			etag = meta.ETag
			lastMod = meta.LastModified
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return Rules{}, SourceNetwork, fmt.Errorf("new request: %w", err)
	}
	if m.UserAgent != "" {
		req.Header.Set("User-Agent", m.UserAgent)
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastMod != "" {
		req.Header.Set("If-Modified-Since", lastMod)
	}
	client := m.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Rules{}, SourceNetwork, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified && m.Cache != nil {
		body, err := m.Cache.LoadBody(ctx, robotsURL)
		if err != nil {
			return Rules{}, SourceCache304, fmt.Errorf("load cached robots: %w", err)
		}
		rules := parseRobots(string(body))
		m.storeMem(robotsURL, rules)
		return rules, SourceCache304, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Rules{}, SourceNetwork, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Rules{}, SourceNetwork, fmt.Errorf("read robots: %w", err)
	}
	if m.Cache != nil {
		_ = m.Cache.Save(ctx, robotsURL, "text/plain", resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), data)
	}
	rules := parseRobots(string(data))
	m.storeMem(robotsURL, rules)
	return rules, SourceNetwork, nil
}

func (m *Manager) storeMem(key string, rules Rules) {
	exp := m.EntryExpiry
	if exp <= 0 {
		exp = 30 * time.Minute
	}
	m.mu.Lock()
	m.mem[key] = memEntry{rules: rules, expiry: m.now().Add(exp)}
	m.mu.Unlock()
}

func parseRobots(text string) Rules {
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var groups []Group
	current := Group{}
	flush := func() {
		if len(current.Agents) == 0 && len(current.Allow) == 0 && len(current.Disallow) == 0 && current.CrawlDelay == nil {
			return
		}
		groups = append(groups, current)
		current = Group{}
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		switch key {
		case "user-agent", "useragent":
			if len(current.Agents) > 0 && (len(current.Allow) > 0 || len(current.Disallow) > 0 || current.CrawlDelay != nil) {
				flush()
			}
			current.Agents = append(current.Agents, strings.ToLower(val))
		case "allow":
			current.Allow = append(current.Allow, val)
		case "disallow":
			current.Disallow = append(current.Disallow, val)
		case "crawl-delay", "crawldelay":
			if s := strings.TrimSpace(val); s != "" {
				if d, err := time.ParseDuration(s + "s"); err == nil {
					dd := d
					current.CrawlDelay = &dd
				}
			}
		}
	}
	flush()
	return Rules{Groups: groups}
}

func isHTTPScheme(u *url.URL) bool {
	if u == nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}

func isLocalOrPrivateHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" || h == "localhost.localdomain" || h == "::1" || h == "[::1]" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return true
		}
	}
	return false
}
