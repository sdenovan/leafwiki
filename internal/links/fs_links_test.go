package links

import (
	"testing"

	"github.com/perber/wiki/internal/core/tree"
)

func TestSourcePath(t *testing.T) {
	cases := []struct {
		route string
		kind  tree.NodeKind
		want  string
	}{
		{"a/b", tree.NodeKindPage, "/a/b"},
		{"a/b", tree.NodeKindSection, "/a/b/"},
		{"", tree.NodeKindSection, "/"},
		{"/x/", tree.NodeKindPage, "/x"},
	}
	for _, c := range cases {
		if got := SourcePath(c.route, c.kind); got != c.want {
			t.Errorf("SourcePath(%q,%q)=%q want %q", c.route, c.kind, got, c.want)
		}
	}
}

func TestResolveFSHref(t *testing.T) {
	cases := []struct {
		source, href string
		kind         fsTargetKind
		route        string
		section      bool
	}{
		// page /x/a stored as root/x/a.md
		{"/x/a", "b.md", fsTargetPage, "/x/b", false},
		{"/x/a", "./b.md#frag", fsTargetPage, "/x/b", false},
		{"/x/a", "../top.md", fsTargetPage, "/top", false},
		{"/x/a", "sub/index.md", fsTargetPage, "/x/sub", true},
		{"/x/a", "sub/", fsTargetPage, "/x/sub", true},
		{"/x/a", "../x/", fsTargetPage, "/x", true},
		{"/x/a", "../../assets/id1/p.png", fsTargetAsset, "/assets/id1/p.png", false},
		{"/x/a", "../../../outside.md", fsTargetPage, "", false},
		// section /x/a stored as root/x/a/index.md
		{"/x/a/", "b.md", fsTargetPage, "/x/a/b", false},
		{"/x/a/", "../b.md", fsTargetPage, "/x/b", false},
		{"/x/a/", "../../../assets/id1/p.png", fsTargetAsset, "/assets/id1/p.png", false},
		// root-level page
		{"/a", "../assets/id/f.pdf", fsTargetAsset, "/assets/id/f.pdf", false},
		{"/a", "Guide%20One.md", fsTargetPage, "/Guide One", false},
		// legacy links are left alone
		{"/x/a", "b", fsTargetNone, "", false},
		{"/x/a", "../b", fsTargetNone, "", false},
		{"/x/a", "/x/b.md", fsTargetNone, "", false},
		{"/x/a", "assets/id/p.png", fsTargetNone, "", false},
		{"/x/a", "https://example.com/a.md", fsTargetNone, "", false},
		{"/x/a", "#section", fsTargetNone, "", false},
	}
	for _, c := range cases {
		got := resolveFSHref(c.source, c.href)
		if got.Kind != c.kind || got.Route != c.route || got.SectionForm != c.section {
			t.Errorf("resolveFSHref(%q,%q)=%+v want kind=%v route=%q section=%v", c.source, c.href, got, c.kind, c.route, c.section)
		}
	}
}

func TestFilesystemLinkGeneration(t *testing.T) {
	cases := []struct {
		source, route string
		section       bool
		want          string
	}{
		{"/x/a", "/x/b", false, "b.md"},
		{"/x/a", "/x/b", true, "b/index.md"},
		{"/x/a", "/top", false, "../top.md"},
		{"/x/a/", "/x/a/c", false, "c.md"},
		{"/x/a/", "/x/a", true, "index.md"},
		{"/x/a", "/x/a", false, "a.md"},
		{"/a", "/b/c", false, "b/c.md"},
		{"/", "/b", false, "b.md"},
	}
	for _, c := range cases {
		if got := FilesystemPageLink(c.source, c.route, c.section); got != c.want {
			t.Errorf("FilesystemPageLink(%q,%q,%v)=%q want %q", c.source, c.route, c.section, got, c.want)
		}
		// round trip
		back := resolveFSHref(c.source, FilesystemPageLink(c.source, c.route, c.section))
		if back.Route != c.route {
			t.Errorf("round trip %q -> %q", c.route, back.Route)
		}
	}

	if got := FilesystemAssetLink("/x/a", "/assets/id/p.png"); got != "../../assets/id/p.png" {
		t.Errorf("asset from page: %q", got)
	}
	if got := FilesystemAssetLink("/x/a/", "/assets/id/p.png"); got != "../../../assets/id/p.png" {
		t.Errorf("asset from section: %q", got)
	}
	if got := FilesystemAssetLink("/a", "assets/id/p.png"); got != "../assets/id/p.png" {
		t.Errorf("asset from root page: %q", got)
	}
}

func TestRewriteForKindChange_Self(t *testing.T) {
	e := NewMarkdownRefactorEngine()
	content := "[sib](b.md) [legacy](../b) [self](a.md)\n\n![img](../../assets/id/p.png)\n\n<img src=\"../../assets/id/q.png\">\n\n`[code](b.md)`\n"
	got := e.RewriteForKindChange(content, "", "/x/a", true, true)
	want := "[sib](../b.md) [legacy](../b) [self](index.md)\n\n![img](../../../assets/id/p.png)\n\n<img src=\"../../../assets/id/q.png\">\n\n`[code](b.md)`\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	// and back again
	if back := e.RewriteForKindChange(got, "", "/x/a", false, true); back != content {
		t.Fatalf("fold back mismatch:\n%s", back)
	}
}

func TestRewriteForKindChange_Incoming(t *testing.T) {
	e := NewMarkdownRefactorEngine()
	content := "[a](a.md#top) [other](c.md) [dir](a/) ![i](../../assets/id/p.png)"
	got := e.RewriteForKindChange(content, "/x/b", "/x/a", true, false)
	want := "[a](a/index.md#top) [other](c.md) [dir](a/) ![i](../../assets/id/p.png)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	back := e.RewriteForKindChange(got, "/x/b", "/x/a", false, false)
	if back != "[a](a.md#top) [other](c.md) [dir](a.md) ![i](../../assets/id/p.png)" {
		t.Fatalf("back %q", back)
	}
}

