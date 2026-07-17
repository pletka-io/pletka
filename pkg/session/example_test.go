package session_test

import (
	"fmt"
	"net/http"

	"github.com/pletka-io/pletka/pkg/i18n"
	"github.com/pletka-io/pletka/pkg/session"
)

// Example demonstrates how to use session management in an application.
func Example() {
	// 1. Create session manager with configuration
	cfg := session.DefaultConfig()
	cfg.DefaultLanguage = "en"
	cfg.DefaultPageSize = 20
	cfg.DebugMode = true

	sessionManager := session.New(cfg)

	// 2. Optionally set up pgx store (otherwise uses memory store)
	// sessionManager.SetupPgxStore(pool)

	// 3. Create your application's template data struct embedding BaseContext
	type PageData struct {
		*session.BaseContext
		Title   string
		Content string
		Items   []string
	}

	// 4. Create handler that uses session
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Create base context from request
		languages := []i18n.Language{
			{Code: "en", Name: "English"},
			{Code: "nl", Name: "Nederlands"},
		}

		baseCtx := session.NewBaseContext(r, sessionManager, languages)

		// Create page-specific data
		data := &PageData{
			BaseContext: baseCtx,
			Title:       "My Page",
			Content:     "Welcome to my page",
			Items:       []string{"Item 1", "Item 2", "Item 3"},
		}

		// Use timer for performance tracking
		defer data.StartTimer()()

		// Access session data
		fmt.Printf("Current language: %s\n", data.Language)
		fmt.Printf("Page size: %d\n", data.PageSize)
		fmt.Printf("Authenticated: %v\n", data.IsAuthenticated)

		// Render template (example)
		// tmpl.Execute(w, data)
	}

	// 5. Set up middleware chain
	mux := http.NewServeMux()

	// Apply middleware in order:
	// 1. Load/Save session
	// 2. CSRF protection
	// 3. Parameter sync
	// 4. Your handlers

	sessionMiddleware := sessionManager.LoadAndSave
	csrfMiddleware := sessionManager.CSRFProtect()
	paramSyncMiddleware := sessionManager.ParamSync("lang", "theme", "page_size")

	// Chain middleware
	finalHandler := sessionMiddleware(
		csrfMiddleware(
			paramSyncMiddleware(
				http.HandlerFunc(handler),
			),
		),
	)

	mux.Handle("/", finalHandler)

	// 6. Add sessionManager.TemplateFuncs() and session.ContextTemplateFuncs()
	// to your template renderer if it accepts Go template functions.
}

// Example template usage:
const exampleTemplate = `
<!DOCTYPE html>
<html lang="{{ .Language }}">
<head>
    <title>{{ .Title }}</title>
</head>
<body>
    {{/* Flash messages */}}
    {{ if .HasFlash }}
        {{ if .Flash }}<div class="alert success">{{ .Flash }}</div>{{ end }}
        {{ if .FlashError }}<div class="alert error">{{ .FlashError }}</div>{{ end }}
    {{ end }}

    {{/* Language switcher */}}
    <select onchange="location.href=this.value">
        {{ range .Languages }}
            <option value="{{ ctxLangURL $.BaseContext .Code }}" 
                    {{ if eq .Code $.Language }}selected{{ end }}>
                {{ .Name }}
            </option>
        {{ end }}
    </select>

    {{/* Theme switcher */}}
    <a href="{{ .ThemeChangeURL "dark" }}">Dark Mode</a>
    <a href="{{ .ThemeChangeURL "light" }}">Light Mode</a>

    {{/* Pagination with page size */}}
    <select onchange="location.href=this.value">
        {{ range list 10 20 50 100 }}
            <option value="{{ ctxPageSizeURL $.BaseContext . }}"
                    {{ if eq . $.PageSize }}selected{{ end }}>
                {{ . }} per page
            </option>
        {{ end }}
    </select>

    {{/* Sortable table headers */}}
    <table>
        <tr>
            <th><a href="{{ .SortURL "name" }}">Name</a></th>
            <th><a href="{{ .SortURL "date" }}">Date</a></th>
        </tr>
    </table>

    {{/* Using session functions directly */}}
    {{ $customPref := sessionString .Request "custom_preference" "default_value" }}
    <p>Your preference: {{ $customPref }}</p>

    {{/* Checking authentication */}}
    {{ if .IsAuthenticated }}
        <p>Welcome, {{ .UserEmail }}!</p>
    {{ else }}
        <a href="/login">Login</a>
    {{ end }}

    {{/* CSRF token for forms */}}
    <form method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        <button type="submit">Submit</button>
    </form>
</body>
</html>
`
