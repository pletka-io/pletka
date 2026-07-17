package usermenu

import (
	"strings"

	"github.com/pletka-io/pletka/pkg/auth"
	"github.com/pletka-io/pletka/pkg/i18n"
)

// UserMenu is the role-resolved dropdown payload for one request. nil
// for anonymous callers — the header renders the Login link in that
// case instead of the trigger.
type UserMenu struct {
	DisplayName string         `json:"display_name"`
	Email       string         `json:"email,omitempty"`
	AvatarURL   string         `json:"avatar_url,omitempty"`
	Items       []UserMenuItem `json:"items"`
}

// UserMenuItem is one entry in the dropdown. Type drives rendering:
//
//	"link"    — anchor with optional icon
//	"divider" — horizontal rule, no Label
//	"danger"  — link with red styling (logout, destructive shortcuts)
//
// Labels go through i18n.L so the i18n.Resolve walker enriches them
// at the response edge with bundle lookups for the active language.
type UserMenuItem struct {
	Type     string `json:"type"`
	Label    any    `json:"label,omitempty"`
	Href     string `json:"href,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Match    string `json:"match,omitempty"`    // future: path-prefix → Active highlight
	Active   bool   `json:"active,omitempty"`   // future: server stamps when current path matches
	External bool   `json:"external,omitempty"` // open in new tab
	Badge    *int   `json:"badge,omitempty"`    // future: unread / count pill
}

// Build returns the menu for the given Principal. Returns nil when
// the principal is missing or inactive — caller renders Login instead.
//
// Order: profile → settings → create-organization → (admin if elevated)
// → logout. Stable across role changes; only the admin entry's
// presence varies.
//
// currentPath stamps Active=true on the item whose Match is the
// longest prefix of the request path — exactly one item highlights at
// a time, and nested routes (e.g. /admin/users) still light up their
// parent (/admin). Pass empty string to disable highlighting.
func Build(p *auth.Principal, currentPath string) *UserMenu {
	if p == nil || !p.IsActive {
		return nil
	}

	items := []UserMenuItem{
		{Type: "link", Label: i18n.L("nav.profile", "Profile"), Href: "/profile", Icon: "user", Match: "/profile"},
		{Type: "link", Label: i18n.L("nav.settings", "Settings"), Href: "/profile/settings", Icon: "cog", Match: "/profile/settings"},
		{Type: "link", Label: i18n.L("nav.create_org", "Create organization"), Href: "/orgs/new", Icon: "plus", Match: "/orgs/new"},
	}

	if p.Role == "super_admin" || p.Role == "admin" {
		items = append(items,
			UserMenuItem{Type: "divider"},
			UserMenuItem{Type: "link", Label: i18n.L("nav.admin", "Admin"), Href: "/admin", Icon: "shield", Match: "/admin"},
		)
	}

	items = append(items,
		UserMenuItem{Type: "divider"},
		UserMenuItem{Type: "danger", Label: i18n.L("nav.logout", "Logout"), Href: "/logout", Icon: "logout"},
	)

	stampActive(items, currentPath)

	display := p.DisplayName
	if display == "" {
		display = p.Slug
	}

	return &UserMenu{
		DisplayName: display,
		Email:       p.Email,
		Items:       items,
	}
}

// stampActive picks the item whose Match is the longest prefix of
// currentPath and flips its Active flag. Empty Match values are
// skipped (logout, dividers). Longest-prefix wins so /profile/settings
// activates "Settings" rather than "Profile" — both are valid prefix
// matches but the more specific one is what the curator is on.
func stampActive(items []UserMenuItem, currentPath string) {
	if currentPath == "" {
		return
	}
	bestIdx := -1
	bestLen := 0
	for i := range items {
		m := items[i].Match
		if m == "" {
			continue
		}
		if !strings.HasPrefix(currentPath, m) {
			continue
		}
		if len(m) > bestLen {
			bestLen = len(m)
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		items[bestIdx].Active = true
	}
}
