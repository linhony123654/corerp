package memory

import (
	"encoding/json"
	"math"
	"sort"

	"corerp/internal/core"
)

const (
	// Auto-switch threshold: use keyword search below this, vector search above.
	VectorThreshold = 100
	// VectorDims is the fixed dimensionality of the embedding space.
	VectorDims = 256
)

// VectorEmbedder converts text to fixed-size vectors.
// P3: character bigram based — zero external dependencies.
// Upgrade path: swap for ONNX all-MiniLM-L6-v2 via the same interface.
type VectorEmbedder struct{}

func NewVectorEmbedder() *VectorEmbedder {
	return &VectorEmbedder{}
}

// Embed converts text into a normalized float vector.
func (v *VectorEmbedder) Embed(text string) []float64 {
	vec := make([]float64, VectorDims)
	runes := []rune(text)
	if len(runes) < 2 {
		// Single character: use unicode codepoint
		idx := int(runes[0]) % VectorDims
		vec[idx] = 1.0
		return vec
	}

	// Character bigram hashing into fixed-size vector
	for i := 0; i < len(runes)-1; i++ {
		idx := (int(runes[i])*31 + int(runes[i+1])) % VectorDims
		if idx < 0 {
			idx = -idx
		}
		vec[idx] += 1.0
	}

	// Normalize to unit length
	sum := 0.0
	for _, v := range vec {
		sum += v * v
	}
	if sum > 0 {
		mag := math.Sqrt(sum)
		for i := range vec {
			vec[i] /= mag
		}
	}

	return vec
}

// cosineSimilarity returns the cosine similarity between two vectors.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

// VectorStore handles vector storage and similarity search.
type VectorStore struct {
	db      interface{ Exec(string, ...interface{}) error }
	embedder *VectorEmbedder
}

func NewVectorStore() *VectorStore {
	return &VectorStore{
		embedder: NewVectorEmbedder(),
	}
}

// EncodeVector serializes a float64 slice to JSON for storage.
func EncodeVector(vec []float64) ([]byte, error) {
	return json.Marshal(vec)
}

// DecodeVector deserializes a JSON vector back to float64 slice.
func DecodeVector(data []byte) ([]float64, error) {
	var vec []float64
	if err := json.Unmarshal(data, &vec); err != nil {
		return nil, err
	}
	return vec, nil
}

// SearchResult holds a single vector search result.
type VectorSearchResult struct {
	ID         string  `json:"id"`
	Score      float64 `json:"score"`
	Content    string  `json:"content"`
}

// SearchFacts performs vector similarity search over semantic facts.
// candidates: pre-filtered facts (by character), query: user input.
func (vs *VectorStore) SearchFacts(query string, candidates []core.FactFrame, limit int) []VectorSearchResult {
	queryVec := vs.embedder.Embed(query)

	type scored struct {
		fact  core.FactFrame
		score float64
	}
	var results []scored

	for _, f := range candidates {
		content := f.Subject + " " + f.Predicate + " " + f.Object
		factVec := vs.embedder.Embed(content)
		sim := cosineSimilarity(queryVec, factVec)
		// Boost by confidence
		sim *= f.Confidence
		results = append(results, scored{fact: f, score: sim})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	var out []VectorSearchResult
	for i, r := range results {
		if i >= limit {
			break
		}
		out = append(out, VectorSearchResult{
			ID:      r.fact.Subject + "_" + r.fact.Predicate,
			Score:   r.score,
			Content: r.fact.Subject + " " + r.fact.Predicate + " " + r.fact.Object,
		})
	}
	return out
}

// SearchEpisodic performs vector similarity search over episodic events.
func (vs *VectorStore) SearchEpisodic(query string, candidates []core.EventFrame, limit int) []VectorSearchResult {
	queryVec := vs.embedder.Embed(query)

	type scored struct {
		event core.EventFrame
		score float64
	}
	var results []scored

	for _, e := range candidates {
		eventVec := vs.embedder.Embed(e.Description)
		sim := cosineSimilarity(queryVec, eventVec)
		sim *= (1.0 + e.EmotionalWeight)
		results = append(results, scored{event: e, score: sim})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	var out []VectorSearchResult
	for i, r := range results {
		if i >= limit {
			break
		}
		out = append(out, VectorSearchResult{
			ID:      r.event.EventID,
			Score:   r.score,
			Content: r.event.Description,
		})
	}
	return out
}

// ShouldUseVector returns true when the data size warrants vector search.
func ShouldUseVector(totalFacts int) bool {
	return totalFacts >= VectorThreshold
}
