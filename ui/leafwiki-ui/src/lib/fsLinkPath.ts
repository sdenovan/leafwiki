import { normalizeWikiRoutePath } from './wikiPath'

/**
 * Filesystem-style link helpers.
 *
 * Mirrors internal/links/fs_links.go. Relative links ending in `.md` or `/`
 * and relative paths into the sibling `assets/` directory are resolved
 * against the directory of the page's *file* on disk, the same way GitHub and
 * other Markdown renderers resolve them:
 *
 *   /root/<route>.md          page
 *   /root/<route>/index.md    section
 *   /assets/<pageID>/<file>   asset
 *
 * All other links keep LeafWiki's legacy route semantics.
 *
 * A "source path" is the page route with a trailing `/` when the page is a
 * section (stored as `index.md`).
 */

const ROOT_DIR = '/root'
const ASSETS_DIR = '/assets'
const INDEX_FILE = 'index.md'

export type FsLinkTarget =
  | { kind: 'none' }
  | { kind: 'asset'; path: string; suffix: string }
  | { kind: 'page'; route: string; sectionForm: boolean; suffix: string }

export function fsSourcePath(route: string, isSection: boolean): string {
  const p = normalizeWikiRoutePath(route)
  if (isSection && p !== '/') return `${p}/`
  return p
}

function sourceIsSection(source: string): boolean {
  return source.length > 1 && source.endsWith('/')
}

function fsSourceDir(source: string): string {
  const route = normalizeWikiRoutePath(source)
  if (route === '/') return `${ROOT_DIR}/`
  if (sourceIsSection(source)) return `${ROOT_DIR}${route}/`
  const idx = route.lastIndexOf('/')
  const dir = idx <= 0 ? '' : route.slice(0, idx)
  return `${ROOT_DIR}${dir}/`
}

function splitSuffix(href: string): [string, string] {
  const idx = href.search(/[?#]/)
  if (idx === -1) return [href, '']
  return [href.slice(0, idx), href.slice(idx)]
}

function isRelativeLocal(base: string): boolean {
  if (!base || base.startsWith('/') || base.startsWith('#')) return false
  if (/^[a-z][a-z0-9+.-]*:/i.test(base)) return false
  if (base.startsWith('assets/')) return false
  return true
}

function safeDecode(s: string): string {
  try {
    return decodeURIComponent(s)
  } catch {
    return s
  }
}

/** Resolves a link with filesystem semantics. */
export function resolveFsHref(source: string, href: string): FsLinkTarget {
  const [base, suffix] = splitSuffix(href.trim())
  if (!isRelativeLocal(base)) return { kind: 'none' }

  let resolved: string
  try {
    resolved = new URL(base, `https://leafwiki.local${fsSourceDir(source)}`)
      .pathname
  } catch {
    return { kind: 'none' }
  }

  if (resolved.startsWith(`${ASSETS_DIR}/`)) {
    return { kind: 'asset', path: resolved, suffix }
  }

  const isMd = base.toLowerCase().endsWith('.md')
  const isDir =
    base.endsWith('/') ||
    base === '.' ||
    base === '..' ||
    base.endsWith('/.') ||
    base.endsWith('/..')
  if (!isMd && !isDir) return { kind: 'none' }

  if (resolved !== ROOT_DIR && !resolved.startsWith(`${ROOT_DIR}/`)) {
    return { kind: 'page', route: '', sectionForm: false, suffix }
  }
  let rest = resolved.slice(ROOT_DIR.length)
  let sectionForm = false
  const last = rest.slice(rest.lastIndexOf('/') + 1)
  if (isMd && last.toLowerCase() === INDEX_FILE) {
    rest = rest.slice(0, rest.length - last.length)
    sectionForm = true
  } else if (isMd) {
    rest = rest.slice(0, -'.md'.length)
  } else {
    sectionForm = true
  }
  let route = normalizeWikiRoutePath(safeDecode(rest))
  if (route === '/') route = ''
  return { kind: 'page', route, sectionForm, suffix }
}

function splitVirtual(p: string): string[] {
  return p.split('/').filter(Boolean)
}

function relativeVirtualPath(dir: string, target: string): string {
  const dirParts = splitVirtual(dir)
  const targetParts = splitVirtual(target)
  let common = 0
  while (
    common < dirParts.length &&
    common < targetParts.length - 1 &&
    dirParts[common] === targetParts[common]
  ) {
    common++
  }
  const parts: string[] = []
  for (let i = common; i < dirParts.length; i++) parts.push('..')
  parts.push(...targetParts.slice(common))
  return parts.join('/')
}

/** Relative `.md` link from the page at `source` to the page at `route`. */
export function fsPageLink(
  source: string,
  route: string,
  targetIsSection: boolean,
): string {
  const r = normalizeWikiRoutePath(route)
  const file = targetIsSection
    ? `${ROOT_DIR}${r}/${INDEX_FILE}`
    : `${ROOT_DIR}${r}.md`
  return relativeVirtualPath(fsSourceDir(source), file)
}

/** Relative link from the page at `source` to `/assets/<id>/<file>`. */
export function fsAssetLink(source: string, assetPath: string): string {
  const [base, suffix] = splitSuffix(assetPath)
  const p = `/${base.replace(/^\/+/, '')}`
  return relativeVirtualPath(fsSourceDir(source), p) + suffix
}

/**
 * Rewrites filesystem-style URLs into the absolute form understood by the
 * rest of the renderer: `/assets/...` for assets and `/<route>` for pages.
 * Other URLs are returned unchanged.
 */
export function toAbsoluteWikiUrl(source: string, url: string): string {
  const target = resolveFsHref(source, url)
  switch (target.kind) {
    case 'asset':
      return target.path + target.suffix
    case 'page':
      return target.route ? target.route + target.suffix : url
    default:
      return url
  }
}
