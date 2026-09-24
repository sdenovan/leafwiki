package tree

import (
	"github.com/perber/wiki/internal/core/markdown"
)

// KindChangeRewriter keeps page content consistent when a node's on-disk
// representation changes between a flat page (<slug>.md) and a section
// (<slug>/index.md). Filesystem-style relative links depend on the file
// location, so both the converted page and pages linking to it may need
// their links rewritten.
//
// All methods are called while the tree write lock is held and therefore
// must not call back into TreeService.
type KindChangeRewriter interface {
	// RewriteSelf returns the rewritten content of node after it changed
	// from oldKind to node.Kind.
	RewriteSelf(node *PageNode, oldKind NodeKind, content string) string
	// RewriteIncoming returns the rewritten content of source, which may
	// link to target (whose kind just changed).
	RewriteIncoming(source *PageNode, target *PageNode, content string) string
	// IncomingSourceIDs returns the IDs of pages that link to target. ok is
	// false when the information is unavailable and all pages must be
	// scanned.
	IncomingSourceIDs(targetID string) (ids []string, ok bool)
}

// SetKindChangeRewriter installs the rewriter invoked after page <-> section
// conversions. Passing nil disables it.
func (t *TreeService) SetKindChangeRewriter(r KindChangeRewriter) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.kindRewriter = r
}

// convertNodeLocked converts node on disk, updates its kind and runs the
// kind-change rewriter. Lock must be held by the caller.
func (t *TreeService) convertNodeLocked(node *PageNode, kind NodeKind) error {
	oldKind := node.Kind
	if err := t.store.ConvertNode(node, kind); err != nil {
		return err
	}
	node.Kind = kind
	if oldKind != kind {
		t.afterKindChangeLocked(node, oldKind)
	}
	return nil
}

func (t *TreeService) afterKindChangeLocked(node *PageNode, oldKind NodeKind) {
	r := t.kindRewriter
	if r == nil {
		return
	}

	if _, err := t.store.RewriteBody(node, func(content string) string {
		return r.RewriteSelf(node, oldKind, content)
	}); err != nil {
		t.log.Warn("could not rewrite links of converted page", "pageID", node.ID, "error", err)
	}

	var sources []*PageNode
	if ids, ok := r.IncomingSourceIDs(node.ID); ok {
		for _, id := range ids {
			if src := t.getNodeByIDLocked(id); src != nil {
				sources = append(sources, src)
			}
		}
	} else {
		for _, src := range t.nodesByID {
			sources = append(sources, src)
		}
	}

	for _, src := range sources {
		if src == nil || src.ID == node.ID || src.ID == "root" {
			continue
		}
		if _, err := t.store.RewriteBody(src, func(content string) string {
			return r.RewriteIncoming(src, node, content)
		}); err != nil {
			t.log.Warn("could not rewrite incoming links after kind change", "sourceID", src.ID, "targetID", node.ID, "error", err)
		}
	}
}

// RewriteAllBodies applies fn to the markdown body of every node that has a
// content file and returns the number of files changed. Intended for offline
// bulk migrations.
func (t *TreeService) RewriteAllBodies(fn func(node *PageNode, content string) string) (int, error) {
	changed := 0
	err := t.withLockedTree(func() error {
		if t.tree == nil {
			return ErrTreeNotLoaded
		}
		for _, node := range t.nodesByID {
			if node == nil || node.ID == "root" {
				continue
			}
			n := node
			ok, err := t.store.RewriteBody(n, func(c string) string { return fn(n, c) })
			if err != nil {
				t.log.Warn("could not rewrite page body", "pageID", n.ID, "error", err)
				continue
			}
			if ok {
				changed++
			}
		}
		return nil
	})
	return changed, err
}

// RewriteBody rewrites the markdown body of entry (frontmatter untouched)
// with fn. It reports whether the file changed. Missing files are ignored.
func (f *NodeStore) RewriteBody(entry *PageNode, fn func(string) string) (bool, error) {
	filePath, err := f.contentPathForNodeRead(entry)
	if err != nil {
		return false, err
	}
	if !fileExists(filePath) {
		return false, nil
	}
	mdFile, err := markdown.LoadMarkdownFile(filePath)
	if err != nil {
		return false, err
	}
	content := mdFile.GetContent()
	updated := fn(content)
	if updated == content {
		return false, nil
	}
	mdFile.SetContent(updated)
	if err := mdFile.WriteToFile(); err != nil {
		return false, err
	}
	return true, nil
}
