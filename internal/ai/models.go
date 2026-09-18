package ai

import (
	"fmt"
	"strings"
)

const (
	DefaultModel      = "nvidia/nemotron-3-ultra-550b-a55b:free"
	DefaultFlashModel = "deepseek/deepseek-v4-flash-0731:free"
	MaxModelIDLen     = 200
)

// ModelCatalog is the Admin OpenRouter model list plus the active id.
type ModelCatalog struct {
	Model  string   `json:"model"`
	Models []string `json:"models"`
}

// DefaultCatalog is nvidia + deepseek. envModel, if set, is selected and appended when missing.
func DefaultCatalog(envModel string) ModelCatalog {
	out := ModelCatalog{
		Model:  DefaultModel,
		Models: []string{DefaultModel, DefaultFlashModel},
	}
	env := strings.TrimSpace(envModel)
	if env == "" {
		return out
	}
	if !catalogHas(out.Models, env) {
		out.Models = append(out.Models, env)
	}
	out.Model = env
	return out
}

// NormalizeCatalog trims, de-dupes, and keeps the selected id in the list.
func NormalizeCatalog(in ModelCatalog) (ModelCatalog, error) {
	seen := map[string]struct{}{}
	models := make([]string, 0, len(in.Models))
	for _, raw := range in.Models {
		id, err := cleanModelID(raw)
		if err != nil {
			return ModelCatalog{}, err
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		models = append(models, id)
	}
	if len(models) == 0 {
		return ModelCatalog{}, fmt.Errorf("need at least one model")
	}
	sel, err := cleanModelID(in.Model)
	if err != nil {
		return ModelCatalog{}, err
	}
	if sel == "" || !catalogHas(models, sel) {
		sel = models[0]
	}
	return ModelCatalog{Model: sel, Models: models}, nil
}

func cleanModelID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", nil
	}
	if len(id) > MaxModelIDLen {
		return "", fmt.Errorf("model id is too long")
	}
	return id, nil
}

func catalogHas(models []string, id string) bool {
	for _, m := range models {
		if m == id {
			return true
		}
	}
	return false
}
