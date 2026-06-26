package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
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

// resolvedSpeech is one speech profile resolved to live state: its rendered settings, loaded
// pronunciation entries, cache dir and reuse policy, plus a lazily-built generator (so provider
// setup — e.g. the ElevenLabs dictionary sync — happens once per profile, only if used). The
// resolver caches one of these per effective profile name as posts reference them.
type resolvedSpeech struct {
	settings      generate.SpeechSettings
	provider      string
	pronunciation []generate.Pronunciation
	cacheDir      string
	reuseContent  bool
	resolveErr    error // ResolveProfile/ResolveSpeech failure → TTS disabled for posts using it

	once sync.Once
	gen  generate.SpeechGenerator
	err  error // generator construction (PrepareSpeech/NewSpeech) failure
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
	cache   string          // tts: absolute cache path, named by content identity
	legacy  string          // tts: pre-split single-hash cache path, adopted on migration if present
	rs      *resolvedSpeech // tts: the resolved speech profile that renders this clip
	force   bool            // tts: re-render even if cache exists (--regenerate)
	size    int64           // filled after run()
	peaks   []float64       // waveform amplitude peaks (0–1), when computed
}

// audioResolver maps a post's audio (recorded audio_file or generated audio:true) to a
// stable URL, accumulates the clips to publish, and produces them. Recorded audio works
// whenever a resolver exists; TTS additionally needs speech to be configured.
type audioResolver struct {
	speechGen      core.SpeechGen // default block + named profiles
	configured     bool           // a speech provider is set (TTS possible)
	envProfile     string         // environment-selected speech profile; a post's wins over it
	root           string
	retry          generate.RetryPolicy // provider rate-limit backoff, applied to each resolved profile
	siteID         string               // namespaces shared-account provider state (e.g. ElevenLabs dict name)
	basePath       string
	baseURL        string
	router         *core.Router
	generateAI     bool
	voiceFor       func(postVoice, author, persona string) string
	log            *clog.Logger
	i18n           ttsTable                   // injected-speech translations (block cues, hint, wrap-up, symbols)
	defaultLang    string                     // site language, used when a post sets none
	acronyms       *acronymReplacer           // glossary acronym → spoken expansion
	defaultAudioOn bool                       // per-post audio: default when a post sets none
	regenerate     bool                       // --regenerate: re-render even when cached
	profiles       map[string]*resolvedSpeech // resolved speech profiles, keyed by effective name
	pronCache      map[string][]generate.Pronunciation
	jobs           map[string]*audioJob
}

func newAudioResolver(speech core.SpeechGen, envProfile, root, basePath, baseURL string, router *core.Router, generateAI, regenerate bool, voiceFor func(string, string, string) string, defaultLang string, acronyms *acronymReplacer, defaultAudioOn bool, log *clog.Logger) *audioResolver {
	ar := &audioResolver{
		speechGen: speech, configured: speech.Configured(), envProfile: envProfile, root: root,
		basePath: basePath, baseURL: baseURL, router: router,
		generateAI: generateAI, regenerate: regenerate, voiceFor: voiceFor, defaultLang: defaultLang,
		acronyms: acronyms, defaultAudioOn: defaultAudioOn, i18n: loadTTSTable(root), log: log,
		profiles: map[string]*resolvedSpeech{}, pronCache: map[string][]generate.Pronunciation{},
		jobs: map[string]*audioJob{},
	}
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		ar.siteID = u.Host
	}
	return ar
}

// resolved returns the live state for a speech profile, resolving and caching it on first use.
// The post's profile name wins over the environment's; an empty name selects the default block.
// A resolution failure is cached (and warned once) so posts that reference it skip TTS cleanly.
func (ar *audioResolver) resolved(postProfile string) *resolvedSpeech {
	name := strings.TrimSpace(postProfile)
	if name == "" {
		name = strings.TrimSpace(ar.envProfile)
	}
	key := name
	if key == "" {
		key = "default"
	}
	if rs, ok := ar.profiles[key]; ok {
		return rs
	}
	rs := &resolvedSpeech{}
	g, err := ar.speechGen.ResolveProfile(name)
	if err == nil {
		var s generate.SpeechSettings
		if s, err = generate.ResolveSpeech(g); err == nil {
			s.SiteID = ar.siteID
			s.Retry = ar.retry
			rs.settings = s
			rs.provider = s.Provider
			rs.reuseContent = s.Reuse == "content"
			rs.cacheDir = ar.dirFor(s.OutputDir)
			rs.pronunciation = ar.loadPron(s.PronunciationDict)
		}
	}
	if err != nil {
		ar.log.Step("AUDIO", "", "warn", fmt.Sprintf("speech profile %q unavailable: %v", key, err))
	}
	rs.resolveErr = err
	ar.profiles[key] = rs
	return rs
}

// dirFor resolves a possibly-relative output_dir against the site root.
func (ar *audioResolver) dirFor(dir string) string {
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(ar.root, filepath.FromSlash(dir))
	}
	return dir
}

