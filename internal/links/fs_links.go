package links

// Filesystem-style links.
//
// LeafWiki historically resolves relative links with "page is a folder"
// semantics against the page's *route*. That does not match how Markdown is
// rendered anywhere else (GitHub, editors, static site generators), where a
// relative link is resolved against the directory containing the *file*.
//
// This file adds support for filesystem-style links, which are recognised by
// their shape:
//
//   - relative links whose path ends in ".md" (e.g. "../guide/setup.md",
//     "child/index.md") or in "/" (a directory, i.e. a section)
//   - relative links that resolve into the sibling assets directory
//     (e.g. "../../assets/<pageID>/diagram.png")
//
// They are resolved against a virtual filesystem that mirrors the data
// directory on disk:
//
//	/root/<route>.md          page
//	/root/<route>/index.md    section
//	/assets/<pageID>/<file>   asset
//
// Every other link keeps the legacy route semantics, so existing content is
// unaffected.
//
// Because the source file location depends on the node kind, functions in
// this package accept a "source path" produced by SourcePath: the page route
// with a trailing "/" when the page is a section (stored as index.md).

import (
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/perber/wiki/internal/core/tree"
)

const (
	fsRootDir   = "/root"
	fsAssetsDir = "/assets"
	fsIndexFile = "index.md"
)

type fsTargetKind int

const (
	fsTargetNone fsTargetKind = iota
	fsTargetPage
	fsTargetAsset
)

// fsTarget is the result of resolving a filesystem-style link.
type fsTarget struct {
	Kind fsTargetKind
	// Route is the normalized wiki route ("/a/b") for pages or the virtual
	// asset path ("/assets/<id>/<file>") for assets. Empty when the link
	// points outside the wiki.
	Route string
	// SectionForm reports whether the link addressed the page as a section
	// ("x/index.md" or "x/") rather than as a flat page ("x.md").
	SectionForm bool
}

// SourcePath returns the encoded source path used for link resolution.
func SourcePath(route string, kind tree.NodeKind) string {
	p := normalizeWikiPath(route)
	if p == "" {
		p = "/"
	}
	if kind == tree.NodeKindSection && p != "/" {
		return p + "/"
	}
	return p
}

// SourcePathForNode returns SourcePath for a tree node.
func SourcePathForNode(node *tree.PageNode) string {
	if node == nil {
		return ""
	}
	return SourcePath(node.CalculatePath(), node.Kind)
}

func sourceIsSection(source string) bool {
	return len(source) > 1 && strings.HasSuffix(source, "/")
}

// withSourceRoute replaces the route of an encoded source path, keeping its kind.
func withSourceRoute(source string, route string) string {
	kind := tree.NodeKindPage
	if sourceIsSection(source) {
		kind = tree.NodeKindSection
	}
	return SourcePath(route, kind)
}

// fsSourceDir returns the virtual directory (with trailing "/") containing the
// source file.
func fsSourceDir(source string) string {
	route := normalizeWikiPath(source)
	if route == "" || route == "/" {
		return fsRootDir + "/"
	}
	if sourceIsSection(source) {
		return fsRootDir + route + "/"
	}
	dir := path.Dir(route)
	if dir == "/" {
		return fsRootDir + "/"
	}
	return fsRootDir + dir + "/"
}

// fsTargetFile returns the virtual file for a page route.
func fsTargetFile(route string, section bool) string {
	route = normalizeWikiPath(route)
	if section {
		return fsRootDir + route + "/" + fsIndexFile
	}
	return fsRootDir + route + ".md"
}

func isLegacyRelativeAsset(base string) bool {
	return strings.HasPrefix(base, "assets/")
}

// isRelativeLocalHref reports whether base (fragment/query stripped) is a
// relative, non-external link that might be filesystem-style.
func isRelativeLocalHref(base string) bool {
	if base == "" || strings.HasPrefix(base, "/") || isExternalLinkDestination(base) {
		return false
	}
	if strings.Contains(base, "://") || strings.HasPrefix(strings.ToLower(base), "data:") {
		return false
	}
	return !isLegacyRelativeAsset(base)
}

