#!/usr/bin/env python3
"""Synthetic local embedding-provider concurrency diagnosis; run with the fixture venv."""
import argparse,concurrent.futures,json,pathlib,socket,subprocess,sys,time,urllib.request,urllib.error
root=pathlib.Path(__file__).resolve().parents[2]
parser=argparse.ArgumentParser();parser.add_argument('--output',type=pathlib.Path,required=True);args=parser.parse_args()
with socket.socket() as sock:
 sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
base=f'http://127.0.0.1:{port}'
model='sentence-transformers/paraphrase-multilingual-mpnet-base-v2'
results=[]
for capacity in [16,32]:
 with open(f'/tmp/omni-provider-{capacity}.log','w') as log:
  process=subprocess.Popen([sys.executable,str(root/'benchmark/omni/embedding_fixture.py'),'--port',str(port),'--concurrency',str(capacity),'--model',model],stdout=log,stderr=subprocess.STDOUT)
  try:
   for _ in range(100):
    if process.poll() is not None:raise RuntimeError("provider process exited")
    try:
     urllib.request.urlopen(base+'/test/stats',timeout=1).close();break
    except Exception:time.sleep(.2)
   else:raise RuntimeError('provider failed to start')
   def call():
    began=time.monotonic();status=0
    request=urllib.request.Request(base+'/v1/embeddings',data=json.dumps({'model':model,'input':['Synthetic tool']}).encode(),headers={'Content-Type':'application/json'})
    try:
     with urllib.request.urlopen(request,timeout=10) as response:response.read();status=response.status
    except urllib.error.HTTPError as error:status=error.code
    return status,(time.monotonic()-began)*1000
   start=time.monotonic();futures=[]
   with concurrent.futures.ThreadPoolExecutor(max_workers=64) as executor:
    for i in range(6000):
     time.sleep(max(0,start+i/100-time.monotonic()));futures.append(executor.submit(call))
    values=[f.result() for f in futures]
   statuses={str(s):sum(v[0]==s for v in values) for s in set(v[0] for v in values)}
   with urllib.request.urlopen(base+'/test/stats') as response:stats=json.load(response)
   latencies=sorted(v[1] for v in values if v[0]==200)
   results.append({'concurrency_limit':capacity,'offered':6000,'target_rps':100,'elapsed_seconds':time.monotonic()-start,'statuses':statuses,'provider_counters':stats,'success_p95_ms':latencies[int(len(latencies)*.95)]})
   print(results[-1],flush=True)
  finally:process.terminate();process.wait(timeout=10)
args.output.write_text(json.dumps({'model':model,'scope':'60s direct provider diagnostic, not a replacement for the 10min gateway test; uncached repeated query','runs':results},indent=2)+'\n')
