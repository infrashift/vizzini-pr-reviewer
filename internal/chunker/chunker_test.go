package chunker

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
)

// TestMain keeps the tests offline: tiktoken-go falls back to an HTTP download
// on a cache miss, so point it at the seeded cache and route any request to a
// dead proxy. A missing cache then fails loudly instead of hitting the network.
func TestMain(m *testing.M) {
	if err := offlineEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "chunker tests:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func offlineEnv() error {
	if os.Getenv("TIKTOKEN_CACHE_DIR") == "" {
		dir, err := filepath.Abs("../../.cache/tiktoken")
		if err != nil {
			return err
		}
		if err := os.Setenv("TIKTOKEN_CACHE_DIR", dir); err != nil {
			return err
		}
	}
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if err := os.Setenv(k, "http://127.0.0.1:1"); err != nil {
			return err
		}
	}
	for _, k := range []string{"NO_PROXY", "no_proxy"} {
		if err := os.Unsetenv(k); err != nil {
			return err
		}
	}
	return nil
}

func newChunker(t *testing.T, maxTokens int) *Chunker {
	t.Helper()
	c, err := New(Options{MaxChunkTokens: maxTokens})
	if err != nil {
		t.Fatalf("New: %v (run `make tiktoken-cache` to seed %s)", err, os.Getenv("TIKTOKEN_CACHE_DIR"))
	}
	return c
}

// genPatch builds a valid unified diff: each hunk has one context line followed
// by linesPerHunk added lines.
func genPatch(prefix string, files, hunksPerFile, linesPerHunk int) string {
	var b strings.Builder
	for i := range files {
		name := fmt.Sprintf("pkg/%s%02d.go", prefix, i)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n", name, name, name, name)
		for j := range hunksPerFile {
			oldStart := 1 + j*100
			newStart := oldStart + j*linesPerHunk
			fmt.Fprintf(&b, "@@ -%d,1 +%d,%d @@ func f%d()\n", oldStart, newStart, linesPerHunk+1, j)
			fmt.Fprintf(&b, " \t// anchor %d\n", j)
			for k := range linesPerHunk {
				fmt.Fprintf(&b, "+\tvalue%d_%d := compute(%d, \"some words here\")\n", j, k, k)
			}
		}
	}
	return b.String()
}

func parse(t *testing.T, patch string) []*diff.File {
	t.Helper()
	d, err := diff.Parse(strings.NewReader(patch), diff.Options{})
	if err != nil {
		t.Fatalf("diff.Parse: %v", err)
	}
	return d.Files
}

// checkInvariants asserts what must hold for every chunking result: budgets are
// respected (except flagged oversized chunks), token counts match the rendered
// text, and every hunk appears exactly once in the original order.
func checkInvariants(t *testing.T, c *Chunker, files []*diff.File, chunks []Chunk) {
	t.Helper()
	var gotHunks []*diff.Hunk
	for i := range chunks {
		ch := &chunks[i]
		if ch.Tokens > c.maxTokens && !ch.Oversized {
			t.Errorf("chunk %d has %d tokens > max %d and is not flagged oversized", i, ch.Tokens, c.maxTokens)
		}
		if got := c.CountTokens(ch.Render()); got != ch.Tokens {
			t.Errorf("chunk %d: Tokens=%d but rendered text counts %d", i, ch.Tokens, got)
		}
		sum := 0
		for j := range ch.Parts {
			p := &ch.Parts[j]
			sum += p.Tokens
			if got := c.CountTokens(p.Render()); got != p.Tokens {
				t.Errorf("chunk %d part %d: Tokens=%d but rendered text counts %d", i, j, p.Tokens, got)
			}
			for k := range p.Hunks {
				gotHunks = append(gotHunks, &p.Hunks[k])
			}
		}
		if sum != ch.Tokens {
			t.Errorf("chunk %d: part tokens sum to %d, chunk says %d", i, sum, ch.Tokens)
		}
	}
	var wantHunks []*diff.Hunk
	for _, f := range files {
		for k := range f.Hunks {
			wantHunks = append(wantHunks, &f.Hunks[k])
		}
	}
	if len(gotHunks) != len(wantHunks) {
		t.Fatalf("chunks hold %d hunks, files have %d", len(gotHunks), len(wantHunks))
	}
	for i := range wantHunks {
		if gotHunks[i].Fragment != wantHunks[i].Fragment {
			t.Errorf("hunk %d out of order or missing", i)
		}
	}
}

func TestChunk_MultiFilePacking(t *testing.T) {
	files := parse(t, genPatch("f", 12, 1, 8))
	probe := newChunker(t, DefaultMaxChunkTokens)
	perFile := probe.split(files[0])[0].Tokens

	// Room for three files per chunk, not four.
	c := newChunker(t, perFile*3+perFile/2)
	chunks := c.Chunk(files)
	checkInvariants(t, c, files, chunks)

	if len(chunks) != 4 {
		t.Fatalf("got %d chunks, want 4 (12 files, 3 per chunk)", len(chunks))
	}
	seen := 0
	for i := range chunks {
		for _, p := range chunks[i].Parts {
			if p.Continued || len(p.Hunks) != len(p.File.Hunks) {
				t.Errorf("file %s was split although it fits the budget", p.File.Path)
			}
			if p.File != files[seen] {
				t.Errorf("part %d is %s, want %s", seen, p.File.Path, files[seen].Path)
			}
			seen++
		}
		// Greedy: the next chunk's first part would not have fit in this one.
		if i+1 < len(chunks) && chunks[i].Tokens+chunks[i+1].Parts[0].Tokens <= c.maxTokens {
			t.Errorf("chunk %d closed early: next part would fit", i)
		}
	}
	if seen != len(files) {
		t.Errorf("packed %d files, want %d", seen, len(files))
	}
}

