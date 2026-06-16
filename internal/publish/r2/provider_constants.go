package r2

// providerName identifies the object-store backend behind an S3-compatible endpoint. The
// driver speaks plain S3 to every backend; a recognised provider additionally unlocks
// control-plane features such as public-URL discovery and exposing a new bucket.
type providerName string

const (
	providerUnknown providerName = ""              // generic S3 / MinIO: S3 API only
	providerR2      providerName = "cloudflare-r2" // Cloudflare R2
)

// endpointProviders maps an endpoint-host glob to the provider it identifies, in order —
// the first match wins, so keep any catch-all last. To teach colophon a new backend, add a
// row here for its host shape and a matching entry in providerBehaviour (provider.go).
var endpointProviders = []struct {
	hostGlob string
	provider providerName
}{
	// Matches the standard host and the jurisdiction variants (<acct>.eu… / <acct>.fedramp…),
	// since the glob's * spans the extra label.
	{"*.r2.cloudflarestorage.com", providerR2},
	// {"*.amazonaws.com", providerS3},  // e.g. AWS S3, with its own discovery path
}