func TestRewrite_FilesystemLinksOnRename(t *testing.T) {
	e := NewMarkdownRefactorEngine()
	rules := []RewriteRule{{OldPath: "/x/b", NewPath: "/y/renamed"}}
	got := e.Rewrite("[b](b.md) [legacy](../b)", "/x/a", rules).Content
	want := "[b](../y/renamed.md) [legacy](../../y/renamed)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRewriteRelativeLinksForPathChange_Filesystem(t *testing.T) {
	e := NewMarkdownRefactorEngine()
	content := "[b](b.md) ![p](../../assets/id/p.png)"
	rel := e.RewriteRelativeLinksForPathChange(content, "/x/a", "/z/q/a", nil).Content
	rel = e.RewriteFilesystemLinks(rel, "/x/a", "/z/q/a", nil, nil, FSScopeMedia).Content
	want := "[b](../../x/b.md) ![p](../../../assets/id/p.png)"
	if rel != want {
		t.Fatalf("got %q want %q", rel, want)
	}
}

func TestExtractAndResolveSkipsFSAssets(t *testing.T) {
	links := extractLinksFromMarkdown("[doc](../../assets/id/doc.pdf) [p](b.md)")
	if len(links) != 2 {
		t.Fatalf("links: %v", links)
	}
	if resolveFSHref("/x/a", links[0]).Kind != fsTargetAsset {
		t.Fatalf("expected asset")
	}
}

func TestKindChangeRewriter_EndToEnd(t *testing.T) {
	svc, ts, _ := setupLinkService(t)
	ts.SetKindChangeRewriter(svc.KindChangeRewriter())

	update := func(id, content string) {
		t.Helper()
		p, err := ts.GetPage(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := ts.UpdateNode("system", p.ID, p.Title, p.Slug, &content, tree.VersionUnchecked, nil, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	content := func(id string) string {
		t.Helper()
		p, err := ts.GetPage(id)
		if err != nil {
			t.Fatal(err)
		}
		return p.Content
	}

	aID, _ := ts.CreateNode("system", nil, "A", "a", pageNodeKind())
	bID, _ := ts.CreateNode("system", nil, "B", "b", pageNodeKind())
	update(*aID, "[b](b.md) ![p](../assets/x/p.png)")
	update(*bID, "[a](a.md)")
	if err := svc.IndexAllPages(); err != nil {
		t.Fatal(err)
	}

	// Adding a child converts a.md -> a/index.md.
	cID, err := ts.CreateNode("system", aID, "C", "c", pageNodeKind())
	if err != nil {
		t.Fatal(err)
	}
	if got := content(*aID); got != "[b](../b.md) ![p](../../assets/x/p.png)" {
		t.Fatalf("self after convert: %q", got)
	}
	if got := content(*bID); got != "[a](a/index.md)" {
		t.Fatalf("incoming after convert: %q", got)
	}

	// Link index still resolves.
	out, err := svc.GetOutgoingLinksForPage(*bID)
	if err != nil || out.Count != 1 || out.Outgoings[0].Broken {
		t.Fatalf("outgoing b: %+v %v", out, err)
	}

	// Removing the last child and converting back folds a/index.md -> a.md.
	if err := ts.DeleteNode("system", *cID, false, tree.VersionUnchecked); err != nil {
		t.Fatal(err)
	}
	if err := ts.ConvertNode("system", *aID, tree.NodeKindPage, tree.VersionUnchecked); err != nil {
		t.Fatal(err)
	}
	if got := content(*aID); got != "[b](b.md) ![p](../assets/x/p.png)" {
		t.Fatalf("self after fold: %q", got)
	}
	if got := content(*bID); got != "[a](a.md)" {
		t.Fatalf("incoming after fold: %q", got)
	}
}

func TestMigrateTreeToFilesystemLinks(t *testing.T) {
	_, ts, _ := setupLinkService(t)
	aID, _ := ts.CreateNode("system", nil, "A", "a", pageNodeKind())
	sID, _ := ts.CreateNode("system", nil, "S", "s", pageNodeKind())
	cID, _ := ts.CreateNode("system", sID, "C", "c", pageNodeKind())

	set := func(id, c string) {
		p, _ := ts.GetPage(id)
		if err := ts.UpdateNode("system", p.ID, p.Title, p.Slug, &c, tree.VersionUnchecked, nil, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	set(*aID, "[s](/s) [c](/s/c#h) [missing](/nope) ![i](/assets/"+*aID+"/p.png) [ext](https://x.io) `[code](/s)`")
	set(*cID, "[up](../../a) [sib](/s) <img src=\"assets/x/q.png\">")

	n, err := MigrateTreeToFilesystemLinks(ts)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	a, _ := ts.GetPage(*aID)
	wantA := "[s](s/index.md) [c](s/c.md#h) [missing](/nope) ![i](../assets/" + *aID + "/p.png) [ext](https://x.io) `[code](/s)`"
	if a.Content != wantA {
		t.Fatalf("a: %q", a.Content)
	}
	c, _ := ts.GetPage(*cID)
	if c.Content != "[up](../a.md) [sib](index.md) <img src=\"../../assets/x/q.png\">" {
		t.Fatalf("c: %q", c.Content)
	}
	// idempotent
	if n, _ := MigrateTreeToFilesystemLinks(ts); n != 0 {
		t.Fatalf("second run changed %d", n)
	}
}