// loadPron loads a pronunciation dictionary by ref (built-in name or path), caching by ref so
// profiles that share a dict load it once.
func (ar *audioResolver) loadPron(ref string) []generate.Pronunciation {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	if p, ok := ar.pronCache[ref]; ok {
		return p
	}
	entries, err := generate.ResolvePronunciationDict(ref, ar.root)
	if err != nil {
		ar.log.Step("AUDIO", "pronunciation dict ignored", "ref", ref, "err", err.Error())
		entries = nil
	} else {
		ar.log.Detail("AUDIO", "pronunciation dict loaded", "ref", ref, "entries", len(entries))
	}
	ar.pronCache[ref] = entries
	return entries
}

// ensureGen builds (once) the generator for a resolved profile, running provider setup
// (e.g. the ElevenLabs dictionary sync) and warning on failure.
func (ar *audioResolver) ensureGen(rs *resolvedSpeech) (generate.SpeechGenerator, error) {
	rs.once.Do(func() {
		s, err := generate.PrepareSpeech(context.Background(), rs.settings, rs.pronunciation, rs.cacheDir)
		if err != nil {
			ar.log.Step("AUDIO", "", "warn", "pronunciation dictionary sync failed: "+err.Error())
			s = rs.settings
		}
		rs.gen, rs.err = generate.NewSpeech(s)
		if rs.err != nil {
			ar.log.Step("AUDIO", rs.provider, "warn", "speech generator unavailable: "+rs.err.Error())
		}
	})
	return rs.gen, rs.err
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

// registerTTS resolves a generated reading to its URLs and queues it, rendering with the post's
// speech profile (else the environment's, else the default block). ok is false when speech
// generation isn't configured or the selected profile failed to resolve.
func (ar *audioResolver) registerTTS(label, htmlBody, lang, postVoice, author, persona, speechProfile string) (rel, abs, mime, outPath string, ok bool) {
	if !ar.active() || !ar.configured {
		return "", "", "", "", false
	}
	rs := ar.resolved(speechProfile)
	if rs.resolveErr != nil {
		return "", "", "", "", false
	}
	if strings.TrimSpace(lang) == "" {
		lang = ar.defaultLang
	}
	// strip/cue code, math, tables, diagrams; spell inline-code symbols — in the post's language
	text := speechText(htmlBody, rs.settings.Transcript, ar.i18n.For(lang), ar.acronyms)
	if strings.TrimSpace(text) == "" {
		return "", "", "", "", false
	}
	voice := rs.settings.Voice
	if v := strings.TrimSpace(ar.voiceFor(postVoice, author, persona)); v != "" {
		voice = v
	}
	pron := generate.FilterPronunciation(rs.pronunciation, text)
	// Publish and cache under the content identity (text + pronunciation) so the URL is stable
	// across provider/model/voice changes; the renderer is recorded in the sidecar, and reuse
	// policy decides whether a renderer change re-renders the single content-named file.
	content := generate.SpeechContentStem(label, text, pron)
	outPath = genOutDir + "/" + content + ttsOutputExt
	mime = ttsOutputMIME
	if _, seen := ar.jobs[outPath]; !seen {
		ar.jobs[outPath] = &audioJob{
			kind: "tts", outPath: outPath, mime: mime, rs: rs,
			req:    generate.SpeechRequest{Text: text, Voice: voice, Model: rs.settings.Model, Pronunciation: pron},
			cache:  filepath.Join(rs.cacheDir, content+ttsOutputExt),
			legacy: filepath.Join(rs.cacheDir, generate.SpeechStem(rs.provider, rs.settings.Model, voice, label, text, pron)+ttsOutputExt),
			force:  ar.regenerate,
		}
	}
	rel, abs = ar.urlFor(outPath)
	return rel, abs, mime, outPath, true
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// sidecarRenderMatches reports whether the clip's sidecar records the given renderer
// (provider/model/voice). A missing or unreadable sidecar matches (true) so clips without
// provenance — older builds, hand-placed files — are reused rather than churned.
func sidecarRenderMatches(cachePath, provider, model, voice string) bool {
	b, err := os.ReadFile(cachePath + ".json")
	if err != nil {
		return true
	}
	var sc struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		Voice    string `json:"voice"`
	}
	if json.Unmarshal(b, &sc) != nil || sc.Provider == "" {
		return true // unreadable or pre-render-tracking sidecar → reuse, don't churn
	}
	return sc.Provider == provider && sc.Model == model && sc.Voice == voice
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

	// Sync provider-side state (the ElevenLabs IPA pronunciation dictionary) for every speech
	// profile in play even when its readings are all cached, so the dictionary exists and
	// refreshes on any --generate-ai build — not only when a reading is (re)generated. Skipped
	// for providers that send pronunciations inline (MiniMax), and a no-op when nothing changed.
	if ar.generateAI {
		for _, rs := range ar.profiles {
			if rs.resolveErr == nil && generate.NeedsDictSync(rs.settings, rs.pronunciation) {
				_, _ = ar.ensureGen(rs)
			}
		}
	}

	results := make([]audioOutcome, len(keys))
	limit := generate.DefaultConcurrency
	if ar.configured { // skip when only recorded files are queued (no TTS provider to resolve)
		if rs := ar.resolved(""); rs.resolveErr == nil && rs.settings.Concurrency > 0 {
			limit = rs.settings.Concurrency
		}
	}
	sem := make(chan struct{}, max(1, limit))
	var wg sync.WaitGroup
	for i, k := range keys {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j *audioJob) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = ar.produce(j, now)
		}(i, ar.jobs[k])
	}
	wg.Wait()
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

