package events

import (
	"strings"
	"sync"

	"github.com/ua-parser/uap-go/uaparser"
)

var (
	userAgentParserOnce sync.Once
	userAgentParser     *uaparser.Parser
	userAgentParserErr  error
)

func applyUserAgentDetails(event *Event) {
	event.BrowserFamily = ""
	event.BrowserVersion = ""
	event.BrowserMajor = ""
	event.OSFamily = ""
	event.OSVersion = ""
	event.DeviceFamily = ""
	event.DeviceBrand = ""
	event.DeviceModel = ""
	event.DeviceType = ""
	event.BrowserEngine = ""

	raw := strings.TrimSpace(event.UserAgent)
	if raw == "" {
		return
	}

	parser, err := getUserAgentParser()
	if err != nil {
		return
	}

	client := parser.Parse(raw)
	if client == nil {
		return
	}
	if client.UserAgent != nil {
		event.BrowserFamily = strings.TrimSpace(client.UserAgent.Family)
		event.BrowserVersion = strings.TrimSpace(client.UserAgent.ToVersionString())
		event.BrowserMajor = strings.TrimSpace(client.UserAgent.Major)
		event.BrowserEngine = deriveBrowserEngine(event.BrowserFamily, raw)
	}
	if client.Os != nil {
		event.OSFamily = strings.TrimSpace(client.Os.Family)
		event.OSVersion = strings.TrimSpace(client.Os.ToVersionString())
	}
	if client.Device != nil {
		event.DeviceFamily = strings.TrimSpace(client.Device.Family)
		event.DeviceBrand = strings.TrimSpace(client.Device.Brand)
		event.DeviceModel = strings.TrimSpace(client.Device.Model)
		event.DeviceType = deriveDeviceType(event.DeviceFamily, event.OSFamily, event.BrowserFamily, raw)
	}
}

func getUserAgentParser() (*uaparser.Parser, error) {
	userAgentParserOnce.Do(func() {
		userAgentParser, userAgentParserErr = uaparser.New()
	})
	return userAgentParser, userAgentParserErr
}

func deriveBrowserEngine(browserFamily, rawUA string) string {
	family := strings.ToLower(strings.TrimSpace(browserFamily))
	raw := strings.ToLower(rawUA)

	switch {
	case family == "chrome" || family == "chromium" || family == "edge" || family == "opera" || strings.Contains(raw, "edg/") || strings.Contains(raw, "opr/"):
		return "Blink"
	case family == "safari" || strings.Contains(raw, "applewebkit"):
		return "WebKit"
	case family == "firefox" || strings.Contains(raw, "gecko/") || strings.Contains(raw, "firefox/"):
		return "Gecko"
	default:
		return ""
	}
}

func deriveDeviceType(deviceFamily, osFamily, browserFamily, rawUA string) string {
	device := strings.ToLower(strings.TrimSpace(deviceFamily))
	osName := strings.ToLower(strings.TrimSpace(osFamily))
	browser := strings.ToLower(strings.TrimSpace(browserFamily))
	raw := strings.ToLower(rawUA)

	switch {
	case strings.Contains(raw, "tablet") || strings.Contains(raw, "ipad"):
		return "tablet"
	case strings.Contains(raw, "mobile") || strings.Contains(raw, "iphone") || strings.Contains(raw, "android") && !strings.Contains(raw, "tablet"):
		return "mobile"
	case strings.Contains(raw, "smart-tv") || strings.Contains(raw, "smarttv") || strings.Contains(raw, "hbbtv") || strings.Contains(raw, "appletv") || strings.Contains(raw, "tv"):
		return "tv"
	case device == "spider" || browser == "googlebot" || browser == "bingbot" || strings.Contains(raw, "bot"):
		return "bot"
	case device == "mac" || strings.Contains(osName, "windows") || strings.Contains(osName, "mac os x") || strings.Contains(osName, "linux") || strings.Contains(osName, "chrome os") || strings.Contains(raw, "x11"):
		return "desktop"
	default:
		return ""
	}
}
