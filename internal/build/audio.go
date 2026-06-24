package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/generate"
)

// resolveSpeech resolves the configured speech generator to settings, or nil when speech
// is off or misconfigured (recorded audio still works without it).
func resolveSpeech(cfg *config.Config, log *clog.Logger) *generate.SpeechSettings {
	if !cfg.Generation.Speech.Configured() {
		return nil
	}
	s, err := generate.ResolveSpeech(cfg.Generation.Speech)
	if err != nil {
		log.Step("AUDIO", "", "warn", "speech generation disabled: "+err.Error())
		return nil
	}
	return &s
}

// retryPolicyFor is the default provider rate-limit backoff with a logging hook, so each backoff
// is visible under the given category (AUDIO/IMAGE) rather than looking like a stalled build.
func retryPolicyFor(category string, log *clog.Logger) generate.RetryPolicy {
	p := generate.DefaultRetryPolicy()
	p.OnRetry = func(attempt int, wait time.Duration, err error) {
		log.Step(category, "", "warn", fmt.Sprintf("rate limited, backing off %s (attempt %d): %v", wait.Round(time.Second), attempt, err))
	}
	return p
}

// audioVoiceFor builds the voice resolver: a post's audio_voice wins, else its author's
// voice, else its persona's voice, else "" (the speech default applies).
func audioVoiceFor(cfg *config.Config) func(postVoice, author, persona string) string {
	return func(postVoice, author, persona string) string {
		if v := strings.TrimSpace(postVoice); v != "" {
			return v
		}
		if a := resolveAuthor(cfg, author); strings.TrimSpace(a.Voice) != "" {
			return a.Voice
		}
		if persona != "" {
			for i := range cfg.Personas {
				if cfg.Personas[i].ID == persona && strings.TrimSpace(cfg.Personas[i].Voice) != "" {
					return cfg.Personas[i].Voice
				}
			}
		}
		return ""
	}
}

// audioJob is one post's audio attachment to publish: either a pre-recorded file copied
// from a source, or a TTS clip generated into the cache. Keyed by its output path.
type audioJob struct {
	kind    string // "file" | "tts"
	outPath string // output-relative publish path
	mime    string
	src     core.Source // file: source to read the recording from
	srcPath string      // file: path within that source
	req     generate.SpeechRequest
	cache   string    // tts: absolute cache path
	size    int64     // filled after run()
	peaks   []float64 // waveform amplitude peaks (0–1), when computed
}

// audioResolver maps a post's audio (recorded audio_file or generated audio:true) to a
// stable URL, accumulates the clips to publish, and produces them. Recorded audio works
// whenever a resolver exists; TTS additionally needs speech to be configured.
type audioResolver struct {
	speech         *generate.SpeechSettings
	cacheDir       string
	basePath       string
	baseURL        string
	router         *core.Router
	generateAI     bool
	voiceFor       func(postVoice, author, persona string) string
	log            *clog.Logger
	i18n           ttsTable         // injected-speech translations (block cues, hint, wrap-up, symbols)
	defaultLang    string           // site language, used when a post sets none
	acronyms       *acronymReplacer // glossary acronym → spoken expansion
	defaultAudioOn bool             // per-post audio: default when a post sets none
	jobs           map[string]*audioJob
}

func newAudioResolver(speech *generate.SpeechSettings, root, basePath, baseURL string, router *core.Router, generateAI bool, voiceFor func(string, string, string) string, defaultLang string, acronyms *acronymReplacer, defaultAudioOn bool, log *clog.Logger) *audioResolver {
	ar := &audioResolver{
		speech: speech, basePath: basePath, baseURL: baseURL, router: router,
		generateAI: generateAI, voiceFor: voiceFor, defaultLang: defaultLang,
		acronyms: acronyms, defaultAudioOn: defaultAudioOn, i18n: loadTTSTable(root), log: log, jobs: map[string]*audioJob{},
	}
	if speech != nil {
		dir := speech.OutputDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(root, filepath.FromSlash(dir))
		}
		ar.cacheDir = dir
	}
	return ar
}

func (ar *audioResolver) active() bool { return ar != nil }

// wantsAudio reports whether a post should get a generated reading: an explicit frontmatter
// `audio:` wins; otherwise the site default (speech configured + enabled).
func (ar *audioResolver) wantsAudio(audio *bool) bool {
	if !ar.active() {
		return false
	}
	if audio != nil {
		return *audio
	}
	return ar.defaultAudioOn
}

// uiLabels returns the localised player UI strings (figcaption, play/pause aria-labels) for a
// page language, falling back to the site default then English.
func (ar *audioResolver) uiLabels(lang string) (listen, play, pause string) {
	if strings.TrimSpace(lang) == "" {
		lang = ar.defaultLang
	}
	s := ar.i18n.For(lang)
	return s.Listen, s.Play, s.Pause
}

