// Package securityheaders provides an HTTP middleware that attaches
// security-hardening response headers to every outgoing response.
//
// Headers applied:
//
//   - X-Content-Type-Options: nosniff
//     Prevents browsers from MIME-sniffing a response away from the declared
//     Content-Type, blocking content injection attacks.
//
//   - X-Frame-Options: DENY
//     Refuses iframe embedding from any origin, preventing clickjacking.
//
//   - X-XSS-Protection: 0
//     Disables the legacy IE/Chrome XSS auditor. Modern browsers rely on CSP
//     instead; the old filter can actually introduce vulnerabilities.
//
//   - Referrer-Policy: strict-origin-when-cross-origin
//     Sends the full URL as Referer on same-origin requests but only the
//     origin on cross-origin requests, and nothing on downgrade (HTTPS→HTTP).
//
//   - Permissions-Policy: camera=(), microphone=(), geolocation=()
//     Explicitly disables access to sensitive browser APIs that the admin
//     dashboard does not require.
//
//   - Strict-Transport-Security (HSTS)
//     Only applied when the TLS flag is set to true. Instructs browsers to
//     connect via HTTPS exclusively for 2 years.
//
// Usage:
//
//	mux := http.NewServeMux()
//	handler := securityheaders.Middleware(securityheaders.Config{TLS: true})(mux)
//	http.ListenAndServeTLS(":443", certFile, keyFile, handler)
package securityheaders

import "net/http"

// Config controls which optional headers are emitted.
type Config struct {
	// TLS enables the Strict-Transport-Security header.
	// Set to true only when the server is reachable via HTTPS.
	TLS bool
}

// Middleware returns a middleware function that wraps the given handler and
// attaches security headers to every response.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()

			// Prevent MIME-type sniffing.
			h.Set("X-Content-Type-Options", "nosniff")

			// Block all iframe embedding (clickjacking protection).
			h.Set("X-Frame-Options", "DENY")

			// Disable the legacy XSS auditor; Content-Security-Policy is the
			// modern replacement and is configured at the reverse-proxy level.
			h.Set("X-XSS-Protection", "0")

			// Send only the origin (not the path) on cross-origin requests.
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Deny camera, microphone, and geolocation to the admin UI.
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

			// HSTS: enforce HTTPS for 2 years, including subdomains.
			// Only safe when the server is actually reachable via TLS.
			if cfg.TLS {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			}

			next.ServeHTTP(w, r)
		})
	}
}
