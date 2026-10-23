#!/usr/bin/env python3
"""Read-only operational evidence during a long test; emits no credentials."""
import argparse, datetime, json, pathlib, re, subprocess, time

def main():
    p=argparse.ArgumentParser();p.add_argument('--namespace',required=True);p.add_argument('--node',required=True);p.add_argument('--output',required=True);a=p.parse_args()
    if not re.fullmatch(r'vmcp-load-[0-9-]+',a.namespace):raise SystemExit('dedicated test namespace required')
    root=pathlib.Path(a.output);start=time.monotonic();number=0
    with (root/'observations.jsonl').open('a') as stream:
        while not (root/'report.json').exists():
            sample={'utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'seconds':time.monotonic()-start}
            try:
                raw=subprocess.check_output(['microk8s','kubectl','get','--raw',f'/api/v1/nodes/{a.node}/proxy/stats/summary'],text=True,timeout=10)
                stats=json.loads(raw)
                sample['containers']=[{'pod':pod['podRef']['name'],'container':c['name'],'rss_bytes':c.get('memory',{}).get('rssBytes'),'cpu_cores':c.get('cpu',{}).get('usageNanoCores',0)/1e9} for pod in stats.get('pods',[]) if pod['podRef']['namespace']==a.namespace for c in pod['containers']]
                phases=re.findall(r'Phase ([^:]+): (\d+) req/s for (\d+)s',(root/'run.log').read_text())
                if phases:sample['phase'],sample['target_rps'],sample['planned_seconds']=phases[-1]
                if number%2==0:
                    text=subprocess.check_output(['microk8s','kubectl','get','--raw',f'/api/v1/nodes/{a.node}/proxy/metrics/cadvisor'],text=True,timeout=10)
                    lines=[line for line in text.splitlines() if f'namespace="{a.namespace}"' in line and any(key in line for key in ('container_cpu_cfs_throttled','container_cpu_cfs_periods_total','container_oom_events_total'))]
                    (root/f'cgroup-observation-{number:04d}.prom').write_text('\n'.join(lines)+'\n')
            except Exception as error:sample['error_type']=type(error).__name__
            stream.write(json.dumps(sample)+'\n');stream.flush();(root/'latest-observation.json').write_text(json.dumps(sample,indent=2))
            number+=1;time.sleep(30)

if __name__=='__main__':main()
