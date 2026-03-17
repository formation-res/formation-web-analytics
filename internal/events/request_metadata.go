package events

import (
	"slices"
	"strings"
)

func applyAcceptLanguageDetails(event *Event) {
	event.AcceptLanguagePrimaryTag = ""
	event.AcceptLanguagePrimaryBase = ""
	event.AcceptLanguagePrimaryRegion = ""
	event.AcceptLanguageTags = nil

	raw := strings.TrimSpace(event.AcceptLanguage)
	if raw == "" {
		return
	}

	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := normalizeLanguageTag(part)
		if tag == "" {
			continue
		}
		if !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		return
	}

	event.AcceptLanguageTags = tags
	event.AcceptLanguagePrimaryTag = tags[0]

	segments := strings.Split(tags[0], "-")
	if len(segments) > 0 {
		event.AcceptLanguagePrimaryBase = strings.ToLower(segments[0])
	}
	for _, segment := range segments[1:] {
		if len(segment) == 2 || len(segment) == 3 {
			event.AcceptLanguagePrimaryRegion = strings.ToUpper(segment)
			break
		}
	}
}

func normalizeLanguageTag(raw string) string {
	tag := strings.TrimSpace(raw)
	if tag == "" {
		return ""
	}
	if idx := strings.Index(tag, ";"); idx >= 0 {
		tag = tag[:idx]
	}
	tag = strings.TrimSpace(tag)
	if tag == "" || tag == "*" {
		return ""
	}

	segments := strings.Split(tag, "-")
	for i, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return ""
		}
		switch {
		case i == 0:
			segments[i] = strings.ToLower(segment)
		case len(segment) == 2 || len(segment) == 3:
			segments[i] = strings.ToUpper(segment)
		case len(segment) == 4:
			segments[i] = strings.ToUpper(segment[:1]) + strings.ToLower(segment[1:])
		default:
			segments[i] = strings.ToLower(segment)
		}
	}
	return strings.Join(segments, "-")
}

func applyTimezoneDetails(event *Event) {
	event.TimezoneArea = ""
	event.TimezoneLocation = ""

	name := strings.TrimSpace(event.Timezone)
	if name == "" {
		return
	}

	parts := strings.Split(name, "/")
	if len(parts) == 0 {
		return
	}
	event.TimezoneArea = parts[0]
	if len(parts) > 1 {
		event.TimezoneLocation = strings.Join(parts[1:], "/")
	}
}