func TestChunk_OversizedFileSplitsAtHunkBoundaries(t *testing.T) {
	files := parse(t, genPatch("big", 1, 10, 6))
	probe := newChunker(t, DefaultMaxChunkTokens)
	whole := probe.split(files[0])[0].Tokens

	c := newChunker(t, whole/3)
	chunks := c.Chunk(files)
	checkInvariants(t, c, files, chunks)

	var parts []Part
	for i := range chunks {
		if chunks[i].Oversized {
			t.Errorf("chunk %d flagged oversized; every single hunk fits the budget", i)
		}
		parts = append(parts, chunks[i].Parts...)
	}
	if len(parts) < 3 {
		t.Fatalf("got %d parts, want the file split into at least 3", len(parts))
	}
	for i := range parts {
		p := &parts[i]
		if p.Continued != (i > 0) {
			t.Errorf("part %d Continued=%v, want %v", i, p.Continued, i > 0)
		}
		r := p.Render()
		if !strings.HasPrefix(r, "### pkg/big00.go (modified)") {
			t.Errorf("part %d does not start with the file header: %q", i, firstLine(r))
		}
		if strings.Contains(firstLine(r), "[continued]") != p.Continued {
			t.Errorf("part %d header %q, Continued=%v", i, firstLine(r), p.Continued)
		}
		if strings.Count(r, "\n@@ ") != len(p.Hunks) {
			t.Errorf("part %d renders a partial hunk", i)
		}
	}
}

func TestChunk_SingleHunkLargerThanBudget(t *testing.T) {
	patch := genPatch("a", 1, 1, 2) + genPatch("huge", 1, 1, 200) + genPatch("z", 1, 1, 2)
	files := parse(t, patch)
	c := newChunker(t, 200)
	chunks := c.Chunk(files)
	checkInvariants(t, c, files, chunks)

	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3 (small, oversized, small)", len(chunks))
	}
	mid := chunks[1]
	if !mid.Oversized || len(mid.Parts) != 1 || mid.Parts[0].File.Path != "pkg/huge00.go" {
		t.Errorf("middle chunk = %+v, want the huge hunk alone and flagged oversized", mid)
	}
	if mid.Tokens <= c.maxTokens {
		t.Errorf("oversized chunk has %d tokens, want > %d", mid.Tokens, c.maxTokens)
	}
	if chunks[0].Oversized || chunks[2].Oversized {
		t.Error("small neighbors were flagged oversized")
	}
}

func TestChunk_FixtureWithDefaults(t *testing.T) {
	patch, err := os.ReadFile("../diff/testdata/multi.patch")
	if err != nil {
		t.Fatal(err)
	}
	d, err := diff.Parse(bytes.NewReader(patch), diff.Options{})
	if err != nil {
		t.Fatal(err)
	}

	c := newChunker(t, 0)
	chunks := c.Chunk(d.Files)
	checkInvariants(t, c, d.Files, chunks)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	r := chunks[0].Render()
	for _, want := range []string{"### assets/logo.png (added, binary; content omitted)", "### scripts/run.sh (renamed from scripts/run", "+ import \"errors\""} {
		if !strings.Contains(r, want) {
			t.Errorf("rendered chunk missing %q", want)
		}
	}
	for _, excluded := range []string{"go.sum", "yarn.lock", "vendor/", ".pb.go", "_gen.go"} {
		if strings.Contains(r, excluded) {
			t.Errorf("rendered chunk contains excluded path %q", excluded)
		}
	}
}

func TestChunk_Empty(t *testing.T) {
	if got := newChunker(t, 0).Chunk(nil); got != nil {
		t.Errorf("Chunk(nil) = %v, want nil", got)
	}
}

func TestNew_InvalidOptions(t *testing.T) {
	if _, err := New(Options{MaxChunkTokens: -1}); err == nil {
		t.Error("want error for negative MaxChunkTokens")
	}
	if _, err := New(Options{Encoding: "no_such_encoding"}); err == nil {
		t.Error("want error for unknown encoding")
	}
}

// A cache miss must fail (TestMain blocks the network) with a hint, not hang
// or silently download. p50k_base is not loaded by any other test, so the
// process-wide tiktoken encoding cache cannot mask the miss.
func TestNew_CacheMissFailsOffline(t *testing.T) {
	t.Setenv("TIKTOKEN_CACHE_DIR", t.TempDir())
	_, err := New(Options{Encoding: "p50k_base"})
	if err == nil {
		t.Fatal("want error on cache miss with the network blocked")
	}
	if !strings.Contains(err.Error(), "make tiktoken-cache") {
		t.Errorf("error %q lacks the seeding hint", err)
	}
}

func TestCountTokens_Concurrent(t *testing.T) {
	c := newChunker(t, 0)
	want := c.CountTokens("func main() { fmt.Println(\"hello\") }")
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				if got := c.CountTokens("func main() { fmt.Println(\"hello\") }"); got != want {
					t.Errorf("CountTokens = %d, want %d", got, want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
