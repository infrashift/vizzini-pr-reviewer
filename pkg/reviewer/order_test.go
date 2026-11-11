package reviewer

import (
	"strconv"
	"strings"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func TestOrderFilesByDependency(t *testing.T) {
	add := func(path, body string) string {
		lines := strings.Split(body, "\n")
		var b strings.Builder
		b.WriteString("diff --git a/" + path + " b/" + path + "\nnew file mode 100644\n--- /dev/null\n+++ b/" + path + "\n")
		b.WriteString("@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@\n")
		for _, l := range lines {
			b.WriteString("+" + l + "\n")
		}
		return b.String()
	}
	patch := add("ui/view.go", "package ui\nimport \"example.com/x/service\"\nvar _ service.S") +
		add("service/s.go", "package service\nimport \"example.com/x/model\"\ntype S struct{ m model.M }") +
		add("model/m.go", "package model\ntype M struct{}")
	files, _, err := gitdiff.Parse(strings.NewReader(patch))
	if err != nil {
		t.Fatal(err)
	}
	got := OrderFilesByDependency(files)
	names := make([]string, 0, len(got))
	for _, f := range got {
		names = append(names, f.NewName)
	}
	if strings.Join(names, " ") != "model/m.go service/s.go ui/view.go" {
		t.Errorf("order %v", names)
	}
}
