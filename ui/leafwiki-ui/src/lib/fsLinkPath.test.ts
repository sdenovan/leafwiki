import { describe, expect, it } from 'vitest'
import {
  fsAssetLink,
  fsPageLink,
  fsSourcePath,
  resolveFsHref,
  toAbsoluteWikiUrl,
} from './fsLinkPath'

// Keep in sync with internal/links/fs_links_test.go.
describe('fsLinkPath', () => {
  it('builds source paths', () => {
    expect(fsSourcePath('a/b', false)).toBe('/a/b')
    expect(fsSourcePath('a/b', true)).toBe('/a/b/')
    expect(fsSourcePath('', true)).toBe('/')
  })

  it.each([
    ['/x/a', 'b.md', '/x/b', false],
    ['/x/a', './b.md#frag', '/x/b', false],
    ['/x/a', '../top.md', '/top', false],
    ['/x/a', 'sub/index.md', '/x/sub', true],
    ['/x/a', 'sub/', '/x/sub', true],
    ['/x/a', '../x/', '/x', true],
    ['/x/a', '../../../outside.md', '', false],
    ['/x/a/', 'b.md', '/x/a/b', false],
    ['/x/a/', '../b.md', '/x/b', false],
  ])('resolves page link %s + %s', (source, href, route, section) => {
    const r = resolveFsHref(source, href)
    expect(r.kind).toBe('page')
    if (r.kind === 'page') {
      expect(r.route).toBe(route)
      expect(r.sectionForm).toBe(section)
    }
  })

  it.each([
    ['/x/a', '../../assets/id1/p.png', '/assets/id1/p.png'],
    ['/x/a/', '../../../assets/id1/p.png', '/assets/id1/p.png'],
    ['/a', '../assets/id/f.pdf', '/assets/id/f.pdf'],
  ])('resolves asset %s + %s', (source, href, path) => {
    expect(resolveFsHref(source, href)).toMatchObject({ kind: 'asset', path })
  })

  it.each([
    'b',
    '../b',
    '/x/b.md',
    'assets/id/p.png',
    'https://example.com/a.md',
    '#section',
    'wikilink-notfound:Foo',
  ])('leaves legacy link %s alone', (href) => {
    expect(resolveFsHref('/x/a', href).kind).toBe('none')
    expect(toAbsoluteWikiUrl('/x/a', href)).toBe(href)
  })

  it.each([
    ['/x/a', '/x/b', false, 'b.md'],
    ['/x/a', '/x/b', true, 'b/index.md'],
    ['/x/a', '/top', false, '../top.md'],
    ['/x/a/', '/x/a/c', false, 'c.md'],
    ['/x/a/', '/x/a', true, 'index.md'],
    ['/a', '/b/c', false, 'b/c.md'],
    ['/', '/b', false, 'b.md'],
  ])('generates page link %s -> %s', (source, route, section, want) => {
    const link = fsPageLink(source, route, section)
    expect(link).toBe(want)
    const back = resolveFsHref(source, link)
    expect(back.kind === 'page' && back.route).toBe(route)
  })

  it('generates asset links', () => {
    expect(fsAssetLink('/x/a', '/assets/id/p.png')).toBe(
      '../../assets/id/p.png',
    )
    expect(fsAssetLink('/x/a/', '/assets/id/p.png')).toBe(
      '../../../assets/id/p.png',
    )
    expect(fsAssetLink('/a', 'assets/id/p.png')).toBe('../assets/id/p.png')
  })

  it('converts to absolute wiki urls', () => {
    expect(toAbsoluteWikiUrl('/x/a', '../b.md#h')).toBe('/b#h')
    expect(toAbsoluteWikiUrl('/x/a', '../../assets/i/p.png')).toBe(
      '/assets/i/p.png',
    )
  })
})
