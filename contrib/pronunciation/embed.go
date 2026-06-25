// Package pronunciation embeds colophon's built-in, provider-agnostic pronunciation
// dictionaries so a site can reference one by name (e.g. pronunciation_dict: en_GB) without
// shipping its own file. The embed directive can only reach files beside it, so this package
// lives under contrib/.
package pronunciation

import "embed"

// FS holds the dictionary files: <locale>.yaml (e.g. en_GB.yaml).
//
//go:embed *.yaml
var FS embed.FS
