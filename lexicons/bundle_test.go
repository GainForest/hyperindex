package lexicons_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/bluesky-social/indigo/atproto/atdata"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

	"github.com/GainForest/hyperindex/internal/validation"
	bundledlexicons "github.com/GainForest/hyperindex/lexicons"
)

func TestLoadMatchesManifestAndBuildsValidator(t *testing.T) {
	loaded, err := bundledlexicons.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	manifestJSON, err := os.ReadFile("../lexicons.json")
	if err != nil {
		t.Fatalf("read lexicons.json: %v", err)
	}
	var manifest struct {
		Resolutions map[string]struct {
			URI string `json:"uri"`
			CID string `json:"cid"`
		} `json:"resolutions"`
	}
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		t.Fatalf("decode lexicons.json: %v", err)
	}
	if len(loaded) != len(manifest.Resolutions) {
		t.Fatalf("bundled Lexicon count = %d, manifest resolutions = %d", len(loaded), len(manifest.Resolutions))
	}
	for id, resolution := range manifest.Resolutions {
		if resolution.URI == "" || resolution.CID == "" {
			t.Fatalf("manifest resolution for %s is missing URI or CID", id)
		}
		raw, ok := loaded[id]
		if !ok {
			t.Errorf("manifest Lexicon %s is not embedded", id)
			continue
		}
		value, err := atdata.UnmarshalJSON(raw)
		if err != nil {
			t.Errorf("decode bundled Lexicon %s as AT Protocol data: %v", id, err)
			continue
		}
		encoded, err := atdata.MarshalCBOR(value)
		if err != nil {
			t.Errorf("encode bundled Lexicon %s as DAG-CBOR: %v", id, err)
			continue
		}
		gotCID, err := cid.Prefix{
			Version:  1,
			Codec:    cid.DagCBOR,
			MhType:   multihash.SHA2_256,
			MhLength: -1,
		}.Sum(encoded)
		if err != nil {
			t.Errorf("compute CID for bundled Lexicon %s: %v", id, err)
			continue
		}
		if gotCID.String() != resolution.CID {
			t.Errorf("bundled Lexicon %s CID = %s, manifest pins %s", id, gotCID, resolution.CID)
		}
	}

	for _, id := range []string{
		"org.hypercerts.workscope.cel",
		"org.hypercerts.claim.activity",
		"app.certified.actor.profile",
		"com.atproto.repo.strongRef",
	} {
		if _, ok := loaded[id]; !ok {
			t.Errorf("required bundled Lexicon %s is missing", id)
		}
	}

	if _, err := validation.NewValidatorFromLexiconBytes(loaded); err != nil {
		t.Fatalf("bundled Lexicons do not build an Indigo validator: %v", err)
	}
}

func TestLoadReturnsIndependentDocuments(t *testing.T) {
	first, err := bundledlexicons.Load()
	if err != nil {
		t.Fatalf("first Load() error = %v", err)
	}
	second, err := bundledlexicons.Load()
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}

	first["org.hypercerts.workscope.cel"][0] = 'x'
	if second["org.hypercerts.workscope.cel"][0] == 'x' {
		t.Fatal("Load() returned mutable bytes shared between calls")
	}
}
