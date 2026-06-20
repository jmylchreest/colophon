// Package generate turns text prompts into images for colophon's `gen:` references.
// Provider drivers (Google Gemini, OpenAI-compatible, MiniMax) sit behind one
// ImageGenerator interface; all speak plain HTTP, so no provider SDK is required.
// The package also owns the content-addressed cache naming (CacheKey/FileName) and
// the sidecar metadata written beside each generated image.
package generate

import "context"

// ImageRequest is a single text-to-image generation request.
type ImageRequest struct {
	Prompt string
	System string // house style / instruction; resolved from theme/site/per-ref
	Model  string
	Params map[string]string // normalised tuning params, e.g. {"aspect":"16:9"}
}

// ImageResult is a generated image plus any metadata the provider echoed back.
type ImageResult struct {
	Bytes         []byte
	MIME          string // e.g. "image/png"
	RevisedPrompt string // the prompt the model actually used, when returned
}

// ImageGenerator turns a prompt into image bytes. Implementations are provider
// drivers constructed via New.
type ImageGenerator interface {
	Generate(ctx context.Context, req ImageRequest) (ImageResult, error)
}
