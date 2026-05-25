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
// Tries remote bge-small-zh server first; falls back to 2-gram if unavailable.
type VectorEmbedder struct {
	remote *RemoteEmbedder
}

func NewVectorEmbedder() *VectorEmbedder {
	return &VectorEmbedder{
		remote: NewRemoteEmbedder(),
	}
}

// Chinese stopwords and punctuation to strip before vectorization.
var stopRunes = map[rune]bool{
	'的': true, '了': true, '在': true, '是': true, '我': true,
	'你': true, '他': true, '她': true, '它': true, '们': true,
	'这': true, '那': true, '吗': true, '呢': true, '吧': true,
	'啊': true, '哦': true, '嗯': true, '哈': true, '呀': true,
	'就': true, '也': true, '都': true, '还': true, '要': true,
	'会': true, '能': true, '可': true, '把': true, '被': true,
	'和': true, '与': true, '对': true, '从': true, '到': true,
	'让': true, '给': true, '为': true, '向': true, '跟': true,
	'有': true, '没': true, '不': true, '很': true, '太': true,
	'个': true, '些': true, '次': true, '点': true, '里': true,
	'上': true, '下': true, '中': true, '前': true, '后': true,
	'去': true, '来': true, '做': true, '说': true, '看': true,
	'想': true, '知': true, '道': true, '得': true, '着': true,
	'过': true, '一': true, '两': true, '几': true, '什': true,
	'么': true, '怎': true, '样': true, '哪': true, '谁': true,
	'事': true, '人': true, '大': true, '小': true, '多': true,
	'少': true, '已': true, '经': true, '以': true, '及': true,
	'所': true, '但': true, '而': true, '或': true, '且': true,
	'只': true, '又': true, '再': true, '才': true, '刚': true,
	'最': true, '更': true, '比': true, '等': true, '其': true,
	'之': true, '将': true, '著': true,
	'！': true, '？': true, '。': true, '，': true,
	'、': true, '：': true, '；': true, '（': true, '）': true,
	'"': true, '\'': true, '「': true,
	'」': true, '『': true, '』': true, '—': true,
	'《': true, '》': true, '【': true, '】': true, '~': true,
	' ': true, '\t': true, '\n': true, '\r': true,
}

func (v *VectorEmbedder) preprocess(runes []rune) []rune {
	filtered := make([]rune, 0, len(runes))
	for _, r := range runes {
		if !stopRunes[r] {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// Embed converts text into a normalized float vector.
// Tries remote bge-small-zh first; falls back to local 2-gram if unavailable.
func (v *VectorEmbedder) Embed(text string) []float64 {
	// Try remote semantic embedding
	if v.remote.Available() {
		vec, err := v.remote.Embed(text)
		if err == nil && len(vec) > 0 {
			return vec
		}
	}

	// Fallback: local 2-gram vectorization
	return v.embedLocal(text)
}

// embedLocal is the 2-gram fallback vectorizer.
// Chinese stopwords/punctuation stripped; falls back to raw if over-filtered.
func (v *VectorEmbedder) embedLocal(text string) []float64 {
	vec := make([]float64, VectorDims)
	raw := []rune(text)
	filtered := v.preprocess(raw)

	// Fall back to raw if stopword removal emptied or nearly emptied the text
	if len(filtered) < 2 {
		filtered = raw
	}
	if len(filtered) < 2 {
		if len(filtered) == 1 {
			idx := int(filtered[0]) % VectorDims
			vec[idx] = 1.0
		}
		return vec
	}

	// Character bigram hashing into fixed-size vector
	for i := 0; i < len(filtered)-1; i++ {
		idx := (int(filtered[i])*31 + int(filtered[i+1])) % VectorDims
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
func (vs *VectorStore) SearchFacts(query string, candidates []core.FactFrame, limit int) []VectorSearchResult {
	// Determine embedding method once (remote vs local) for consistency
	useRemote := vs.embedder.remote.Available()
	var queryVec []float64
	var factVecs [][]float64

	if useRemote {
		// Batch embed: query + all candidates at once
		texts := []string{query}
		for _, f := range candidates {
			texts = append(texts, f.Subject+" "+f.Predicate+" "+f.Object)
		}
		batched, err := vs.embedder.remote.EmbedBatch(texts)
		if err == nil && len(batched) == len(texts) {
			queryVec = batched[0]
			factVecs = batched[1:]
		}
	}

	if queryVec == nil {
		// Fallback: local 2-gram individually
		useRemote = false
		queryVec = vs.embedder.embedLocal(query)
		for _, f := range candidates {
			factVecs = append(factVecs, vs.embedder.embedLocal(f.Subject+" "+f.Predicate+" "+f.Object))
		}
	}

	type scored struct {
		fact  core.FactFrame
		score float64
	}
	var results []scored

	for i, f := range candidates {
		if i >= len(factVecs) {
			break
		}
		sim := cosineSimilarity(queryVec, factVecs[i])
		sim *= f.Confidence
		results = append(results, scored{fact: f, score: sim})
	}
	_ = useRemote

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
	useRemote := vs.embedder.remote.Available()
	var queryVec []float64
	var eventVecs [][]float64

	if useRemote {
		texts := []string{query}
		for _, e := range candidates {
			texts = append(texts, e.Description)
		}
		batched, err := vs.embedder.remote.EmbedBatch(texts)
		if err == nil && len(batched) == len(texts) {
			queryVec = batched[0]
			eventVecs = batched[1:]
		}
	}

	if queryVec == nil {
		useRemote = false
		queryVec = vs.embedder.embedLocal(query)
		for _, e := range candidates {
			eventVecs = append(eventVecs, vs.embedder.embedLocal(e.Description))
		}
	}

	type scored struct {
		event core.EventFrame
		score float64
	}
	var results []scored

	for i, e := range candidates {
		if i >= len(eventVecs) {
			break
		}
		sim := cosineSimilarity(queryVec, eventVecs[i])
		sim *= (1.0 + e.EmotionalWeight)
		results = append(results, scored{event: e, score: sim})
	}
	_ = useRemote

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
