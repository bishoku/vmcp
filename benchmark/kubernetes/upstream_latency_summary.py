#!/usr/bin/env python3
"""Derive descriptive phase comparisons from retained latency evidence, without DB access."""
import argparse
import json
from pathlib import Path
from statistics import median


def summarize(directory):
    report = json.loads((directory/'upstream-latency.json').read_text())
    phases = []
    for phase in report['phases']:
        m = phase['metrics']; name = m['name']
        before = json.loads((directory/f'{name}-sql-before.json').read_text())
        after = json.loads((directory/f'{name}-sql-after.json').read_text())
        prior = {(r['queryid'], r['toplevel']): r for r in before}
        statements = []
        for row in after:
            old = prior.get((row['queryid'], row['toplevel']), {})
            calls = row['calls']-old.get('calls', 0)
            if calls < 0:
                raise ValueError('statement statistics reset')
            if calls:
                statements.append({'query': row['query'], 'toplevel': row['toplevel'], 'calls': calls,
                                   'execution_ms': row['total_exec_time']-old.get('total_exec_time', 0)})
        samples = [s for s in report['samples'] if s['Phase'] == name and s['Seconds'] <= phase['elapsed_seconds_from_start']]
        nodes = [n for s in samples for n in s['Nodes']]
        top = [s for s in statements if s['toplevel']]
        events = sum(s['calls'] for s in top if s['query'].startswith('SELECT e.seq,e.id::text,e.payload,e.terminal'))
        phases.append(dict(name=name, metrics=m, upstream_delay_ms=phase['upstream_delay_ms'],
            observed_wait_mean_ms=phase['observed_wait_mean_ms'], peak_upstream_waiters=phase['peak_upstream_waiters'],
            p95_minus_injected_ms=m['P95MS']-phase['upstream_delay_ms'],
            p99_minus_injected_ms=m['P99MS']-phase['upstream_delay_ms'],
            top_level_commands=sum(s['calls'] for s in top),
            commands_per_success_including_background=sum(s['calls'] for s in top)/max(m['Success'], 1),
            event_reads_per_success=events/max(m['Success'], 1),
            sample_count=len(samples), median_cpu_percent=median(n['ReportedCPUPercent'] for n in nodes) if nodes else None,
            max_rss_bytes=max((n['RSSBytes'] for n in nodes), default=None),
            max_streams=max((n['Metrics'].get('vmcp_streams_active', 0) for n in nodes), default=None),
            max_pool_acquired=max((n['Metrics'].get('vmcp_pool_acquired', 0) for n in nodes), default=None),
            max_cancelled_acquires=max((n['Metrics'].get('vmcp_pool_cancelled_acquires_total', 0) for n in nodes), default=None),
            statements=statements))
    return {'scope': 'Descriptive comparison, no SLO gate. SQL includes background/monitoring commands; delay subtraction includes network, harness and scheduling overhead, not pure gateway time.', 'phases': phases}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    result = summarize(args.directory)
    (args.directory/'upstream-latency-summary.json').write_text(json.dumps(result, indent=2)+'\n')
    for phase in result['phases']:
        print(phase['name'], 'p95', round(phase['metrics']['P95MS'], 3), 'ms;',
              round(phase['commands_per_success_including_background'], 3), 'commands/success including background;',
              phase['peak_upstream_waiters'], 'peak upstream waiters;', phase['max_pool_acquired'], 'max sampled DB connections')