func (ar *audioResolver) produce(j *audioJob, now time.Time) (out audioOutcome) {
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
		rs := j.rs
		provider := rs.provider
		if !j.force {
			if b, err := os.ReadFile(j.cache); err == nil {
				// A content-named clip exists. Reuse it unless reuse:exact and the renderer
				// recorded in the sidecar differs from the current provider/model/voice.
				if rs.reuseContent || sidecarRenderMatches(j.cache, provider, j.req.Model, j.req.Voice) {
					j.size = int64(len(b))
					if meta, err := os.ReadFile(j.cache + ".json"); err == nil {
						j.peaks = parsePeaks(meta)
					}
					// Backfill peaks for clips cached before server-side waveforms — derived from the
					// cached bytes, no re-render — so every reading ships a sidecar.
					if len(j.peaks) == 0 {
						if pk, ok := peaksFromWAV(b); ok {
							j.peaks = pk
							_ = writeAudioSidecar(j.cache, provider, j.req, j.peaks, now)
						}
					}
					out.bytes, out.detail, out.logArgs = b, true, []any{"cached", path.Base(j.outPath), "bytes", len(b)}
					return out
				}
				// reuse:exact + renderer changed → fall through and re-render.
			} else if b, err := os.ReadFile(j.legacy); err == nil {
				// Migration: adopt a clip cached by a pre-content-naming build under the new
				// content name (+ sidecar), so upgrading doesn't re-render unchanged audio.
				if meta, err := os.ReadFile(j.legacy + ".json"); err == nil {
					j.peaks = parsePeaks(meta)
				}
				if len(j.peaks) == 0 {
					if pk, ok := peaksFromWAV(b); ok {
						j.peaks = pk
					}
				}
				if err := os.MkdirAll(rs.cacheDir, 0o755); err == nil {
					_ = os.WriteFile(j.cache, b, 0o644)
					_ = writeAudioSidecar(j.cache, provider, j.req, j.peaks, now)
				}
				j.size = int64(len(b))
				out.bytes, out.detail, out.logArgs = b, true, []any{"adopted", path.Base(j.outPath), "bytes", len(b)}
				return out
			}
		}
		if !ar.generateAI {
			out.logArgs = []any{"skip", path.Base(j.outPath), "hint", "build --generate-ai to create it"}
			return out
		}
		gen, err := ar.ensureGen(rs)
		if err != nil {
			out.logArgs = []any{"skip", path.Base(j.outPath)}
			return out
		}
		res, err := gen.Generate(context.Background(), j.req)
		if err != nil {
			out.logArgs = []any{"warn", fmt.Sprintf("generate %q failed: %v", path.Base(j.outPath), err)}
			return out
		}
		// The provider returns raw PCM; wrap it as WAV. The waveform peaks are derived from those
		// same samples — not a second render — and shipped beside the audio so the player uses
		// them directly; the in-browser visualiser is only a fallback when the sidecar is absent.
		samples := pcmToSamples(res.Bytes)
		audio := encodeWAV(samples, res.SampleRate)
		if pk, ok := peaksFromSamples(samples); ok {
			j.peaks = pk
		}
		if err := os.MkdirAll(rs.cacheDir, 0o755); err != nil {
			out.fatal = err
			return out
		}
		if err := os.WriteFile(j.cache, audio, 0o644); err != nil {
			out.fatal = err
			return out
		}
		_ = writeAudioSidecar(j.cache, provider, j.req, j.peaks, now)
		j.size = int64(len(audio))
		out.bytes, out.logArgs = audio, []any{"generated", path.Base(j.outPath), "voice", j.req.Voice, "bytes", len(audio)}
		return out
	}
}

// writeAudioSidecar records cache metadata next to a generated clip: provenance, the renderer
// identity (provider/model/voice) that reuse:exact compares against, and the precomputed waveform
// peaks (when available) so a cache hit reuses them without re-analysing the audio. The same peaks
// are published beside the clip as <audio>.json; the player falls back to an in-browser waveform
// only when that sidecar is absent.
func writeAudioSidecar(audioPath, provider string, req generate.SpeechRequest, peaks []float64, now time.Time) error {
	sc := map[string]any{
		"text_chars": len(req.Text),
		"provider":   provider,
		"voice":      req.Voice,
		"model":      req.Model,
		"generated":  now.Format(time.RFC3339),
	}
	if len(peaks) > 0 {
		sc["peaks"] = peaks
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
