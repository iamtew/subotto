package ai

import (
	"strings"
	"testing"
)

func TestDefaultCatalogEnvBootstrap(t *testing.T) {
	plain := DefaultCatalog("")
	if plain.Model != DefaultModel || len(plain.Models) != 2 || plain.Models[1] != DefaultFlashModel {
		t.Fatalf("%+v", plain)
	}
	custom := DefaultCatalog("acme/foo")
	if custom.Model != "acme/foo" || len(custom.Models) != 3 {
		t.Fatalf("%+v", custom)
	}
	nvidia := DefaultCatalog(DefaultModel)
	if nvidia.Model != DefaultModel || len(nvidia.Models) != 2 {
		t.Fatalf("env default should not duplicate: %+v", nvidia)
	}
}

func TestNormalizeCatalog(t *testing.T) {
	if _, err := NormalizeCatalog(ModelCatalog{
		Models: []string{strings.Repeat("x", MaxModelIDLen+1)},
	}); err == nil {
		t.Fatal("expected long id error")
	}
	got, err := NormalizeCatalog(ModelCatalog{
		Model:  "  b  ",
		Models: []string{" a ", "", "a", "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "b" || len(got.Models) != 2 || got.Models[0] != "a" {
		t.Fatalf("%+v", got)
	}
	got, err = NormalizeCatalog(ModelCatalog{Models: []string{"only"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "only" {
		t.Fatalf("missing select should pick first: %+v", got)
	}
	if _, err := NormalizeCatalog(ModelCatalog{}); err == nil {
		t.Fatal("empty catalog")
	}
}
