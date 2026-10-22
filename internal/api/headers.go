package api

import "net/http"

// contentSecurityPolicy allows scripts from this origin only (no inline or
// third-party code) and no framing (clickjacking). Map styles are admin
// settings that may point at any tile host, and their tiles, sprites and
// fonts are only fetched or drawn, never run, so https: is allowed for those.
// MapLibre needs inline styles and blob: workers/images.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https:; connect-src 'self' https:; " +
	"font-src 'self' data:; worker-src 'self' blob:; child-src 'self' blob:; object-src 'none'; base-uri 'self'; " +
	"form-action 'self'; frame-ancestors 'none'; manifest-src 'self'"

// securityHeaders sets browser hardening headers on every response.
func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(self), camera=(), microphone=(), payment=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if meta(r).HTTPS {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}
