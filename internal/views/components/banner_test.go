package components

import (
	"strings"
	"testing"
)

func TestBanner_RendersAllParts(t *testing.T) {
	html := renderToString(t, Banner(BannerProps{
		ID:          "ios-banner",
		Title:       "Hylete is on iPhone",
		Description: "Get in touch for iOS access.",
		LinkHref:    "/feedback",
		LinkLabel:   "Get in touch",
		IconName:    "smartphone",
	}))

	for _, want := range []string{
		`id="ios-banner"`,
		"Hylete is on iPhone",
		"Get in touch for iOS access.",
		`href="/feedback"`,
		"lucide-smartphone",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected banner HTML to contain %q", want)
		}
	}
}

func TestBanner_OmitsOptionalParts(t *testing.T) {
	html := renderToString(t, Banner(BannerProps{Title: "Only a title"}))

	if strings.Contains(html, "<a ") {
		t.Error("expected no CTA anchor when LinkHref is empty")
	}
	if strings.Contains(html, "<svg") {
		t.Error("expected no icon when IconName is empty")
	}
	if !strings.Contains(html, "Only a title") {
		t.Error("expected title in output")
	}
}

func TestBanner_EscapesLinkHref(t *testing.T) {
	// templ.URL sanitizes javascript: and other unsafe schemes;
	// the sanitized value must not survive into the markup.
	html := renderToString(t, Banner(BannerProps{
		Title:    "T",
		LinkHref: "javascript:alert(1)",
	}))
	if strings.Contains(html, "javascript:") {
		t.Errorf("expected unsafe link scheme to be stripped, got: %s", html)
	}
}