// resolveFSHref resolves a link destination with filesystem semantics.
// It returns Kind == fsTargetNone when the link is not filesystem-style.
func resolveFSHref(source string, destination string) fsTarget {
	base, _ := splitLinkDestination(normalizeCandidateDestination(destination))
	if !isRelativeLocalHref(base) {
		return fsTarget{}
	}

	ref, err := url.Parse(base)
	if err != nil {
		return fsTarget{}
	}
	dirURL, err := url.Parse("https://leafwiki.local" + fsSourceDir(source))
	if err != nil {
		return fsTarget{}
	}
	resolved := dirURL.ResolveReference(ref).Path

	if strings.HasPrefix(resolved, fsAssetsDir+"/") {
		return fsTarget{Kind: fsTargetAsset, Route: resolved}
	}

	lower := strings.ToLower(base)
	isMd := strings.HasSuffix(lower, ".md")
	isDir := strings.HasSuffix(base, "/") || base == "." || base == ".." || strings.HasSuffix(base, "/.") || strings.HasSuffix(base, "/..")
	if !isMd && !isDir {
		return fsTarget{}
	}

	if resolved != fsRootDir && !strings.HasPrefix(resolved, fsRootDir+"/") {
		// Points outside of the wiki pages directory.
		return fsTarget{Kind: fsTargetPage}
	}
	rest := strings.TrimPrefix(resolved, fsRootDir)
	section := false
	switch {
	case isMd && strings.EqualFold(path.Base(rest), fsIndexFile):
		rest = path.Dir(rest)
		section = true
	case isMd:
		rest = rest[:len(rest)-len(".md")]
	default:
		section = true
	}
	route := normalizeWikiPath(rest)
	if route == "/" {
		// The wiki root has no addressable page.
		route = ""
	}
	return fsTarget{Kind: fsTargetPage, Route: route, SectionForm: section}
}

// IsFilesystemLink reports whether destination is a filesystem-style link
// (a relative ".md" link, directory link, or relative asset path).
func IsFilesystemLink(source string, destination string) bool {
	return resolveFSHref(source, destination).Kind != fsTargetNone
}

// relativeVirtualPath returns the relative path from directory dir (with
// trailing "/") to file target.
func relativeVirtualPath(dir string, target string) string {
	dirParts := splitVirtual(dir)
	targetParts := splitVirtual(target)
	common := 0
	for common < len(dirParts) && common < len(targetParts)-1 && dirParts[common] == targetParts[common] {
		common++
	}
	parts := make([]string, 0, len(dirParts)-common+len(targetParts)-common)
	for i := common; i < len(dirParts); i++ {
		parts = append(parts, "..")
	}
	parts = append(parts, targetParts[common:]...)
	return strings.Join(parts, "/")
}

