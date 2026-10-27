#!/usr/bin/env python3
"""Create a small reviewable Omni summary from local raw acceptance evidence."""
import argparse
import json
import pathlib
import re
import statistics


def gc_summary(path):
    if not path.exists():
        return {'available': False}
    points = []
    pattern = re.compile(r'gc \d+ @([\d.]+)s .*?: ([\d.+]+) ms clock, ([\d.+/]+) ms cpu, (\d+)->(\d+)->(\d+) MB')
    for line in path.read_text().splitlines():
        match = pattern.search(line)
        if not match or float(match[1]) < 30:
            continue
        wall = [float(v) for v in match[2].split('+')]
        cpu = sum(float(v) for v in re.split(r'[+/]', match[3]))
        points.append((float(match[1]), cpu, wall[0] + wall[-1], int(match[6])))
    if len(points) < 2:
        return {'available': False}
    elapsed = points[-1][0] - points[0][0]
    pauses = sorted(p[2] for p in points)
    return {'available': True, 'scope': 'original pod; first 30 process seconds excluded; all measured phases combined',
            'observed_seconds': elapsed, 'cycles': len(points), 'cycles_per_second': (len(points)-1)/elapsed,
            'mean_gc_cpu_cores': sum(p[1] for p in points[1:])/1000/elapsed,
            'p95_stw_ms': pauses[min(len(pauses)-1, int(len(pauses)*.95))],
            'max_live_heap_go_MB': max(p[3] for p in points)}


def summarize(root):
    raw = json.loads((root/'omni-report.json').read_text())
    result = {k: v for k,v in raw.items() if k not in ('samples','phases')}
    if result.get('functional_only'):
        result['workload']='100 TPS across nine unaffected vMCPs: four REST and five native MCP; grant and publication revocation'
    if 'loaded_governance_traffic' in result:
        result['loaded_governance_traffic']=dict(result['loaded_governance_traffic'])
        result['loaded_governance_traffic'].pop('slo_pass',None)
        result['loaded_governance_traffic'].pop('Seconds',None)
        result['loaded_governance_traffic']['gate']='functional fault exercise: no invalid results and success >=99%; not a steady-state latency gate'
    result['phases'] = [{k:v for k,v in p.items() if k not in ('Seconds','Nodes','ErrorExamples')} for p in raw['phases']]
    # The initial driver retained the preceding phase label while building the
    # index. Trim resource samples to the measured traffic duration (5s resolution).
    starts={}
    durations={p['name']:p['PlannedSeconds'] for p in raw['phases']}
    for sample in raw['samples']:
        starts.setdefault(sample['Phase'], max(0,sample['Seconds']-5))
    groups = {}
    for sample in raw['samples']:
        if sample['Phase'] in ('setup','faults'):
            continue
        if sample['Phase'] in durations and sample['Seconds'] > starts[sample['Phase']]+durations[sample['Phase']]:
            continue
        for node in sample['Nodes']:
            if not node['Alive']:
                continue
            groups.setdefault((sample['Phase'],node['Name']),[]).append(node)
    resources=[]
    for (phase,name),points in groups.items():
        valid=[p for p in points if p['RSSBytes']>0]
        if not valid:
            resources.append({'phase':phase,'component':name,'error':'no kubelet measurements'});continue
        rss=[p['RSSBytes'] for p in valid];third=max(1,len(rss)//3)
        before,after=statistics.median(rss[:third]),statistics.median(rss[-third:])
        resources.append({'phase':phase,'component':name,'samples':len(valid),'sample_errors':sum(bool(p.get('error')) for p in points),
                          'max_rss_MiB':max(rss)/2**20,'max_container_usage_MiB':max(p['MemoryUsageBytes'] for p in valid)/2**20,
                          'mean_cpu_cores':statistics.mean(p['ReportedCPUPercent']/100 for p in valid),
                          'first_third_median_rss_MiB':before/2**20,'last_third_median_rss_MiB':after/2**20,
                          'rss_growth_percent':100*(after/before-1)})
    result['resources']=resources
    result['gc']={name:gc_summary(root/(name+'-original-pod.log')) for name in ('acceptance-node-1','omni-mcp')}
    if (root/'omni-mcp-gc.log').exists():
        result['gc']['omni-mcp']=gc_summary(root/'omni-mcp-gc.log')
    if (root/'index-loaded.json').exists():
        result['loaded_index']=json.loads((root/'index-loaded.json').read_text())
    if (root/'embedding-process.jsonl').exists():
        model=[json.loads(line) for line in (root/'embedding-process.jsonl').read_text().splitlines()]
        result['embedding_process']={'sample_count':len(model),'max_rss_MiB':max(p['rss_bytes'] for p in model)/2**20,'peak_sample_cpu_cores':max(p['cpu_cores'] for p in model),'scope':'host process, separate from Omni pod limits; includes initial indexing'}
    if (root/'audit-observations.jsonl').exists():
        lines=(root/'audit-observations.jsonl').read_text().splitlines()
        if lines:result['audit_transport_latency_cumulative']=json.loads(lines[-1])
    pods=json.loads((root/'kubernetes-pods.json').read_text())
    result['final_pods']=[{'name':p['metadata']['name'],'containers':[{'name':c['name'],'restarts':c['restartCount'],'last_state':c.get('lastState',{})} for c in p.get('status',{}).get('containerStatuses',[])]} for p in pods['items']]
    result['note']='Kubelet sampling is not a continuous high-water mark. Final pod counters follow intentional fault replacements; inspect Kubernetes events and original logs for earlier OOMs.'
    return result


if __name__ == '__main__':
    parser=argparse.ArgumentParser();parser.add_argument('raw',type=pathlib.Path);parser.add_argument('output',type=pathlib.Path);args=parser.parse_args()
    args.output.write_text(json.dumps(summarize(args.raw),indent=2,ensure_ascii=False)+'\n')
