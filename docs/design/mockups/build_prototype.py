#!/usr/bin/env python3
"""Generate prototype.html from the five screens and the assets inlined with them.

The source of truth is every file read by name below, not the five screens alone;
a change to any of them owes prototype.html a regenerate. This script inlines the
shared CSS and JS, concatenates the five <main> blocks, and rewrites every link to
the five files into a #hash route so the single file clicks through the same way.
"""
import hashlib, re, pathlib

SRC = pathlib.Path(__file__).resolve().parent
PAGES = [('home', 'index.html'), ('services', 'services.html'),
         ('libraries', 'libraries.html'), ('search', 'search.html'),
         ('requests', 'requests.html')]
ROUTE = {'index.html': '#home', 'services.html': '#services',
         'libraries.html': '#libraries', 'search.html': '#search',
         'requests.html': '#requests'}

# Every build input is read through here so that the manifest written into
# prototype.html is a BYPRODUCT OF THE READS THEMSELVES rather than a parallel
# list kept in step by hand. A hand-maintained list is a list that drifts, and a
# drifted manifest is a stamp vouching for bytes nothing actually looked at.
# Bytes, not text, because the digest has to survive being recomputed by a
# different language's reader (check.mjs re-hashes these same files) and no
# newline or encoding translation may sit between the two.
INPUTS = {}


def read(name):
    data = (SRC / name).read_bytes()
    INPUTS[name] = hashlib.sha256(data).hexdigest()
    return data.decode()


def block(text, name):
    m = re.search(r'<!-- ' + name + r' START -->(.*?)<!-- ' + name + r' END -->', text, re.S)
    if not m:
        raise SystemExit('marker not found: ' + name)
    return m.group(1)


def rewrite_links(html):
    # Filenames may carry a #fragment or a ?query (the media-type nav entries
    # are search.html?type=..., the library scope is ?lib=..., and the
    # Search-to-Requests action is requests.html?q=...&type=...).
    #
    # The query is carried across rather than dropped. It used to be thrown
    # away, which made every media-type nav entry collapse onto a bare #search
    # and -- once Search grew a "Search indexers" action per row -- would have
    # silently discarded the query and category the whole action exists to
    # pass. usarr.js's router splits the hash on "?" so the route still
    # resolves, and the parameters survive in the address bar where a reader
    # can see that they are real.
    def sub(m):
        route = ROUTE[m.group('file')]
        query = m.group('query') or ''
        # A #fragment on a cross-page link cannot survive hash routing; a
        # ?query can, so only the query is kept.
        if query.startswith('#'):
            query = ''
        return 'href="' + route + query + '"'

    pattern = (r'href="(?P<file>' + '|'.join(re.escape(f) for f in ROUTE) +
               r')(?P<query>[?#][^"]*)?"')
    return re.sub(pattern, sub, html)


# fonts.css carries the two @font-face rules and nothing else, so usarr.css
# stays a stylesheet a person can read. Both are inlined here, fonts first,
# because prototype.html has to be a single file with no external request.
css = read('fonts.css') + '\n' + read('usarr.css')
js = read('usarr.js')
index = read('index.html')

sprite = block(index, 'SPRITE')
topbar = rewrite_links(block(index, 'TOPBAR'))

# This file is the one built for publishing, so the statement that it is a
# mockup over invented data has to survive a regenerate. It lives in the shared
# top bar rather than on any one page, and this assertion is what stops a
# future edit to index.html quietly dropping it from every screen at once.
if 'class="mocknote"' not in topbar:
    raise SystemExit('the permanent mockup notice is missing from the top bar')
sidebar = rewrite_links(block(index, 'SIDEBAR')).replace(' aria-current="page"', '')

body = '\n'.join(rewrite_links(block(read(f), 'PAGE:' + r).strip())
                 for r, f in PAGES)

# The script's own bytes are an input too. The doctype, the <head>, the <title>
# and the banner below are template text that lives HERE, so an edit to any of
# them changes prototype.html without touching one of the eight files read
# above. A stamp that omitted this file would vouch for a prototype.html the
# current script no longer produces. The cost is that even a comment-only edit
# here owes a regenerate; that is one command, and it is the price of the stamp
# meaning what it says.
read('build_prototype.py')

# Sorted so the block is stable across runs and a diff shows only what moved.
manifest = '\n'.join(f'       {name}  sha256:{digest}'
                     for name, digest in sorted(INPUTS.items()))

# The <title> is user-visible copy -- it is the browser tab, the bookmark and
# the history entry -- so §13's rules bind it like any other string, and
# check.mjs now reads document.title for exactly that reason. It used to say
# "UsArr v0.1 screen mockup — static, invented data, nothing implemented",
# which broke both: an em dash in a nine-word string, and a v0.1 claim over a
# page whose default view is the full stack, which §16 sequences AFTER v0.1.
# The milestone belongs to the install switcher, which can state it truthfully
# for whichever install is selected; a fixed title cannot.
#
# It then read "UsArr screen mockups: static, invented data, nothing
# implemented" until the screens shipped and "nothing implemented" went false —
# the same defect as the top-bar notice, in the one string a rendered DOM walk
# cannot see. What survives is the half that stays true: the data is invented.
# Implementation status is §16's and the tree's, not a browser tab's, and
# check.mjs asserts that document.title claims none (REVIEW-LOG SD-01a).
out = f'''<!doctype html>
<html lang="en" data-density="compact">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>UsArr screen mockups: static, invented data</title>
<style>
{css}
</style>
</head>
<body>
<!-- Generated from index.html, services.html, libraries.html, search.html and
     requests.html, with fonts.css, usarr.css and usarr.js inlined. Do not edit
     this file by hand: edit whichever of those files the change belongs in, and
     regenerate with:

         python3 docs/design/mockups/build_prototype.py

     BUILD STAMP. The digests below are of the exact input bytes this file was
     generated from. docs/design/check.mjs re-hashes each of these files on disk
     and compares, so an edited source that was not followed by a regenerate
     turns `make design` red instead of passing unnoticed. The stamp lives in
     here, under this banner, rather than in a manifest file of its own: a
     manifest beside the sources reads as a file that wants hand-updating, and a
     pin someone bumps without regenerating is worth nothing. Do not edit these
     lines -- regenerate, which is the only thing that makes them true.

{manifest}
     END BUILD STAMP -->
{sprite}
<div class="app" data-sidebar="open">
{topbar}
{sidebar}
{body}
</div>
<div class="toasts"></div>
<script>
{js}
</script>
</body>
</html>
'''

(SRC / 'prototype.html').write_text(out)
print('wrote', SRC / 'prototype.html', len(out), 'bytes')
