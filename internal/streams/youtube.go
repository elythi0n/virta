package streams

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/elythi0n/virta/internal/platform"
)

// --- YouTube (anonymous /live page scrape, same source the chat adapter resolves from) ---

const defaultYouTubeBase = "https://www.youtube.com"

// ytMaxPageBytes bounds the /live HTML read; the markers, title, and view count all sit in the
// embedded JSON well within this.
const ytMaxPageBytes = 4 << 20

var (
	ytVideoID = regexp.MustCompile(`"videoId":"([A-Za-z0-9_-]{11})"`)
	ytTitle   = regexp.MustCompile(`<meta\s+(?:name="title"|property="og:title")\s+content="([^"]*)"`)
	// The live view count appears as videoViewCountRenderer runs ("1,234 watching now"); the
	// shape shifts between runs/simpleText, so the count is best-effort and omitted on a miss.
	ytViewers = regexp.MustCompile(`"videoViewCountRenderer":\{"viewCount":\{(?:"runs":\[\{"text":"([^"]+)"|"simpleText":"([^"]+)")`)
)

type youtubeProvider struct {
	c    *http.Client
	base string
}

// NewYouTube builds the YouTube stream-info provider. Pass a nil client for the default and ""
// for youtube.com (override both in tests). It scrapes the channel's /@slug/live page — the
// same anonymous source the chat adapter resolves broadcasts from.
func NewYouTube(c *http.Client, base string) Provider {
	if base == "" {
		base = defaultYouTubeBase
	}
	return &youtubeProvider{c: httpClient(c), base: base}
}

func (p *youtubeProvider) Name() string { return "youtube" }

func (p *youtubeProvider) Fetch(ctx context.Context, ch platform.ChannelRef) (Info, bool, error) {
	if ch.Platform != platform.YouTube || ch.Slug == "" {
		return Info{}, false, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+"/@"+url.PathEscape(ch.Slug)+"/live", nil)
	if err != nil {
		return Info{}, true, err
	}
	req.Header.Set("User-Agent", chromeUA)
	req.Header.Set("Accept-Language", "en")
	resp, err := p.c.Do(req)
	if err != nil {
		return Info{}, true, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return Info{Live: false}, true, nil // unknown channel ≈ not live, not a provider failure
	}
	if resp.StatusCode != http.StatusOK {
		return Info{}, true, fmt.Errorf("streams: youtube GET %s: %d", ch.Slug, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, ytMaxPageBytes))
	if err != nil {
		return Info{}, true, err
	}
	page := string(body)
	if !strings.Contains(page, `{"isLive":true}`) && !strings.Contains(page, `"isLiveNow":true`) {
		return Info{Live: false}, true, nil
	}
	m := ytVideoID.FindStringSubmatch(page)
	if m == nil {
		return Info{Live: false}, true, nil
	}
	info := Info{
		Live:         true,
		ThumbnailURL: "https://i.ytimg.com/vi/" + m[1] + "/hqdefault_live.jpg",
	}
	if t := ytTitle.FindStringSubmatch(page); t != nil {
		info.Title = html.UnescapeString(t[1])
	}
	if v := ytViewers.FindStringSubmatch(page); v != nil {
		raw := v[1]
		if raw == "" {
			raw = v[2]
		}
		if n, err := strconv.Atoi(digitsOnly(raw)); err == nil && n > 0 {
			info.ViewerCount = n
		}
	}
	return info, true, nil
}

// digitsOnly strips grouping separators from a localized count ("1,234" → "1234").
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