func splitVirtual(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// FilesystemPageLink returns the relative filesystem-style link from the page
// at source (see SourcePath) to the page at targetRoute.
func FilesystemPageLink(source string, targetRoute string, targetIsSection bool) string {
	return relativeVirtualPath(fsSourceDir(source), fsTargetFile(targetRoute, targetIsSection))
}

// FilesystemAssetLink returns the relative link from the page at source to an
// asset URL of the form "/assets/<pageID>/<file>".
func FilesystemAssetLink(source string, assetPath string) string {
	assetPath = "/" + strings.TrimPrefix(assetPath, "/")
	return relativeVirtualPath(fsSourceDir(source), assetPath)
}

// rewriteFSDestination rewrites a filesystem-style destination written at
// oldSource so that it is valid at newSource, after applying rename rules to
// the target route and forcing the file form of routes in kindOverrides
// (route -> isSection). ok is false when destination is not filesystem-style.
func rewriteFSDestination(oldSource, newSource, destination string, rules []RewriteRule, kindOverrides map[string]bool) (newDest string, changed bool, ok bool) {
	target := resolveFSHref(oldSource, destination)
	if target.Kind == fsTargetNone {
		return destination, false, false
	}
	_, suffix := splitLinkDestination(destination)

	var rewritten string
	switch target.Kind {
	case fsTargetAsset:
		rewritten = FilesystemAssetLink(newSource, target.Route)
	case fsTargetPage:
		if target.Route == "" {
			return destination, false, true
		}
		route := target.Route
		if r, hit := applyRewriteRules(route, rules); hit {
			route = r
		}
		section := target.SectionForm
		if override, hit := kindOverrides[target.Route]; hit {
			section = override
		}
		rewritten = FilesystemPageLink(newSource, route, section)
		// Preserve a directory-style link ("x/") when the target is still a section.
		if section && !strings.HasSuffix(strings.ToLower(stripSuffix(destination)), ".md") {
			rewritten = strings.TrimSuffix(rewritten, fsIndexFile)
			if rewritten == "" {
				rewritten = "./"
			}
		}
	}
	if rewritten == "" {
		return destination, false, true
	}

	// Avoid churn when the destination is already equivalent.
	oldBase := stripSuffix(normalizeCandidateDestination(destination))
	if rewritten == oldBase || "./"+rewritten == oldBase {
		return destination, false, true
	}
	return rewritten + suffix, true, true
}

func stripSuffix(destination string) string {
	base, _ := splitLinkDestination(destination)
	return base
}

// htmlSrcRe matches src="..." / src='...' / href="..." attributes in inline HTML.
var htmlSrcRe = regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*("([^"]*)"|'([^']*)')`)

// RewriteFilesystemLinks rewrites filesystem-style links (".md" page links,
// directory links and relative asset paths) in content that was written at
// oldSource so that they are correct when the content lives at newSource.
//
// Rename rules are applied to page targets, and kindOverrides (route ->
// isSection) forces the file form (x.md vs x/index.md) for specific targets.
// scope selects plain links, media (images and inline HTML src/href
// attributes), or both. Legacy route links are never touched.
func (e *MarkdownRefactorEngine) RewriteFilesystemLinks(content, oldSource, newSource string, rules []RewriteRule, kindOverrides map[string]bool, scope FSRewriteScope) RewriteResult {
	if content == "" {
		return RewriteResult{Content: content}
	}
	if !strings.Contains(content, ".md") && !strings.Contains(content, "/") {
		return RewriteResult{Content: content}
	}

	excluded := e.collectExcludedRanges(content)
	includeMedia := scope&FSScopeMedia != 0
	includeLinks := scope&FSScopeLinks != 0
	occurrences := scanLinkOccurrences(content, excluded, includeMedia)

	var replacements []RewriteReplacement
	add := func(start, end int, raw string) {
		newDest, changed, _ := rewriteFSDestination(oldSource, newSource, raw, rules, kindOverrides)
		if changed {
			replacements = append(replacements, RewriteReplacement{Start: start, End: end, NewValue: newDest})
		}
	}
	for _, occ := range occurrences {
		if (occ.IsImage && includeMedia) || (!occ.IsImage && includeLinks) {
			add(occ.Start, occ.End, occ.RawDestination)
		}
	}

	if includeMedia {
		for _, m := range htmlSrcRe.FindAllStringSubmatchIndex(content, -1) {
			if isExcludedOffset(m[0], excluded) {
				continue
			}
			start, end := m[4], m[5]
			if start == -1 {
				start, end = m[6], m[7]
			}
			if start == -1 {
				continue
			}
			add(start, end, content[start:end])
		}
	}

	if len(replacements) == 0 {
		return RewriteResult{Content: content}
	}
	sort.SliceStable(replacements, func(i, j int) bool { return replacements[i].Start < replacements[j].Start })
	return RewriteResult{
		Content:      applyReplacements(content, replacements),
		Replacements: replacements,
	}
}

