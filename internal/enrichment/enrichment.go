package enrichment

import (
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/adilbains/uzip/internal/model"
	"github.com/oschwald/geoip2-golang"
)

var botPattern = regexp.MustCompile(`(?i)(bot|crawler|spider|slurp|bingpreview|facebookexternalhit|headless|python-requests|curl/|wget/)`)

type Enricher struct{ geo *geoip2.Reader }

func Open(geoDatabase string) (*Enricher, error) {
	if geoDatabase == "" {
		return &Enricher{}, nil
	}
	reader, err := geoip2.Open(geoDatabase)
	if err != nil {
		return nil, err
	}
	return &Enricher{geo: reader}, nil
}

func (e *Enricher) Close() error {
	if e.geo == nil {
		return nil
	}
	return e.geo.Close()
}

func (e *Enricher) Apply(event *model.ClickEvent) {
	ua := strings.ToLower(event.UserAgent)
	switch {
	case strings.Contains(ua, "mobile"), strings.Contains(ua, "android"), strings.Contains(ua, "iphone"):
		event.Device = "Mobile"
	case strings.Contains(ua, "ipad"), strings.Contains(ua, "tablet"):
		event.Device = "Tablet"
	default:
		event.Device = "Desktop"
	}
	switch {
	case strings.Contains(ua, "edg/"):
		event.Browser = "Edge"
	case strings.Contains(ua, "firefox/"):
		event.Browser = "Firefox"
	case strings.Contains(ua, "chrome/"):
		event.Browser = "Chrome"
	case strings.Contains(ua, "safari/"):
		event.Browser = "Safari"
	default:
		event.Browser = "Other"
	}
	switch {
	case strings.Contains(ua, "windows"):
		event.OS = "Windows"
	case strings.Contains(ua, "android"):
		event.OS = "Android"
	case strings.Contains(ua, "iphone"), strings.Contains(ua, "ipad"):
		event.OS = "iOS"
	case strings.Contains(ua, "mac os"):
		event.OS = "macOS"
	case strings.Contains(ua, "linux"):
		event.OS = "Linux"
	default:
		event.OS = "Other"
	}
	if parsed, err := url.Parse(event.Referrer); err == nil {
		event.Referrer = parsed.Hostname()
	}
	event.IsBot = botPattern.MatchString(event.UserAgent)
	if e.geo != nil {
		if ip := net.ParseIP(event.IPAddress); ip != nil {
			if record, err := e.geo.Country(ip); err == nil {
				event.Country = record.Country.IsoCode
			}
		}
	}
	event.IPAddress = ""
}
