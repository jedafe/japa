#!/usr/bin/env python3
"""Codemod: replace hardcoded 'JapaConnect'/'Japa Connect' brand strings.

Rules:
- Inside t(...) call spans              -> '{site}' token inside the literal
- String literal in <script> or {} expr -> `${siteName()}` (literal -> template)
- Inside markup attribute strings       -> '{siteName()}' kept inside quotes
- Bare markup text                      -> '{siteName()}'
- Comments, <style>, regexes            -> untouched (flagged for review)
- Server files / tests                  -> skipped (handled manually)

Adds `import { siteName }` when siteName() is newly used.
"""
import re
import sys
from pathlib import Path

BRAND = re.compile(r'(?<![A-Za-z0-9_])Japa ?Connect(?![A-Za-z0-9_])')

APP_IMPORT = {
    'main': '$lib/i18n',
    'tools': '$lib/i18n',
    'academy': '$lib/i18n',
    'docs': '$lib/i18n',
    'admin': '$lib/stores/site.svelte',
    'control': '$lib/stores/site.svelte',
}

EXCLUDE_PARTS = {'node_modules', '.wxt', '.output', 'build', 'dist', 'test-results'}
EXCLUDE_SUFFIX = ('.server.ts', '.test.ts', '.spec.ts', '.d.ts')
EXCLUDE_NAMES = {'hooks.server.ts', 'service-worker.ts'}


def find_comments(text: str) -> list[tuple[int, int]]:
    spans = []
    i, n = 0, len(text)
    while i < n:
        if text.startswith('//', i):
            # Skip URL schemes (https://, app://) — not a comment.
            if i > 0 and text[i - 1] == ':':
                i += 2
                continue
            j = text.find('\n', i)
            spans.append((i, n if j == -1 else j))
            i = n if j == -1 else j
        elif text.startswith('/*', i):
            j = text.find('*/', i + 2)
            spans.append((i, n if j == -1 else j + 2))
            i = n if j == -1 else j + 2
        elif text.startswith('<!--', i):
            j = text.find('-->', i + 4)
            spans.append((i, n if j == -1 else j + 3))
            i = n if j == -1 else j + 3
        else:
            i += 1
    return spans


def find_strings(text: str, start: int, end: int, comments: list[tuple[int, int]]) -> list[tuple[int, int, str]]:
    """Quoted literals wholly inside [start, end), skipping comment spans."""
    out = []
    i = start
    while i < end:
        c = text[i]
        if in_spans(i, comments):
            i = next(b for a, b in comments if a <= i < b)
            continue
        if c in "'\"`":
            q = c
            j = i + 1
            while j < end:
                if text[j] == '\\':
                    j += 2
                    continue
                if text[j] == q:
                    break
                j += 1
            out.append((i, min(j + 1, end), q))
            i = j + 1
        else:
            i += 1
    return out


def find_attr_strings(text: str, regions: list[tuple[int, int]]) -> list[tuple[int, int, str]]:
    """Markup attribute strings: `ident= "…" / '…'` inside the given regions."""
    out = []
    for a, b in regions:
        sub = text[a:b]
        for m in re.finditer(r'=\s*(["\'])', sub):
            q = m.group(1)
            qs = a + m.start(1)
            j = qs + 1
            while j < b and text[j] != q:
                if text[j] == '\\':
                    j += 1
                j += 1
            out.append((qs, min(j + 1, b), q))
    return out


def invert_spans(n: int, spans: list[tuple[int, int]]) -> list[tuple[int, int]]:
    out, cur = [], 0
    for a, b in sorted(spans):
        if cur < a:
            out.append((cur, a))
        cur = max(cur, b)
    if cur < n:
        out.append((cur, n))
    return out


def find_call_spans(text: str, fname: str = 't') -> list[tuple[int, int]]:
    """Spans of fname(...) calls: from '(' to matching ')'."""
    spans = []
    for m in re.finditer(r'\b' + fname + r'\s*\(', text):
        open_paren = text.index('(', m.start())
        depth, i = 0, open_paren
        in_str = None
        while i < len(text):
            ch = text[i]
            if in_str:
                if ch == '\\':
                    i += 2
                    continue
                if ch == in_str:
                    in_str = None
            elif ch in "'\"`":
                in_str = ch
            elif ch == '(':
                depth += 1
            elif ch == ')':
                depth -= 1
                if depth == 0:
                    spans.append((open_paren, i + 1))
                    break
            i += 1
    return spans


def region_spans(text: str, tag: str) -> list[tuple[int, int]]:
    return [(m.start(1), m.end(1)) for m in re.finditer(r'<' + tag + r'[^>]*>(.*?)</' + tag + r'>', text, re.S)]


def find_brace_exprs(text: str, exclude: list[tuple[int, int]]) -> list[tuple[int, int]]:
    """Top-level {…} expressions in markup (outside script/style/comments)."""
    mask = [False] * len(text)
    for a, b in exclude:
        for i in range(a, b):
            mask[i] = True
    spans = []
    stack = []
    i, n = 0, len(text)
    in_str = None
    while i < n:
        if mask[i]:
            i += 1
            continue
        ch = text[i]
        if in_str:
            if ch == '\\':
                i += 2
                continue
            if ch == in_str:
                in_str = None
            i += 1
            continue
        if ch in "'\"`":
            in_str = ch
            i += 1
            continue
        if ch == '{':
            stack.append(i)
        elif ch == '}' and stack:
            start = stack.pop()
            if not stack:
                spans.append((start, i + 1))
        i += 1
    return spans


