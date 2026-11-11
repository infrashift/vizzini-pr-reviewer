package okf

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/infrashift/vizzini-pr-reviewer/internal/diff"
)

// filePatch is a one-hunk diff of path whose hunk holds lines (each already
// prefixed with ' ', '+' or '-').
func filePatch(path string, lines ...string) string {
	oldN, newN := 0, 0
	for _, l := range lines {
		switch l[0] {
		case ' ':
			oldN++
			newN++
		case '-':
			oldN++
		case '+':
			newN++
		}
	}
	return "diff --git a/" + path + " b/" + path + "\n--- a/" + path + "\n+++ b/" + path +
		"\n@@ -1," + strconv.Itoa(oldN) + " +1," + strconv.Itoa(newN) + " @@\n" + strings.Join(lines, "\n") + "\n"
}

// ISSUE-021: the declared package or namespace of each language, as a
// dot-separated name; files that declare none keep an empty Package.
func TestTargetsFor_Namespaces(t *testing.T) {
	cases := []struct {
		path  string
		lines []string
		want  string
	}{
		{"pkg/db/users.go", []string{" package store", "+// x"}, "store"},
		{"src/main/java/com/acme/store/Users.java", []string{"+package com.acme.store;", "+class Users {}"}, "com.acme.store"},
		{"app/src/Billing.kt", []string{" package com.acme.billing", "+fun bill() {}"}, "com.acme.billing"},
		{"src/Main.scala", []string{"+package acme.orders", "+object Main"}, "acme.orders"},
		{"src/Repositories/OrderRepository.cs", []string{"+namespace Shop.Repositories;", "+public class R {}"}, "Shop.Repositories"},
		{"src/Legacy.cs", []string{" namespace Shop.Legacy {", "+  class A {}", " }", " namespace Shop.Other { }"}, "Shop.Legacy"},
		{"app/Http/Kernel.php", []string{"+<?php", "+namespace App\\Http;"}, "App.Http"},
		{"src/geo.cpp", []string{"+inline namespace geo::v2 {", "+int x;"}, "geo.v2"},
		{"src/billing.ts", []string{"+export namespace Billing.Invoices {", "+  export const x = 1;"}, "Billing.Invoices"},
		{"src/legacy.ts", []string{" declare namespace Legacy {", "+  const y: number;"}, "Legacy"},
		{"src/types.d.ts", []string{` declare module "express" {`, "+  interface Request {}"}, ""},
		{"app.py", []string{"+import os", "+package = 1"}, ""},
		{"web/app.js", []string{"+namespace = {}"}, ""},
		{"src/Mixed.cs", []string{"+// package com.not.this", "+var x = 1;"}, ""},
	}
	var patch strings.Builder
	for _, c := range cases {
		patch.WriteString(filePatch(c.path, c.lines...))
	}
	d, err := diff.Parse(strings.NewReader(patch.String()), diff.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := TargetsFor(d.Files)
	if len(got) != len(cases) {
		t.Fatalf("%d targets for %d files", len(got), len(cases))
	}
	for i, c := range cases {
		if got[i].Path != c.path || got[i].Package != c.want {
			t.Errorf("%s: Package %q, want %q", c.path, got[i].Package, c.want)
		}
	}
}

func TestMatch_Namespaces(t *testing.T) {
	b := &Bundle{Docs: []Doc{
		{ID: "segment", Path: "a.md", Domains: []string{"Repositories"}},
		{ID: "qualified", Path: "b.md", Domains: []string{"Shop.Repositories"}},
		{ID: "colons", Path: "c.md", Domains: []string{"Shop::Repositories"}},
		{ID: "backslash", Path: "d.md", Domains: []string{`App\Http`}},
		{ID: "slashes", Path: "e.md", Domains: []string{"com/acme/store"}},
		{ID: "tagged", Path: "f.md", Tags: []string{"shop.repositories"}},
		{ID: "java-store", Path: "g.md", Domains: []string{"store"}},
	}}
	ids := func(ts ...Target) []string {
		ms := b.Match(ts)
		out := make([]string, 0, len(ms))
		for _, m := range ms {
			out = append(out, m.Doc.ID+"/"+m.Reason)
		}
		return out
	}
	for name, tc := range map[string]struct {
		target Target
		want   []string
	}{
		"C# namespace: segment, qualified and :: spellings, tag last": {
			Target{Path: "src/Data/OrderRepository.cs", Package: "Shop.Repositories"},
			[]string{"segment/domain", "qualified/domain", "colons/domain", "tagged/tags"},
		},
		"nested namespace matches a qualified domain": {
			Target{Path: "src/Sql.cs", Package: "Shop.Repositories.Sql"},
			[]string{"segment/domain", "qualified/domain", "colons/domain", "tagged/tags"},
		},
		"sibling namespace with a shared prefix does not": {
			Target{Path: "src/Repo.cs", Package: "Shop.Repo"},
			nil,
		},
		"parent namespace does not match the child domain": {
			Target{Path: "src/Shop.cs", Package: "Shop"},
			nil,
		},
		"PHP namespace with a backslash domain": {
			Target{Path: "app/Kernel.php", Package: "App.Http"},
			[]string{"backslash/domain"},
		},
		"Java package: slash domain and a single segment": {
			Target{Path: "src/Users.java", Package: "com.acme.store"},
			[]string{"slashes/domain", "java-store/domain"},
		},
		"directories still match without a namespace": {
			Target{Path: "src/Repositories/x.ts"},
			[]string{"segment/domain"},
		},
	} {
		got := ids(tc.target)
		slicesSort(got)
		want := append([]string(nil), tc.want...)
		slicesSort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
}

func slicesSort(s []string) { slices.Sort(s) }
