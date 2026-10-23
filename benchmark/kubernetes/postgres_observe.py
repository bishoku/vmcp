"""Bounded read-only PostgreSQL samples in the disposable load-test namespace.

CREATE EXTENSION only enables diagnostics in the synthetic test database. No
settings, query plans, or application data are changed by the observer.
"""
import datetime
import json
import pathlib
import re
import subprocess
import threading
import time

SAMPLE = """
SET statement_timeout='3s';
SELECT json_build_object(
 'utc',clock_timestamp(),
 'activity',(SELECT json_agg(x) FROM (
  SELECT state,wait_event_type,wait_event,count(*) AS sessions
  FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid()
  GROUP BY 1,2,3) x),
 'blocked',(SELECT json_agg(x) FROM (
  SELECT pid,wait_event_type,wait_event,left(query,1500) AS query,
   extract(epoch FROM clock_timestamp()-xact_start) AS transaction_seconds,
   pg_blocking_pids(pid) AS blockers
  FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' LIMIT 160) x),
 'notification_lock',(SELECT json_agg(x) FROM (SELECT pid,mode,granted,classid,objid,objsubid FROM pg_locks WHERE locktype='object' AND classid=1262 AND objid=0) x),
 'locks',(SELECT json_agg(x) FROM (
  SELECT locktype,mode,granted,count(*) AS count FROM pg_locks
  WHERE database=(SELECT oid FROM pg_database WHERE datname=current_database())
  GROUP BY 1,2,3) x),
 'database',(SELECT row_to_json(d) FROM pg_stat_database d WHERE datname=current_database()),
 'wal',(SELECT row_to_json(w) FROM pg_stat_wal w),
 'tables',(SELECT json_agg(t) FROM pg_stat_user_tables t),
 'indexes',(SELECT json_agg(i) FROM pg_stat_user_indexes i),
 'cleanup',(SELECT json_build_object('remaining_fixture_operations',count(*)) FROM mcp_operations WHERE created_at<now()-interval '15 minutes' AND state<>'running')
);
"""
STATEMENTS = """
SET statement_timeout='5s';
SELECT coalesce(json_agg(s),'[]') FROM (
SELECT queryid,query,calls,total_exec_time,mean_exec_time,max_exec_time,rows,
 shared_blks_hit,shared_blks_read,shared_blks_dirtied,shared_blks_written,
 shared_blk_read_time,shared_blk_write_time,wal_records,wal_bytes
FROM pg_stat_statements WHERE dbid=(SELECT oid FROM pg_database WHERE datname=current_database())
ORDER BY total_exec_time DESC LIMIT 150) s;
"""
class DatabaseObserver:
    def __init__(self, kubectl, output, node):
        if not re.fullmatch(r'vmcp-load-[0-9-]+',kubectl[3]):
            raise ValueError('isolated load-test namespace required')
        self.kubectl=kubectl;self.output=pathlib.Path(output);self.node=node
        self.done=threading.Event();self.thread=threading.Thread(target=self.run,daemon=True)
    def sql(self,database,query):
        result=subprocess.run(self.kubectl+['exec','-i','deployment/postgres','--','psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','acceptance','-d',database],input=query,text=True,capture_output=True,timeout=12)
        if result.returncode:raise RuntimeError(result.stderr[-500:])
        return result.stdout.strip()
    def start(self):self.thread.start()
    def stop(self):self.done.set();self.thread.join(timeout=25)
    def run(self):
        database=None;number=0;plans_saved=False
        with (self.output/'postgres-samples.jsonl').open('a') as samples, (self.output/'postgres-observer-errors.log').open('a') as errors:
            while not self.done.is_set():
                try:
                    if database is None:
                        names=self.sql('acceptance',"SELECT datname FROM pg_database WHERE datname ~ '^vmcp_t_[a-f0-9]+$'").splitlines()
                        if len(names)!=1:
                            self.done.wait(2);continue
                        database=names[0]
                        self.sql(database,'CREATE EXTENSION IF NOT EXISTS pg_stat_statements;')
                        limits={}
                        for name in ('cpu.max','memory.max','cpu.stat','memory.events'):
                            limits[name]=subprocess.check_output(self.kubectl+['exec','deployment/postgres','--','cat','/sys/fs/cgroup/'+name],text=True,timeout=10)
                        (self.output/'postgres-cgroup-limits.json').write_text(json.dumps(limits,indent=2))
                    if not (self.output/'postgres-effective-settings.json').exists():
                        actual=self.sql(database,"SELECT json_agg(s) FROM (SELECT name,setting,unit,source FROM pg_settings WHERE name IN ('max_connections','shared_buffers','effective_cache_size','work_mem','maintenance_work_mem','autovacuum_work_mem','wal_buffers','min_wal_size','max_wal_size','checkpoint_timeout','checkpoint_completion_target','autovacuum_naptime','autovacuum_vacuum_scale_factor','autovacuum_analyze_scale_factor','jit','synchronous_commit','fsync','full_page_writes','track_io_timing')) s;")
                        (self.output/'postgres-effective-settings.json').write_text(json.dumps(json.loads(actual),indent=2))
                    value=json.loads(self.sql(database,SAMPLE))
                    log=(self.output/'run.log').read_text()
                    phases=list(re.finditer(r'Phase ([^:]+):',log))
                    value['phase']=phases[-1].group(1) if phases else 'setup'
                    if phases and value['phase']+': completed ' in log[phases[-1].end():]:
                        value['phase']='verification'
                    samples.write(json.dumps(value)+'\n');samples.flush()
                    if value['phase']=='steady' and not plans_saved:
                        plans=self.sql(database, """SET statement_timeout='3s';
SELECT name,setting,unit FROM pg_settings WHERE name IN ('max_connections','shared_buffers','effective_cache_size','work_mem','maintenance_work_mem','autovacuum_work_mem','wal_buffers','min_wal_size','max_wal_size','checkpoint_timeout','checkpoint_completion_target','autovacuum_naptime','autovacuum_vacuum_scale_factor','autovacuum_analyze_scale_factor','jit','synchronous_commit','fsync','full_page_writes','track_io_timing','pg_stat_statements.track');
SELECT tablename,indexname,indexdef FROM pg_indexes WHERE tablename IN ('mcp_operations','mcp_streams','mcp_pending_requests');
EXPLAIN (ANALYZE,BUFFERS) SELECT 1 FROM ONLY mcp_streams WHERE operation_id='00000000-0000-4000-8000-000000000000' LIMIT 1;
EXPLAIN (ANALYZE,BUFFERS) SELECT id FROM mcp_operations WHERE state<>'running' AND created_at<now()-interval '15 minutes' LIMIT 200;
""")
                        (self.output/'postgres-index-plans.txt').write_text(plans)
                        plans_saved=True
                    if number%3==0:
                        statements=json.loads(self.sql(database,STATEMENTS))
                        (self.output/f'postgres-statements-{number:04d}.json').write_text(json.dumps({'utc':value['utc'],'phase':value['phase'],'statements':statements},indent=2))
                        (self.output/'postgres-statements-latest.json').write_text(json.dumps(statements,indent=2))
                        raw=subprocess.check_output(['microk8s','kubectl','get','--raw',f'/api/v1/nodes/{self.node}/proxy/metrics/cadvisor'],text=True,timeout=12)
                        namespace=self.kubectl[3]
                        lines=[l for l in raw.splitlines() if f'namespace="{namespace}"' in l and any(k in l for k in ('container_cpu_cfs_','container_oom_events_total','container_cpu_usage_seconds_total'))]
                        (self.output/f'cgroup-database-{number:04d}.prom').write_text('\n'.join(lines)+'\n')
                    number+=1
                except Exception as error:
                    errors.write(datetime.datetime.now(datetime.timezone.utc).isoformat()+' '+str(error)+'\n');errors.flush()
                self.done.wait(5)
