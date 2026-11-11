//go:build ignore

// gen renders the landing-page demo with Vizzini's real code: the annotated
// diff chunk the model receives, and a finding that passes the CUE schema.
//
//	go run ./docs/src/demo/gen.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
	"github.com/infrashift/vizzini-pr-reviewer/internal/schema"
)

const patch = `diff --git a/internal/store/users.go b/internal/store/users.go
index 3f1c2aa..9b7e410 100644
--- a/internal/store/users.go
+++ b/internal/store/users.go
@@ -41,6 +41,7 @@ func (s *Store) FindUser(ctx context.Context, name string) (*User, error) {
 	if name == "" {
 		return nil, ErrEmptyName
 	}
-	row := s.db.QueryRowContext(ctx, "SELECT id, name FROM users WHERE name = $1", name)
+	q := "SELECT id, name FROM users WHERE name = '" + name + "'"
+	row := s.db.QueryRowContext(ctx, q)
 	var u User
 	return &u, row.Scan(&u.ID, &u.Name)
`

const finding = `{
  "summary": "FindUser now builds its SQL by string concatenation instead of a bound parameter.",
  "risk_score": 92,
  "findings": [
    {
      "file": "internal/store/users.go",
      "start_line": 44,
      "end_line": 45,
      "severity": "Critical",
      "category": "Security",
      "title": "SQL injection via string-built query",
      "body": "name is concatenated into the query, so a name like ' OR '1'='1 returns arbitrary rows. The previous version bound it as $1.",
      "suggestion": "row := s.db.QueryRowContext(ctx, \"SELECT id, name FROM users WHERE name = $1\", name)"
    }
  ]
}`

func main() {
	d, err := diff.Parse(strings.NewReader(patch), diff.Options{})
	if err != nil {
		log.Fatal(err)
	}
	var b strings.Builder
	for _, f := range d.Files {
		b.WriteString(f.Header())
		for i := range f.Hunks {
			b.WriteString(f.Hunks[i].Render())
		}
	}

	v, err := schema.New()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := v.ValidateReview([]byte(finding)); err != nil {
		log.Fatalf("demo finding fails the schema: %v", err)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, []byte(finding), "", "  "); err != nil {
		log.Fatal(err)
	}

	dir := filepath.Join("docs", "src", "demo")
	write(filepath.Join(dir, "review-input.txt"), strings.ReplaceAll(b.String(), "\t", "    "))
	write(filepath.Join(dir, "review-output.json"), pretty.String()+"\n")
	fmt.Println("wrote", dir)
}

func write(path, s string) {
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil { //nolint:gosec // G306: public demo fixture
		log.Fatal(err)
	}
}
