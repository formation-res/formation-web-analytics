package events

import (
	"strings"
	"testing"

	"github.com/formation-res/formation-web-analytics/internal/config"
)

func TestValidateRejectsOversizedPayloadString(t *testing.T) {
	cfg := config.Config{
		MaxFieldLength:    32,
		MaxPayloadEntries: 16,
		MaxPayloadDepth:   4,
	}
	event := Event{
		Type:   "chat_answer",
		SiteID: "site",
		Payload: map[string]any{
			"rendered_html": strings.Repeat("x", 33),
		},
	}

	event.ApplyFieldLimits(cfg)

	if got := len(event.Payload["rendered_html"].(string)); got != 32 {
		t.Fatalf("expected payload to be truncated to 32, got %d", got)
	}
	if err := event.Validate(cfg); err != nil {
		t.Fatalf("expected truncated payload to validate, got %v", err)
	}
}
