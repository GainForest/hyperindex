// Package lexicons exposes the Hypercerts Lexicon bundle installed by
// @atproto/lex and compiled into the Hyperindex binary.
package lexicons

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	internallexicon "github.com/GainForest/hyperindex/internal/lexicon"
)

// documents contains the CID-pinned Lexicon files tracked by ../lexicons.json.
// Naming each authority directory keeps non-Lexicon package files out of the
// runtime bundle.
//
//go:embed app com org pub
var documents embed.FS

// Load returns a fresh map of bundled Lexicon documents keyed by NSID.
func Load() (map[string][]byte, error) {
	loaded := make(map[string][]byte)
	origins := make(map[string]string)

	err := fs.WalkDir(documents, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		raw, err := documents.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read bundled Lexicon %s: %w", path, err)
		}

		var marker struct {
			Lexicon json.RawMessage `json:"lexicon"`
			ID      string          `json:"id"`
		}
		if err := json.Unmarshal(raw, &marker); err != nil {
			return fmt.Errorf("decode bundled Lexicon %s: %w", path, err)
		}
		if len(marker.Lexicon) == 0 || marker.ID == "" {
			return fmt.Errorf("bundled file %s is not a Lexicon document", path)
		}
		lex, err := internallexicon.ParseBytes(raw)
		if err != nil {
			return fmt.Errorf("parse bundled Lexicon %s: %w", path, err)
		}
		if existingPath, duplicate := origins[lex.ID]; duplicate {
			return fmt.Errorf("duplicate bundled Lexicon id %s declared by both %s and %s", lex.ID, existingPath, path)
		}

		origins[lex.ID] = path
		loaded[lex.ID] = raw
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("hypercerts Lexicon bundle is empty; run npx @atproto/lex@0.3.0 install --ci")
	}
	return loaded, nil
}