// RewriteForKindChange rewrites content after the page at route changed kind
// (page <-> section). self reports whether content belongs to the converted
// page itself; in that case the source location changes too.
func (e *MarkdownRefactorEngine) RewriteForKindChange(content string, source string, route string, nowSection bool, self bool) string {
	route = normalizeWikiPath(route)
	overrides := map[string]bool{route: nowSection}
	oldSource, newSource := source, source
	if self {
		oldKind, newKind := tree.NodeKindSection, tree.NodeKindPage
		if nowSection {
			oldKind, newKind = tree.NodeKindPage, tree.NodeKindSection
		}
		oldSource = SourcePath(route, oldKind)
		newSource = SourcePath(route, newKind)
	}
	scope := FSScopeLinks
	if self {
		scope = FSScopeAll
	}
	return e.RewriteFilesystemLinks(content, oldSource, newSource, nil, overrides, scope).Content
}

// FSRewriteScope selects which occurrences RewriteFilesystemLinks rewrites.
type FSRewriteScope int

const (
	FSScopeLinks FSRewriteScope = 1 << iota
	FSScopeMedia
	FSScopeAll = FSScopeLinks | FSScopeMedia
)

// scanLinkOccurrences is scanInlineLinkOccurrences with optional image support.
func scanLinkOccurrences(content string, excluded []textRange, includeImages bool) []inlineLinkOccurrence {
	if !includeImages {
		return scanInlineLinkOccurrences(content, excluded)
	}
	var occurrences []inlineLinkOccurrence
	for i := 0; i < len(content); i++ {
		if isExcludedOffset(i, excluded) || content[i] != '[' {
			continue
		}
		labelEnd := findClosingBracket(content, i)
		if labelEnd == -1 {
			continue
		}
		j := labelEnd + 1
		if j >= len(content) || content[j] != '(' {
			continue
		}
		occurrence, ok := parseInlineLinkOccurrence(content, j+1)
		if !ok {
			continue
		}
		// Do not skip past the label: images may be nested inside link labels.
		occurrence.IsImage = i > 0 && content[i-1] == '!'
		occurrences = append(occurrences, occurrence)
	}
	return occurrences
}

// kindChangeRewriter implements tree.KindChangeRewriter for filesystem-style
// links.
type kindChangeRewriter struct {
	store  *LinksStore
	engine *MarkdownRefactorEngine
}

// KindChangeRewriter returns a tree.KindChangeRewriter that keeps
// filesystem-style links valid when pages are converted between flat pages
// and sections.
func (b *LinkService) KindChangeRewriter() tree.KindChangeRewriter {
	return &kindChangeRewriter{store: b.store, engine: NewMarkdownRefactorEngine()}
}

func (k *kindChangeRewriter) RewriteSelf(node *tree.PageNode, _ tree.NodeKind, content string) string {
	return k.engine.RewriteForKindChange(content, "", node.CalculatePath(), node.Kind == tree.NodeKindSection, true)
}

func (k *kindChangeRewriter) RewriteIncoming(source *tree.PageNode, target *tree.PageNode, content string) string {
	return k.engine.RewriteForKindChange(content, SourcePathForNode(source), target.CalculatePath(), target.Kind == tree.NodeKindSection, false)
}

func (k *kindChangeRewriter) IncomingSourceIDs(targetID string) ([]string, bool) {
	if k.store == nil {
		return nil, false
	}
	backlinks, err := k.store.GetBacklinksForPage(targetID)
	if err != nil {
		return nil, false
	}
	seen := make(map[string]struct{}, len(backlinks))
	ids := make([]string, 0, len(backlinks))
	for _, bl := range backlinks {
		if _, dup := seen[bl.FromPageID]; dup {
			continue
		}
		seen[bl.FromPageID] = struct{}{}
		ids = append(ids, bl.FromPageID)
	}
	return ids, true
}
