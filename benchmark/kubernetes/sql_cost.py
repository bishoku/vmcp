#!/usr/bin/env python3
"""Summarize retained PostgreSQL statement deltas; never connects to a database."""
import argparse
import json
from pathlib import Path


def interval(directory, name):
    before = json.loads((directory / f'sql-{name}-before.json').read_text())
    after = json.loads((directory / f'sql-{name}-after.json').read_text())
    prior = {(row['queryid'], row['toplevel']): row for row in before}
    changes = []
    for row in after:
        old = prior.get((row['queryid'], row['toplevel']), {})
        calls = row['calls'] - old.get('calls', 0)
        if calls < 0:
            raise ValueError('statistics reset during measurement')
        if calls:
            changes.append(dict(query=row['query'], toplevel=row['toplevel'], calls=calls,
                rows=row['rows']-old.get('rows', 0),
                execution_ms=row['total_exec_time']-old.get('total_exec_time', 0),
                wal_bytes=int(row['wal_bytes'])-int(old.get('wal_bytes', 0))))
    return {'top_level_commands': sum(r['calls'] for r in changes if r['toplevel']),
            'nested_statements': sum(r['calls'] for r in changes if not r['toplevel']),
            'statements': sorted(changes, key=lambda r: (not r['toplevel'], -r['calls'], r['query']))}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    result = {name: interval(args.directory, name) for name in ('idle', 'single', 'serial')}
    result['scope'] = 'Includes background SQL; top-level commands include transaction boundaries. Nested statement costs overlap parent execution; do not sum them.'
    (args.directory / 'sql-cost-deltas.json').write_text(json.dumps(result, indent=2)+'\n')
    for name in ('idle', 'single', 'serial'):
        print(name, result[name]['top_level_commands'], 'top-level commands;', result[name]['nested_statements'], 'nested statements')
