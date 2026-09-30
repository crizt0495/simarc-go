package handlers

import (
	"html/template"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"arsippro/internal/models"
)

// parseShellTemplates parses every layout + component exactly once, which is
// all the app shell needs (pages supply their own {{define "content"}}).
func parseShellTemplates(t *testing.T) *template.Template {
	t.Helper()
	layoutFiles, _ := filepath.Glob("web/templates/layouts/*.html")
	compFiles, _ := filepath.Glob("web/templates/components/*.html")
	files := append(append([]string{}, layoutFiles...), compFiles...)
	if len(files) == 0 {
		t.Fatal("no layout/component templates found — wrong working directory?")
	}
	ts, err := template.New("").Funcs(TemplateFuncs()).ParseFiles(files...)
	if err != nil {
		t.Fatalf("shell templates failed to parse: %v", err)
	}
	return ts
}

// renderShell renders the app shell (layouts/app.html) in the given layout
// mode. No page content and no database are needed: the {{block "content"}}
// simply renders empty, so this exercises the navigation chrome only.
func renderShell(t *testing.T, ts *template.Template, mode, currentPath string) string {
	t.Helper()
	data := map[string]interface{}{
		"LayoutMode":   mode,
		"CurrentPath":  currentPath,
		"AssetVersion": "test",
		"AppName":      "SIMARC",
		"AppURL":       "http://127.0.0.1:8080",
		"CSRFToken":    "test-token",
		"Year":         2026,
		"title":        "Shell",
		// The user menu is behind {{if .AuthUser}}; supply one so the header's
		// utility cluster is actually rendered and can be asserted on.
		"AuthUser": &models.User{
			Name: "Test User",
			Role: &models.Role{Name: "Admin"},
		},
	}
	var sb strings.Builder
	if err := ts.ExecuteTemplate(&sb, "layouts/app.html", data); err != nil {
		t.Fatalf("rendering app.html in %q mode failed: %v", mode, err)
	}
	return sb.String()
}

var hrefRe = regexp.MustCompile(`href="(/[^"#?]*)"`)

// navLinks returns the sorted, de-duplicated set of internal hrefs in a chunk
// of rendered HTML, ignoring the ones that belong to the shell chrome rather
// than to the navigation (logout forms, the brand link, the search form).
func navLinks(t *testing.T, html string, chrome ...string) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, m := range hrefRe.FindAllStringSubmatch(html, -1) {
		href := m[1]
		if href == "" || href == "/" {
			continue
		}
		skip := false
		for _, c := range chrome {
			if href == c {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		seen[href] = true
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// TestLayoutMode_SidebarRendersSidebarAndNoTopnav is the guard for the
// browser build: nothing about the existing sidebar UI may change.
func TestLayoutMode_SidebarRendersSidebarAndNoTopnav(t *testing.T) {
	ts := parseShellTemplates(t)
	body := renderShell(t, ts, LayoutModeSidebar, "/arsip")

	if !strings.Contains(body, `class="sidebar"`) {
		t.Error("sidebar mode must render the left sidebar")
	}
	if strings.Contains(body, `id="topnav"`) {
		t.Error("sidebar mode must not render the top navigation")
	}
	if !strings.Contains(body, `class="sidebar-overlay"`) {
		t.Error("sidebar mode must render the sidebar overlay")
	}
	if strings.Contains(body, "layout-topbar") {
		t.Error(`sidebar mode must not put "layout-topbar" on <body>`)
	}
}

// TestLayoutMode_TopbarRendersTopnavAndNoSidebar covers the desktop shell.
func TestLayoutMode_TopbarRendersTopnavAndNoSidebar(t *testing.T) {
	ts := parseShellTemplates(t)
	body := renderShell(t, ts, LayoutModeTopbar, "/arsip")

	if !strings.Contains(body, `id="topnav"`) {
		t.Error("topbar mode must render the top navigation")
	}
	if strings.Contains(body, `class="sidebar"`) {
		t.Error("topbar mode must not render the left sidebar")
	}
	if strings.Contains(body, `class="sidebar-overlay"`) {
		t.Error("topbar mode must not render the sidebar overlay")
	}
	if !strings.Contains(body, `<body class="layout-topbar">`) {
		t.Error(`topbar mode must put "layout-topbar" on <body> so the CSS switches shells`)
	}
	// The top bar must keep the utility cluster that used to live above the
	// sidebar: search, theme toggle and the user menu.
	for _, want := range []string{`class="top-navbar"`, `id="themeToggle"`, `class="nav-user"`} {
		if !strings.Contains(body, want) {
			t.Errorf("topbar mode must keep %s in the header", want)
		}
	}
}

// TestLayoutMode_TopnavCoversEverySidebarLink is the important one: the top
// bar is a re-grouping of the sidebar, so no destination may be dropped.
// If someone adds a link to the sidebar and forgets the top navigation (or
// the other way round), this fails.
func TestLayoutMode_TopnavCoversEverySidebarLink(t *testing.T) {
	ts := parseShellTemplates(t)

	// Render the sidebar component on its own, with no chrome to subtract.
	var sb strings.Builder
	data := map[string]interface{}{
		"CurrentPath":  "/dashboard",
		"CSRFToken":    "test-token",
		"AssetVersion": "test",
	}
	if err := ts.ExecuteTemplate(&sb, "components/sidebar.html", data); err != nil {
		t.Fatalf("rendering sidebar.html failed: %v", err)
	}
	sidebar := navLinks(t, sb.String(), "/profil", "/dashboard")

	var tb strings.Builder
	if err := ts.ExecuteTemplate(&tb, "components/topnav.html", data); err != nil {
		t.Fatalf("rendering topnav.html failed: %v", err)
	}
	topnav := navLinks(t, tb.String(), "/dashboard")

	if len(sidebar) == 0 {
		t.Fatal("no links parsed out of the sidebar — the extractor is broken")
	}

	have := map[string]bool{}
	for _, l := range topnav {
		have[l] = true
	}
	for _, l := range sidebar {
		if !have[l] {
			t.Errorf("top navigation is missing a sidebar destination: %s", l)
		}
	}

	haveTop := map[string]bool{}
	for _, l := range sidebar {
		haveTop[l] = true
	}
	for _, l := range topnav {
		if !haveTop[l] {
			t.Errorf("top navigation exposes a destination the sidebar does not: %s", l)
		}
	}

	t.Logf("sidebar and top navigation both expose %d destinations", len(sidebar))
}

// TestSetLayoutMode_RejectsUnknownValues makes sure a bad value can never
// leave the app without a navigation shell.
func TestSetLayoutMode_RejectsUnknownValues(t *testing.T) {
	original := LayoutMode()
	defer SetLayoutMode(original)

	for _, mode := range []string{LayoutModeSidebar, LayoutModeTopbar} {
		SetLayoutMode(mode)
		if got := LayoutMode(); got != mode {
			t.Errorf("SetLayoutMode(%q) -> LayoutMode() = %q", mode, got)
		}
	}

	for _, bad := range []string{"", "TOPBAR", "drawer", "halaman"} {
		SetLayoutMode(bad)
		if got := LayoutMode(); got != LayoutModeSidebar {
			t.Errorf("SetLayoutMode(%q) should fall back to %q, got %q", bad, LayoutModeSidebar, got)
		}
	}
}
