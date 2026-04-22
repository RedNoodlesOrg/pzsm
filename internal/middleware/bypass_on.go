//go:build devbypass

package middleware

// DevBypassEnabled reports whether this build accepts a DEV_USER_EMAIL
// fallback in place of the Cloudflare Access header. True when built with
// -tags devbypass.
const DevBypassEnabled = true
