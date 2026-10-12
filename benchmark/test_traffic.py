"""Opt-in traffic-generator integration test against the running demo stack."""
import json
import time
import urllib.request
import uuid
from smoke import api

BASE = 'http://localhost:3302'
def traffic(path, body=None):
    req = urllib.request.Request(BASE + '/api/' + path, data=json.dumps(body).encode() if body is not None else None, headers={'Content-Type':'application/json','Origin':BASE})
    with urllib.request.urlopen(req,timeout=30) as response: return json.load(response)

def main():
    slug = 'traffic-' + uuid.uuid4().hex[:10]
    upstream = virtual = key_created = False
    try:
        api('POST','/upstreams',{'slug':slug,'name':slug,'url':'http://mcp-calculator:9100/mcp','transport':'streamable_http','headers':{'Authorization':'Bearer calculator-secret'}}); upstream=True
        api('POST','/upstreams/'+slug+'/refresh-tools',{})
        api('POST','/virtual-servers',{'slug':slug,'name':slug,'authRequired':True,'tools':[{'upstream':slug,'tool':'calculate','alias':'sum'}]});virtual=True
        key=api('POST','/agent-keys',{'slug':slug,'virtualServers':[slug]})['key'];key_created=True
        assert slug in [x['slug'] for x in traffic('servers')]
        body={'server':slug,'key':key}
        for attempt in range(30):
            try:
                tools=traffic('tools',body);break
            except Exception:
                if attempt==29: raise
                time.sleep(1)
        assert 'sum' in [x['name'] for x in tools]
        traffic('start',dict(body,tool='sum',arguments={'a':4,'b':7},rate=5,duration=3,workers=2))
        for _ in range(60):
            status=traffic('status')
            if status['state']=='finished': break
            time.sleep(.5)
        assert status['state']=='finished',status
        assert status['ok']>=5 and status['errors']==0 and status['tool_errors']==0,status
        assert key not in json.dumps(status)
        print('Traffic integration passed:',status)
    finally:
        traffic('stop',{})
        if key_created: api('DELETE','/agent-keys/'+slug)
        if virtual: api('DELETE','/virtual-servers/'+slug)
        if upstream: api('DELETE','/upstreams/'+slug)

if __name__=='__main__': main()