// urlFor builds the (page, absolute) URLs for an output path, routing to the object store
// when a route binds it, else rooting at base_path.
func (ar *audioResolver) urlFor(outPath string) (rel, abs string) {
	if u := ar.router.AssetURL(outPath); u != "" {
		return u, u
	}
	return ar.basePath + outPath, absURL(ar.baseURL, outPath)
}

// registerFile resolves a pre-recorded audio_file to its URLs and queues it for copy. An
// external (http/data) ref passes through unpublished. ok is false for a resolver that's
// off or an empty ref.
func (ar *audioResolver) registerFile(it included, ref string) (rel, abs, mime, outPath string, ok bool) {
	if !ar.active() || strings.TrimSpace(ref) == "" {
		return "", "", "", "", false
	}
	mime = audioMIMEByExt(ref)
	if !localRef(ref) {
		return ref, ref, mime, "", true // already-hosted recording
	}
	outPath = path.Clean(path.Join(it.slug, ref))
	if _, seen := ar.jobs[outPath]; !seen {
		ar.jobs[outPath] = &audioJob{
			kind: "file", outPath: outPath, mime: mime,
			src:     it.src,
			srcPath: path.Clean(path.Join(path.Dir(it.c.SourcePath), ref)),
		}
	}
	rel, abs = ar.urlFor(outPath)
	return rel, abs, mime, outPath, true
}

// registerTTS resolves a generated reading to its URLs and queues it. ok is false when
// speech generation isn't configured.
func (ar *audioResolver) registerTTS(label, htmlBody, lang, postVoice, author, persona string) (rel, abs, mime, outPath string, ok bool) {
	if !ar.active() || ar.speech == nil {
		return "", "", "", "", false
	}
	if strings.TrimSpace(lang) == "" {
		lang = ar.defaultLang
	}
	// strip/cue code, math, tables, diagrams; spell inline-code symbols — in the post's language
	text := speechText(htmlBody, ar.speech.Transcript, ar.i18n.For(lang), ar.acronyms)
	if strings.TrimSpace(text) == "" {
		return "", "", "", "", false
	}
	s := ar.speech
	voice := s.Voice
	if v := strings.TrimSpace(ar.voiceFor(postVoice, author, persona)); v != "" {
		voice = v
	}
	stem := generate.SpeechStem(s.Provider, s.Model, voice, s.Format, label, text)
	filename := stem + "." + s.Format
	outPath = genOutDir + "/" + filename
	mime = generate.AudioMIME(s.Format)
	if _, seen := ar.jobs[outPath]; !seen {
		ar.jobs[outPath] = &audioJob{
			kind: "tts", outPath: outPath, mime: mime,
			req:   generate.SpeechRequest{Text: text, Voice: voice, Format: s.Format, Model: s.Model},
			cache: filepath.Join(ar.cacheDir, filename),
		}
	}
	rel, abs = ar.urlFor(outPath)
	return rel, abs, mime, outPath, true
}

// size returns the published byte length for an output path (for feed enclosures), 0 if
// the clip wasn't produced.
func (ar *audioResolver) size(outPath string) int64 {
	if ar == nil {
		return 0
	}
	if j := ar.jobs[outPath]; j != nil {
		return j.size
	}
	return 0
}

