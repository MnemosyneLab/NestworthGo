package mobileapi

import (
	"net/http"
	"strings"
)

// Only the desktop-authorized frontend reaches this handler. Browser local
// network permission and certificate trust are still required where applicable.
func (s *Server) preflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Access-Control-Request-Method")
	w.Header().Add("Vary", "Access-Control-Request-Headers")
	method := r.Header.Get("Access-Control-Request-Method")
	if method != "GET" && method != "POST" && method != "DELETE" {
		fail(w, forbidden())
		return
	}
	for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
		switch strings.ToLower(strings.TrimSpace(header)) {
		case "", "authorization", "content-type", "x-nestworth-mobile", "x-csrf-token":
		default:
			fail(w, forbidden())
			return
		}
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Nestworth-Mobile, X-CSRF-Token")
	if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
	}
	w.WriteHeader(http.StatusNoContent)
}