def in_spans(pos: int, spans) -> bool:
    return any(a <= pos < b for a, b in spans)


# Files that declare their own siteName binding or define the site store must
# be converted by hand — auto-rewriting would create circular imports.
MANUAL_DECL = re.compile(r'(const|let|var|function)\s+siteName\b|\bsiteName\s*[:?=]')


def transform_file(path: Path, app: str, review: list[str]) -> bool:
    text = path.read_text()
    if not BRAND.search(text):
        return False

    if '/i18n/' in str(path) or path.name == 'site.svelte.ts':
        return False
    if MANUAL_DECL.search(text):
        review.append(f'{path} MANUAL (declares siteName)')
        return False

    is_svelte = path.suffix == '.svelte'
    comments = find_comments(text)
    scripts = region_spans(text, 'script') if is_svelte else [(0, len(text))]
    styles = region_spans(text, 'style') if is_svelte else []
    # Code regions = <script> bodies + top-level {…} markup expressions.
    brace_exprs = find_brace_exprs(text, comments + scripts + styles) if is_svelte else []
    code_regions = sorted(scripts + brace_exprs)
    t_spans = find_call_spans(text)

    strings = []
    for a, b in code_regions:
        strings.extend(find_strings(text, a, b, comments))
    # Markup attribute strings live outside code regions/comments/styles.
    markup_regions = invert_spans(len(text), comments + scripts + styles + brace_exprs)
    attr_strings = find_attr_strings(text, markup_regions)

    repls = []
    used_site_fn = False

    lit_of = {}
    for (a, b, q) in strings + attr_strings:
        for m in BRAND.finditer(text, a, b):
            lit_of[m.start()] = (a, b, q)

    for m in BRAND.finditer(text):
        s = m.start()
        ln = text.count('\n', 0, s) + 1
        if in_spans(s, comments) or in_spans(s, styles):
            continue
        lit = lit_of.get(s)
        if lit:
            a, b, q = lit
            in_code = in_spans(a, code_regions)
            if in_code and in_spans(s, t_spans):
                repls.append((s, m.end(), '{site}'))
            elif in_code:
                repls.append((s, m.end(), '${siteName()}'))
                used_site_fn = True
            else:
                repls.append((s, m.end(), '{siteName()}'))
                used_site_fn = True
        else:
            if in_spans(s, code_regions):
                review.append(f'{path}:{ln} CODE: {text[max(0, s-50):s+50]!r}')
                continue
            repls.append((s, m.end(), '{siteName()}'))
            used_site_fn = True

    if not repls:
        return False

    for a, b, r in sorted(repls, reverse=True):
        text = text[:a] + r + text[b:]

    # Script literals that now hold ${siteName()} must become template literals.
    if used_site_fn:
        def fix_quotes(mm):
            body = mm.group(2)
            if '`' in body:
                return mm.group(0)
            return '`' + body + '`'
        text = re.sub(r"(['\"])([^'\"]*?\$\{siteName\(\)\}[^'\"]*?)\1", fix_quotes, text)
        # Whole-string literals collapse to a bare call.
        text = text.replace('`${siteName()}`', 'siteName()')

    needs_import = 'siteName()' in text and not re.search(r'import[^;]*\bsiteName\b', text) and 'function siteName' not in text
    if needs_import and used_site_fn:
        imp_path = APP_IMPORT[app]
        existing = re.search(r"import\s*\{([^}]*)\}\s*from\s*'" + re.escape(imp_path) + r"'", text)
        if existing:
            names = existing.group(1).rstrip()
            if 'siteName' not in names:
                sep = ', ' if names.strip() else ''
                text = text[:existing.start(1)] + names + sep + 'siteName' + text[existing.end(1):]
        elif is_svelte:
            m = re.search(r'<script[^>]*>', text)
            if m:
                insert_at = m.end()
                text = text[:insert_at] + f"\n\timport {{ siteName }} from '{imp_path}';" + text[insert_at:]
            else:
                text = f"<script lang=\"ts\">\n\timport {{ siteName }} from '{imp_path}';\n</script>\n" + text
        else:
            imps = list(re.finditer(r'^import[^;]+;\s*$', text, re.M))
            if imps:
                pos = imps[-1].end()
                text = text[:pos] + f"\nimport {{ siteName }} from '{imp_path}';" + text[pos:]
            else:
                text = f"import {{ siteName }} from '{imp_path}';\n" + text

    path.write_text(text)
    return True


def main():
    root = Path('/home/jmobile/websites/japa/japaconnect')
    apps = sys.argv[1].split(',') if len(sys.argv) > 1 else ['main', 'tools', 'academy', 'docs', 'admin', 'control']
    changed = []
    review: list[str] = []
    for app in apps:
        src = root / 'apps' / app / 'src'
        for p in sorted(src.rglob('*')):
            if p.suffix not in ('.svelte', '.ts'):
                continue
            if any(part in EXCLUDE_PARTS for part in p.parts):
                continue
            if p.name.endswith(EXCLUDE_SUFFIX) or p.name in EXCLUDE_NAMES:
                continue
            if '/lib/server/' in str(p):
                continue
            try:
                if transform_file(p, app, review):
                    changed.append(str(p))
            except Exception as e:
                print(f'ERROR {p}: {e}')
    print(f'\n{len(changed)} files changed')
    for c in changed:
        print(' ', c.replace(str(root) + '/', ''))
    if review:
        print(f'\n{len(review)} flagged for review:')
        for r in review:
            print(' ', r)


if __name__ == '__main__':
    main()
