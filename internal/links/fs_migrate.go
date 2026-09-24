package links

import (
	"sort"
	"strings"

	"github.com/perber/wiki/internal/core/tree"
)

// PageKindLookup reports whether a page exists at route and whether it is a
// section.
type PageKindLookup func(route string) (exists bool, isSection bool)

// convertLegacyDestination converts a legacy link destination (absolute or
// route-relative page link, or absolute/"assets/" asset path) into a
// filesystem-style link. Links to unknown pages, external links, anchors and
// links that are already filesystem-style are returned unchanged.
func convertLegacyDestination(source string, destination string, lookup PageKindLookup) (string, bool) {
	raw := normalizeCandidateDestination(destination)
	base, suffix := splitLinkDestination(raw)
	if base == "" || isExternalLinkDestination(base) || strings.Contains(base, "://") ||
		strings.HasPrefix(strings.ToLower(base), "data:") || strings.HasPrefix(base, "wikilink-") {
		return destination, false
	}
	if IsFilesystemLink(source, raw) {
		return destination, false
	}

	if isAssetLinkDestination(base) {
		return FilesystemAssetLink(source, base) + suffix, true
	}

	route := normalizeWikiPath(base)
	if !strings.HasPrefix(base, "/") {
		resolved, err := resolveURLPath(normalizeWikiPath(source), base)
		if err != nil {
			return destination, false
		}
		route = resolved
	}
	if route == "" || route == "/" {
		return destination, false
	}
	exists, isSection := lookup(route)
	if !exists {
		return destination, false
	}
	return FilesystemPageLink(source, route, isSection) + suffix, true
}

// ConvertToFilesystemLinks rewrites legacy links, images and inline HTML
// src/href attributes in content (located at source, see SourcePath) into
// filesystem-style links. Code spans and blocks are left untouched.
func (e *MarkdownRefactorEngine) ConvertToFilesystemLinks(content string, source string, lookup PageKindLookup) RewriteResult {
	if content == "" {
		return RewriteResult{Content: content}
	}
	excluded := e.collectExcludedRanges(content)

	var replacements []RewriteReplacement
	add := func(start, end int, raw string) {
		if newDest, changed := convertLegacyDestination(source, raw, lookup); changed {
			replacements = append(replacements, RewriteReplacement{Start: start, End: end, NewValue: newDest})
		}
	}
	for _, occ := range scanLinkOccurrences(content, excluded, true) {
		add(occ.Start, occ.End, occ.RawDestination)
	}
	for _, m := range htmlSrcRe.FindAllStringSubmatchIndex(content, -1) {
		if isExcludedOffset(m[0], excluded) {
			continue
		}
		start, end := m[4], m[5]
		if start == -1 {
			start, end = m[6], m[7]
		}
		if start != -1 {
			add(start, end, content[start:end])
		}
	}
	if len(replacements) == 0 {
		return RewriteResult{Content: content}
	}
	sort.SliceStable(replacements, func(i, j int) bool { return replacements[i].Start < replacements[j].Start })
	return RewriteResult{Content: applyReplacements(content, replacements), Replacements: replacements}
}

// MigrateTreeToFilesystemLinks converts every page in ts to filesystem-style
// links and returns the number of pages changed.
func MigrateTreeToFilesystemLinks(ts *tree.TreeService) (int, error) {
	engine := NewMarkdownRefactorEngine()
	// Snapshot route -> kind up front: RewriteAllBodies holds the tree lock.
	kinds := map[string]bool{}
	var ids []string
	_ = ts.WalkNodes(func(id string) error { ids = append(ids, id); return nil })
	for _, id := range ids {
		if n, err := ts.FindPageByID(id); err == nil && n != nil {
			kinds[normalizeWikiPath(n.CalculatePath())] = n.Kind == tree.NodeKindSection
		}
	}
	lookup := func(route string) (bool, bool) {
		isSection, ok := kinds[normalizeWikiPath(route)]
		return ok, isSection
	}
	return ts.RewriteAllBodies(func(node *tree.PageNode, content string) string {
		return engine.ConvertToFilesystemLinks(content, SourcePathForNode(node), lookup).Content
	})
}
