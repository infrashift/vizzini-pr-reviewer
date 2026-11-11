// Package chunker packs parsed diff files into token-bounded chunks for the
// LLM, splitting oversized files only at hunk boundaries.
package chunker

import (
	"fmt"
	"strings"

	"github.com/pkoukk/tiktoken-go"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
)

const (
	// DefaultMaxChunkTokens is the token budget for one chunk.
	DefaultMaxChunkTokens = 6000
	// DefaultEncoding is the BPE encoding used to count tokens.
	DefaultEncoding = "cl100k_base"
)

// Options configures a Chunker. Zero values select the defaults.
type Options struct {
	MaxChunkTokens int
	// Encoding is a tiktoken encoding name, such as "cl100k_base" or "o200k_base".
	Encoding string
}

// Chunker counts tokens and packs files into chunks. It is safe for
// concurrent use.
type Chunker struct {
	enc       *tiktoken.Tiktoken
	maxTokens int
}

// Part is a whole file, or a contiguous run of its hunks when the file alone
// exceeds the budget.
type Part struct {
	File  *diff.File
	Hunks []diff.Hunk
	// Continued is true for every part of a split file after the first.
	Continued bool
	Tokens    int
}

// Chunk is a group of parts rendered into one LLM request.
type Chunk struct {
	Parts  []Part
	Tokens int
	// Oversized is true when the chunk holds a single hunk that exceeds the
	// budget on its own. Hunks are never split mid-line, so the caller decides
	// whether to send, truncate or skip it.
	Oversized bool
}

// New returns a Chunker. The encoding is read from TIKTOKEN_CACHE_DIR; seed it
// with `make tiktoken-cache` so token counting never needs the network.
func New(opts Options) (*Chunker, error) {
	if opts.MaxChunkTokens < 0 {
		return nil, fmt.Errorf("MaxChunkTokens must not be negative, got %d", opts.MaxChunkTokens)
	}
	if opts.MaxChunkTokens == 0 {
		opts.MaxChunkTokens = DefaultMaxChunkTokens
	}
	if opts.Encoding == "" {
		opts.Encoding = DefaultEncoding
	}
	enc, err := tiktoken.GetEncoding(opts.Encoding)
	if err != nil {
		return nil, fmt.Errorf("load tiktoken encoding %q (is TIKTOKEN_CACHE_DIR seeded? see `make tiktoken-cache`): %w", opts.Encoding, err)
	}
	return &Chunker{enc: enc, maxTokens: opts.MaxChunkTokens}, nil
}

// CountTokens returns the number of tokens in s.
func (c *Chunker) CountTokens(s string) int {
	return len(c.enc.EncodeOrdinary(s))
}

// Chunk packs files, in order, into chunks of at most MaxChunkTokens tokens.
// A file that fits is never split. A file that does not fit is split into
// parts at hunk boundaries, and each part repeats the file header.
func (c *Chunker) Chunk(files []*diff.File) []Chunk {
	var (
		chunks []Chunk
		cur    Chunk
	)
	flush := func() {
		if len(cur.Parts) > 0 {
			chunks = append(chunks, cur)
		}
		cur = Chunk{}
	}

	for _, f := range files {
		for _, p := range c.split(f) {
			if p.Tokens > c.maxTokens {
				flush()
				chunks = append(chunks, Chunk{Parts: []Part{p}, Tokens: p.Tokens, Oversized: true})
				continue
			}
			if cur.Tokens+p.Tokens > c.maxTokens {
				flush()
			}
			cur.Parts = append(cur.Parts, p)
			cur.Tokens += p.Tokens
		}
	}
	flush()
	return chunks
}

// split returns the file as one part if it fits, else as several parts that
// each fit, except a part holding a single hunk that is too large by itself.
func (c *Chunker) split(f *diff.File) []Part {
	header := c.CountTokens(f.Header())
	hunkTokens := make([]int, len(f.Hunks))
	total := header
	for i := range f.Hunks {
		hunkTokens[i] = c.CountTokens(f.Hunks[i].Render())
		total += hunkTokens[i]
	}
	if total <= c.maxTokens || len(f.Hunks) <= 1 {
		return []Part{{File: f, Hunks: f.Hunks, Tokens: total}}
	}

	contHeader := c.CountTokens(continuedHeader(f))
	var parts []Part
	start := 0
	for start < len(f.Hunks) {
		h := header
		if len(parts) > 0 {
			h = contHeader
		}
		end, tokens := start, h
		for end < len(f.Hunks) && (end == start || tokens+hunkTokens[end] <= c.maxTokens) {
			tokens += hunkTokens[end]
			end++
		}
		parts = append(parts, Part{File: f, Hunks: f.Hunks[start:end], Continued: len(parts) > 0, Tokens: tokens})
		start = end
	}
	return parts
}

func continuedHeader(f *diff.File) string {
	return strings.TrimSuffix(f.Header(), "\n") + " [continued]\n"
}

// Render returns the part as prompt text: the file header followed by its hunks.
func (p *Part) Render() string {
	var b strings.Builder
	if p.Continued {
		b.WriteString(continuedHeader(p.File))
	} else {
		b.WriteString(p.File.Header())
	}
	for i := range p.Hunks {
		b.WriteString(p.Hunks[i].Render())
	}
	return b.String()
}

// Render returns the chunk as prompt text.
func (c *Chunk) Render() string {
	var b strings.Builder
	for i := range c.Parts {
		b.WriteString(c.Parts[i].Render())
	}
	return b.String()
}
