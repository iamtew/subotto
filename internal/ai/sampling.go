package ai

import "encoding/json"

const (
	MaxSamplingTokens = 8192
	MaxSamplingTopK   = 200
)

// Sampling is OpenRouter chat sampling knobs from Admin.
type Sampling struct {
	MaxTokens         int     `json:"max_tokens"`
	Temperature       float64 `json:"temperature"`
	TopP              float64 `json:"top_p"`
	TopK              int     `json:"top_k"`
	FrequencyPenalty  float64 `json:"frequency_penalty"`
	PresencePenalty   float64 `json:"presence_penalty"`
	RepetitionPenalty float64 `json:"repetition_penalty"`
	MinP              float64 `json:"min_p"`
	TopA              float64 `json:"top_a"`
}

// DefaultSampling matches the Admin mockup (0 = omit that knob).
func DefaultSampling() Sampling {
	return Sampling{
		Temperature:       0.7,
		TopP:              1,
		RepetitionPenalty: 1,
	}
}

type samplingWire struct {
	MaxTokens         *int     `json:"max_tokens"`
	Temperature       *float64 `json:"temperature"`
	TopP              *float64 `json:"top_p"`
	TopK              *int     `json:"top_k"`
	FrequencyPenalty  *float64 `json:"frequency_penalty"`
	PresencePenalty   *float64 `json:"presence_penalty"`
	RepetitionPenalty *float64 `json:"repetition_penalty"`
	MinP              *float64 `json:"min_p"`
	TopA              *float64 `json:"top_a"`
}

// ParseSamplingJSON fills defaults for missing keys, then clamps.
func ParseSamplingJSON(raw []byte) (Sampling, error) {
	var wire samplingWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Sampling{}, err
	}
	out := DefaultSampling()
	if wire.MaxTokens != nil {
		out.MaxTokens = *wire.MaxTokens
	}
	if wire.Temperature != nil {
		out.Temperature = *wire.Temperature
	}
	if wire.TopP != nil {
		out.TopP = *wire.TopP
	}
	if wire.TopK != nil {
		out.TopK = *wire.TopK
	}
	if wire.FrequencyPenalty != nil {
		out.FrequencyPenalty = *wire.FrequencyPenalty
	}
	if wire.PresencePenalty != nil {
		out.PresencePenalty = *wire.PresencePenalty
	}
	if wire.RepetitionPenalty != nil {
		out.RepetitionPenalty = *wire.RepetitionPenalty
	}
	if wire.MinP != nil {
		out.MinP = *wire.MinP
	}
	if wire.TopA != nil {
		out.TopA = *wire.TopA
	}
	return NormalizeSampling(out), nil
}

// NormalizeSampling clamps every knob to the Admin slider range.
func NormalizeSampling(in Sampling) Sampling {
	return Sampling{
		MaxTokens:         clampInt(in.MaxTokens, 0, MaxSamplingTokens),
		Temperature:       clampFloat(in.Temperature, 0, 2),
		TopP:              clampFloat(in.TopP, 0, 1),
		TopK:              clampInt(in.TopK, 0, MaxSamplingTopK),
		FrequencyPenalty:  clampFloat(in.FrequencyPenalty, -2, 2),
		PresencePenalty:   clampFloat(in.PresencePenalty, -2, 2),
		RepetitionPenalty: clampFloat(in.RepetitionPenalty, 0, 2),
		MinP:              clampFloat(in.MinP, 0, 1),
		TopA:              clampFloat(in.TopA, 0, 1),
	}
}

func (s *Sampling) apply(req *chatRequest) {
	if s == nil || req == nil {
		return
	}
	if s.MaxTokens > 0 {
		req.MaxTokens = &s.MaxTokens
	}
	req.Temperature = &s.Temperature
	req.TopP = &s.TopP
	if s.TopK > 0 {
		req.TopK = &s.TopK
	}
	req.FrequencyPenalty = &s.FrequencyPenalty
	req.PresencePenalty = &s.PresencePenalty
	req.RepetitionPenalty = &s.RepetitionPenalty
	if s.MinP > 0 {
		req.MinP = &s.MinP
	}
	if s.TopA > 0 {
		req.TopA = &s.TopA
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
