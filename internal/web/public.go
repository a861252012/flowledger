package web

import (
	"html/template"
	"net/http"
	"path"
	"strings"
)

var publicPage = template.Must(template.ParseFS(assets, "templates/public.html"))

// Explicit observation routes only: a GET wallet route can still expose private
// state or refresh a journal. Account-scoped handlers must never be public.
func publicReadAllowed(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if path.Clean(r.URL.Path) != r.URL.Path {
		return false
	}
	path := r.URL.Path
	if path == "/" || strings.HasPrefix(path, "/static/") {
		return true
	}
	if after, ok := strings.CutPrefix(path, "/net/"); ok {
		parts := strings.SplitN(after, "/", 2)
		if len(parts) != 2 {
			return false
		}
		switch parts[0] {
		case "arbitrum", "base", "optimism", "polygon":
			path = "/" + parts[1]
		default:
			return false
		}
	}
	if path == "/api/network" || path == "/api/balance" || path == "/api/watch/token" {
		return true
	}
	hash, ok := strings.CutPrefix(path, "/api/transactions/")
	return ok && len(hash) == 66 && strings.HasPrefix(hash, "0x") && !strings.Contains(hash, "/")
}
