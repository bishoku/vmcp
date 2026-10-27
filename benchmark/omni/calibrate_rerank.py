#!/usr/bin/env python3
"""Exploratory query-level calibration on recorded synthetic rerank scores.

Select on calibration only; report validation without selecting against it. These
are already-known development data, not independent production holdout evidence.
The input report preserves model scores, candidate order and split provenance.
"""
import argparse
import json
from pathlib import Path


def metrics(rows, split, no_match=0, confidence=0, rerank=True, window=20):
    totals = {}
    for row in rows:
        if split != 'all' and row['split'] != split:
            continue
        case = row['case']
        group = case['language'] if case['expected'] else 'negative'
        m = totals.setdefault(group, dict(queries=0, hit1=0, hit5=0, reciprocal_rank_sum=0, false_positive_queries=0, empty_queries=0))
        m['queries'] += 1
        pool = row['candidates'][:window]
        best = max((c['rerank_score'] for c in pool), default=0)
        if rerank and best < no_match:
            selected = []
        elif rerank and best >= confidence:
            selected = sorted(pool, key=lambda c: (-c['rerank_score'], c['tool_ref']))[:5]
        else:
            selected = pool[:5]
        rank = next((n + 1 for n, c in enumerate(selected) if c['name'] in case['expected']), 0)
        m['hit1'] += int(rank == 1)
        m['hit5'] += int(rank > 0)
        m['reciprocal_rank_sum'] += 1 / rank if rank else 0
        m['false_positive_queries'] += int(not case['expected'] and bool(selected))
        m['empty_queries'] += int(not selected)
    for m in totals.values():
        m['mrr_at_5'] = m.pop('reciprocal_rank_sum') / m['queries']
    return totals


def passes(candidate, baseline):
    return all(candidate[l][k] + 1e-9 >= baseline[l][k] for l in ('en', 'tr') for k in ('hit1', 'hit5', 'mrr_at_5')) and candidate['negative']['false_positive_queries'] <= baseline['negative']['false_positive_queries']


def calibrate(report):
    rows = report['results']
    # Fixed exploratory grids, not selected by looking at validation metrics.
    no_match_grid = [0, .00001, .00002, .00005, .0001, .0002, .0005, .001, .005, .01, .025, .05, .1]
    confidence_grid = [0, .01, .025, .05, .1, .2, .4, .6, .8, .9, .99]
    baseline = metrics(rows, 'calibration', rerank=False)
    candidates = []
    for floor in no_match_grid:
        for confidence in confidence_grid:
            if confidence and confidence < floor:
                continue
            measured = metrics(rows, 'calibration', floor, confidence)
            if passes(measured, baseline):
                objective = (measured['negative']['false_positive_queries'], -sum(measured[l]['hit1'] for l in ('en', 'tr')), -sum(measured[l]['mrr_at_5'] for l in ('en', 'tr')), floor, confidence)
                candidates.append((objective, floor, confidence, measured))
    selected = min(candidates, default=None)
    if selected is None:
        floor, confidence = 0, 0
    else:
        _, floor, confidence, _ = selected
    summaries = {s: {'baseline': metrics(rows, s, rerank=False), 'query_gate': metrics(rows, s, floor, 0), 'guarded_rerank': metrics(rows, s, floor, confidence)} for s in ('calibration', 'validation', 'all')}
    return {
        'scope': 'Second exploratory iteration after strict per-tool calibration failed. Same known synthetic corpus and development split. No independent holdout claim.',
        'method': 'Reject entire query below no_match_score; keep retrieval order below confidence_score; otherwise rerank. Tool score floor is disabled for this experiment.',
        'embedding_model': report['embedding_model'], 'rerank_model': report['rerank_model'],
        'no_match_grid': no_match_grid, 'confidence_grid': confidence_grid,
        'selected_no_match_score': floor, 'selected_confidence_score': confidence,
        'calibration_candidate_found': selected is not None,
        'validation_non_regression_gate': selected is not None and passes(summaries['validation']['guarded_rerank'], summaries['validation']['baseline']),
        'summary': summaries,
    }


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('input')
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    result = calibrate(json.loads(Path(args.input).read_text()))
    Path(args.output).write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({k: result[k] for k in ('selected_no_match_score', 'selected_confidence_score', 'calibration_candidate_found', 'validation_non_regression_gate')}))
