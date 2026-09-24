import { useConfigStore } from '@/stores/config'
import { useTreeStore } from '@/stores/tree'
import { fsAssetLink, fsPageLink, fsSourcePath } from './fsLinkPath'
import { buildViewUrl, stripBasePath } from './routePath'
import { normalizeWikiRoutePath, toWikiLookupPath } from './wikiPath'

/**
 * Editor-side helpers that decide which link format to insert.
 *
 * With `--filesystem-links` enabled the editor inserts relative `.md` page
 * links and relative asset paths, so the Markdown on disk also renders on
 * GitHub. Otherwise the legacy absolute formats are kept.
 */

export function isFilesystemLinksEnabled(): boolean {
  return useConfigStore.getState().filesystemLinks
}

/** Source path (see fsLinkPath.ts) for a page id, or null if unknown. */
function sourceForPageId(pageId: string): string | null {
  const page = useTreeStore.getState().getPageById(pageId)
  if (!page) return null
  return fsSourcePath(page.path, page.kind === 'section')
}

/** Source path for the page currently shown in the browser location. */
function sourceForLocation(): string | null {
  if (typeof window === 'undefined') return null
  const pathname = window.location.pathname
  const current = normalizeWikiRoutePath(
    buildViewUrl(stripBasePath(pathname) ?? pathname),
  )
  const page = useTreeStore.getState().getPageByPath(toWikiLookupPath(current))
  if (!page) return null
  return fsSourcePath(page.path, page.kind === 'section')
}

/**
 * Returns the URL to insert for an uploaded/selected asset of `pageId`.
 * `assetUrl` is the server-provided `/assets/<id>/<file>` URL.
 */
export function editorAssetUrl(pageId: string, assetUrl: string): string {
  if (!isFilesystemLinksEnabled()) return assetUrl
  const normalized = assetUrl.startsWith('/assets/')
    ? assetUrl
    : assetUrl.startsWith('assets/')
      ? `/${assetUrl}`
      : null
  if (!normalized) return assetUrl
  const source = sourceForPageId(pageId) ?? sourceForLocation()
  if (!source) return assetUrl
  return fsAssetLink(source, normalized)
}

/**
 * Returns the link to insert for the page at `targetPath` (tree path without
 * leading slash) from the page currently being edited.
 */
export function editorPageLink(targetPath: string): string {
  const legacy = `/${targetPath}`
  if (!isFilesystemLinksEnabled()) return legacy
  const source = sourceForLocation()
  if (!source) return legacy
  const target = useTreeStore.getState().getPageByPath(targetPath)
  return fsPageLink(source, legacy, target?.kind === 'section')
}
