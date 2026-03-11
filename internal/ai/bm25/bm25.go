package bm25

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// Encoder encodes text into sparse BM25 vectors using character bigrams.
// This is suitable for Chinese text where word boundaries are implicit.
type Encoder struct {
	k1 float64 // term saturation parameter (typically 1.2–2.0)
	b  float64 // length normalization parameter (typically 0.75)
}

// NewEncoder creates a BM25 encoder with standard parameters.
func NewEncoder() *Encoder {
	return &Encoder{k1: 1.5, b: 0.75}
}

// Encode converts text to sparse vector representation as (positions, values).
// Positions are hashed bigram IDs; values are TF-IDF-like BM25 weights.
// The output is suitable for entity.NewSliceSparseEmbedding.
func (e *Encoder) Encode(text string) (positions []uint32, values []float32) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	// Build bigram term frequency map
	tf := make(map[uint32]int)
	runes := []rune(text)
	docLen := len(runes)

	for i := 0; i < len(runes)-1; i++ {
		bigram := string(runes[i : i+2])
		h := hashBigram(bigram)
		tf[h]++
	}
	// Also add unigrams for very short texts
	if utf8.RuneCountInString(text) < 3 {
		for _, r := range runes {
			h := hashBigram(string(r))
			tf[h]++
		}
	}

	if len(tf) == 0 {
		return nil, nil
	}

	// Typical average document length for short knowledge chunks (~200 chars)
	avgDocLen := 200.0
	normLen := float64(docLen) / avgDocLen

	// Compute BM25 term weights: weight = tf * (k1+1) / (tf + k1*(1-b+b*normLen))
	// IDF is approximated as 1.0 since we have no corpus statistics at encode time.
	posSlice := make([]uint32, 0, len(tf))
	valMap := make(map[uint32]float32, len(tf))
	for pos, freq := range tf {
		w := float64(freq) * (e.k1 + 1) / (float64(freq) + e.k1*(1-e.b+e.b*normLen))
		valMap[pos] = float32(math.Max(w, 0))
		posSlice = append(posSlice, pos)
	}

	// Milvus requires positions to be sorted in ascending order
	sort.Slice(posSlice, func(i, j int) bool { return posSlice[i] < posSlice[j] })

	vals := make([]float32, len(posSlice))
	for i, p := range posSlice {
		vals[i] = valMap[p]
	}
	return posSlice, vals
}

// hashBigram maps a bigram string to a uint32 bucket using FNV-like hashing.
// We use 24-bit space (16M buckets) to keep sparse vectors reasonably sized.
func hashBigram(s string) uint32 {
	const (
		prime  = uint32(16777619)
		offset = uint32(2166136261)
		mask   = uint32(0xFFFFFF) // 24-bit = ~16M buckets
	)
	h := offset
	for _, b := range []byte(s) {
		h ^= uint32(b)
		h *= prime
	}
	return h & mask
}
