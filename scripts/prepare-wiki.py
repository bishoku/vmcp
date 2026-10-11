#!/usr/bin/env python3
"""Render repository documentation as GitHub Wiki Markdown pages."""
import os
from pathlib import Path
import re
import sys
from urllib.parse import quote, urlsplit


def render(text, source, root, repository_url):
    def replace(match):
        target = match.group(1)
        parsed = urlsplit(target)
        if parsed.scheme or parsed.netloc or not parsed.path or target.startswith('/'):
            return match.group(0)
        resolved = (source.parent / parsed.path).resolve()
        suffix = ('?' + parsed.query if parsed.query else '') + ('#' + parsed.fragment if parsed.fragment else '')
        if resolved.parent == root / 'docs' and resolved.suffix == '.md':
            return '](' + quote(resolved.stem) + suffix + ')'
        try:
            relative = resolved.relative_to(root)
        except ValueError:
            return match.group(0)
        kind = 'tree' if resolved.is_dir() else 'blob'
        return '](' + repository_url + '/' + kind + '/main/' + quote(relative.as_posix()) + suffix + ')'

    lines = []
    fence = None
    for line in text.splitlines(keepends=True):
        marker = re.match(r'^\s{0,3}(`{3,}|~{3,})', line)
        if marker:
            value = marker.group(1)
            if fence is None:
                fence = value
            elif value[0] == fence[0] and len(value) >= len(fence):
                fence = None
            lines.append(line)
        elif fence is None:
            lines.append(re.sub(r'\]\(([^\s)]+)\)', replace, line))
        else:
            lines.append(line)
    return ''.join(lines)


def main():
    root = Path(__file__).resolve().parent.parent
    out = Path(sys.argv[1]).resolve()
    source = root / 'docs'
    if out == source or source.is_relative_to(out) or out.is_relative_to(source):
        raise SystemExit('Output must be outside docs/')
    if not (source / 'Home.md').is_file():
        raise SystemExit('docs/Home.md is required')
    repository = os.environ.get('GITHUB_REPOSITORY', 'bishoku/vmcp')
    server = os.environ.get('GITHUB_SERVER_URL', 'https://github.com')
    out.mkdir(parents=True, exist_ok=True)
    for page in sorted(source.glob('*.md')):
        (out / page.name).write_text(render(page.read_text(), page, root, server + '/' + repository))
    print(f'Wiki pages written to {out}')


if __name__ == '__main__':
    main()
