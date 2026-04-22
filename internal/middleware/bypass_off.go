//go:build !devbypass

package middleware

// DevBypassEnabled reports whether this build accepts a DEV_USER_EMAIL
// fallback in place of the Cloudflare Access header. False for production
// builds; set to true by building with -tags devbypass.
const DevBypassEnabled = false