// run produces and publishes every queued clip: recorded files are copied from their
// source; TTS clips are reused from cache or generated (when generateAI) into cache +
// sidecar. Work runs in a bounded pool; publishing (write) and logging replay serially.
func (ar *audioResolver) run(write func(string, []byte) error, now time.Time) error {
	if !ar.active() || len(ar.jobs) == 0 {
		return nil
	}
	keys := make([]string, 0, len(ar.jobs))
	for k := range ar.jobs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var ttsOnce sync.Once
	var tts generate.SpeechGenerator
	var ttsErr error
	ensureTTS := func() (generate.SpeechGenerator, error) {
		ttsOnce.Do(func() {
			if ar.speech != nil {
				tts, ttsErr = generate.NewSpeech(*ar.speech)
			} else {
				ttsErr = fmt.Errorf("speech generation not configured")
			}
		})
		return tts, ttsErr
	}

	results := make([]audioOutcome, len(keys))
	limit := generate.DefaultConcurrency
	if ar.speech != nil && ar.speech.Concurrency > 0 {
		limit = ar.speech.Concurrency
	}
	sem := make(chan struct{}, max(1, limit))
	var wg sync.WaitGroup
	for i, k := range keys {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j *audioJob) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = ar.produce(j, ensureTTS, now)
		}(i, ar.jobs[k])
	}
	wg.Wait()

	if ttsErr != nil {
		ar.log.Step("AUDIO", "", "warn", "speech generator unavailable: "+ttsErr.Error())
	}
	for i, k := range keys {
		r := results[i]
		if r.fatal != nil {
			return r.fatal
		}
		if r.detail {
			ar.log.Detail("AUDIO", ar.jobs[k].kind, r.logArgs...)
		} else {
			ar.log.Step("AUDIO", ar.jobs[k].kind, r.logArgs...)
		}
		if r.bytes != nil {
			if err := write(k, r.bytes); err != nil {
				return err
			}
			// Publish the waveform peaks beside the audio so the player can fetch <audio>.json;
			// absent → the player falls back to the live Web Audio visualiser.
			if pk := ar.jobs[k].peaks; len(pk) > 0 {
				if pj, err := json.Marshal(map[string]any{"peaks": pk}); err == nil {
					if err := write(k+".json", pj); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// filePeaks resolves waveform peaks for a recorded file without any decoder dependency: an
// author-committed <file>.json sidecar wins, else a WAV is read directly; anything else
// returns nil so the player uses its live visualiser.
func (ar *audioResolver) filePeaks(src core.Source, srcPath string, audio []byte) []float64 {
	if rc, err := src.Open(context.Background(), srcPath+".json"); err == nil {
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if pk := parsePeaks(data); pk != nil {
			return pk
		}
	}
	if strings.HasSuffix(strings.ToLower(srcPath), ".wav") {
		if pk, ok := peaksFromWAV(audio); ok {
			return pk
		}
	}
	return nil
}

// parsePeaks reads a {"peaks":[…]} sidecar (author-provided or our cache metadata).
func parsePeaks(data []byte) []float64 {
	var s struct {
		Peaks []float64 `json:"peaks"`
	}
	if json.Unmarshal(data, &s) != nil || len(s.Peaks) == 0 {
		return nil
	}
	return s.Peaks
}

// audioOutcome is one job's result from the parallel phase, replayed serially for
// deterministic publishing + logging.
type audioOutcome struct {
	bytes   []byte
	logArgs []any
	detail  bool
	fatal   error
}

func (ar *audioResolver) produce(j *audioJob, ensureTTS func() (generate.SpeechGenerator, error), now time.Time) (out audioOutcome) {
	switch j.kind {
	case "file":
		rc, err := j.src.Open(context.Background(), j.srcPath)
		if err != nil {
			out.logArgs = []any{"warn", "missing recording: " + j.srcPath}
			return out
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			out.fatal = err
			return out
		}
		j.size = int64(len(b))
		j.peaks = ar.filePeaks(j.src, j.srcPath, b) // free: author sidecar or WAV; else live fallback
		out.bytes, out.detail, out.logArgs = b, true, []any{"file", j.outPath, "bytes", len(b)}
		return out
	default: // tts
		if b, err := os.ReadFile(j.cache); err == nil {
			j.size = int64(len(b))
			// Read any peaks an older build persisted (read-compat); current builds derive the
			// waveform in the browser, so a cache hit with no peaks just ships no sidecar.
			if meta, err := os.ReadFile(j.cache + ".json"); err == nil {
				j.peaks = parsePeaks(meta)
			}
			out.bytes, out.detail, out.logArgs = b, true, []any{"cached", path.Base(j.outPath), "bytes", len(b)}
			return out
		}
		if !ar.generateAI {
			out.logArgs = []any{"skip", path.Base(j.outPath), "hint", "build --generate-ai to create it"}
			return out
		}
		gen, err := ensureTTS()
		if err != nil {
			out.logArgs = []any{"skip", path.Base(j.outPath)}
			return out
		}
		res, err := gen.Generate(context.Background(), j.req)
		if err != nil {
			out.logArgs = []any{"warn", fmt.Sprintf("generate %q failed: %v", path.Base(j.outPath), err)}
			return out
		}
		// No second render for a waveform: the browser derives peaks from this audio (player.js).
		if err := os.MkdirAll(ar.cacheDir, 0o755); err != nil {
			out.fatal = err
			return out
		}
		if err := os.WriteFile(j.cache, res.Bytes, 0o644); err != nil {
			out.fatal = err
			return out
		}
		_ = writeAudioSidecar(j.cache, j.req, res.MIME, now)
		j.size = int64(len(res.Bytes))
		out.bytes, out.logArgs = res.Bytes, []any{"generated", path.Base(j.outPath), "voice", j.req.Voice, "bytes", len(res.Bytes)}
		return out
	}
}

// writeAudioSidecar records cache metadata next to a generated clip (provenance + the inputs the
// cache key is derived from). The waveform is no longer precomputed — the player derives it from
// the audio in-browser — so no peaks are stored.
func writeAudioSidecar(audioPath string, req generate.SpeechRequest, mime string, now time.Time) error {
	sc := map[string]any{
		"text_chars": len(req.Text),
		"voice":      req.Voice,
		"model":      req.Model,
		"format":     req.Format,
		"mime":       mime,
		"generated":  now.Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(audioPath+".json", append(b, '\n'), 0o644)
}

// audioMIMEByExt guesses an audio MIME from a filename extension (for recorded files).
func audioMIMEByExt(ref string) string {
	switch strings.ToLower(path.Ext(ref)) {
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".aac":
		return "audio/aac"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".opus":
		return "audio/opus"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	default:
		return "audio/mpeg"
	}
}
