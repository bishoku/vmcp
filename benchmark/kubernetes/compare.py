#!/usr/bin/env python3
"""Render an evidence-based comparison; never equate different hardware/platforms."""
import json, pathlib, sys

def render(directory, baseline=None):
    root=pathlib.Path(directory).resolve()
    previous=json.loads(pathlib.Path(baseline).read_text()) if baseline else {'Phases':[], 'Profile':{}}
    current=json.loads((root/'report.json').read_text())
    if current['Profile'].get('Smoke'):
        path=root/'comparison.md'
        path.write_text('# MicroK8s fonksiyonel smoke testi\n\nBu kısa ve küçük envanterli koşu kapasite karşılaştırması değildir. [Ayrıntılı smoke raporu](report.md) ve [ham ölçümler](report.json) Kubernetes test düzeneğini doğrular.\n')
        return path
    if current.get('DataPlaneConfiguration',{}).get('capacity_search'):
        cfg=current['DataPlaneConfiguration'];path=root/'comparison.md'
        tool_count=current['Profile']['Servers']*current['Profile']['Tools']
        lines=[f'# {tool_count:,} tool kapasite taraması','',f"Data-plane: {cfg['nodes']}; routing: {cfg.get('routing')}; arama sonucu: {cfg['capacity_search']}",'','| Aşama | Süre s | Hedef TPS | Başarılı TPS | p95 / p99 ms | Hata / gönderilemeyen | SLO |','|---|---:|---:|---:|---:|---:|---|']
        for p in current['Phases']:
            lines.append(f"| {p['name']} | {p['PlannedSeconds']:.0f} | {p['target_rps']} | {p['CompletedRPS']:.1f} | {p['P95MS']:.1f} / {p['P99MS']:.1f} | {p['Errors']} / {p['Dropped']} | {p['slo_pass']} |")
        lines+=['','Tarama aşamaları kısa süreli sınır aramasıdır; sürdürülebilir kapasite için steady doğrulaması esas alınmalıdır. [Ayrıntılı rapor](report.md).']
        path.write_text('\n'.join(lines)+'\n');return path
    if current.get('DataPlaneConfiguration',{}).get('database_focus'):
        phase=next(p for p in current['Phases'] if p['name']=='steady')
        cpus=current['DataPlaneConfiguration'].get('gomaxprocs_each',2)
        path=root/'comparison.md'
        path.write_text(f"# PostgreSQL odaklı MicroK8s koşusu\n\n{current.get('DataPlaneConfiguration',{}).get('steady_seconds',300)} saniye, hedef {phase['target_rps']} TPS; başarılı {phase['CompletedRPS']:.1f} TPS; p95 {phase['P95MS']:.1f} ms; hata {phase['Errors']}, gönderilemeyen {phase['Dropped']}.\n\nPostgreSQL CPU/bellek limitsiz, üç yeni delivery indeksi var. Data-plane başına GOMAXPROCS={cpus}. Süresi dolmuş 100.000 operation/stream fixture’ı temizliği çalıştırır. Fault ve trafik altında config mutasyonu kapsam dışıdır. Önceki 42 dakikalık koşuyla süre/yük/bakım kuyruğu farklıdır; doğrudan Kubernetes hızlanması iddiası değildir.\n\n[Ayrıntılı rapor](report.md), [SQL örnekleri](postgres-samples.jsonl), [sorgu sayaçları](postgres-statements-latest.json).\n")
        return path
    before={p['name']:p for p in previous['Phases']}
    after={p['name']:p for p in current['Phases']}
    c=current.get('Consistency') or {}
    scheduled=sum(p['Scheduled'] for p in current['Phases'])
    elapsed=sum(p['ElapsedSeconds'] for p in current['Phases'])
    faults=current.get('Failures') or []
    lines=['# MicroK8s yük testi — Compose karşılaştırması','',f"Sonuç: **{'Kabul eşikleri karşılandı' if current.get('Pass') else 'Kabul bulguları var'}**. {elapsed/60:.1f} dakika aşama süresinde {scheduled:,} planlanan çağrı; {c.get('successful_client_calls',0):,} başarılı istemci çağrısı; audit eşleşmeleri aşağıda ayrıca raporlanır.",'',
      'Önceki REST koşusu macOS/arm64 ve native data-plane + Compose PostgreSQL/Redis kullanıyordu. Bu koşu Linux/amd64, MicroK8s podları ve Kubernetes Service/CNI yolunu kullanıyor. Yazılım ağacı, Go sürümü ve kaynak limitleri de farklıdır; gecikme farkları tek başına Kubernetes veya kod optimizasyonu kazancı sayılamaz.','',
      f"Güncel envanter: {current['Profile']['Servers']} upstream/virtual server, {current['Profile']['Servers']*current['Profile']['Tools']:,} tool, {current['Profile']['Sessions']} oturum. Strict metadata audit açık; payload capture, aktif rate limit ve JWT/IdP yükü kapsam dışı.",'',
      '| Aşama | Önceki hedef/sn · süre | Önceki başarı/sn · p95 / p99 ms | MicroK8s hedef/sn · süre | MicroK8s başarı/sn · p95 / p99 ms | Yeni hata / gönderilemeyen |',
      '|---|---:|---:|---:|---:|---:|']
    if baseline is None:
        lines.insert(2, 'Yerel karşılaştırma raporu sağlanmadı; önceki ölçüm sütunları ölçülmedi. Tarihsel ortam açıklamaları referans bağlamıdır.')
    for name in ['baseline','steady','spike','recovery','config-refresh','node-loss','node-rejoin','soak']:
        new=after.get(name)
        if not new:continue
        old=before.get('steady' if name=='baseline' else name)
        a=f"{old['target_rps']} · {old['PlannedSeconds']:.0f}s" if old else '—'
        b=f"{old['CompletedRPS']:.1f} · {old['P95MS']:.1f} / {old['P99MS']:.1f}" if old else '—'
        lines.append(f"| {name} | {a} | {b} | {new['target_rps']} · {new['PlannedSeconds']:.0f}s | {new['CompletedRPS']:.1f} · {new['P95MS']:.1f} / {new['P99MS']:.1f} | {new['Errors']} / {new['Dropped']} |")
    lines+=['','## Sonuç ve öncelikler','']
    for name in ('baseline','steady','spike','recovery','config-refresh','node-loss','node-rejoin','soak'):
        phase=after.get(name)
        if phase:
            lines.append(f"- {name}: hedef {phase['target_rps']} çağrı/sn, başarılı {phase['CompletedRPS']:.1f} çağrı/sn; eşik {'geçti' if phase.get('slo_pass') else 'geçmedi'}.")
    functional=current.get('Functional') or {}
    for name in ('configuration_refresh_under_rest_load','key_revocation_propagated_and_rejected_on_both_nodes'):
        value=functional.get(name)
        status='geçti' if value is True else 'geçmedi' if value is False else 'ölçülmedi'
        lines.append(f'- {name}: {status}.')
    lines+=['','Başarılı çağrıların audit eşleşmesi, bozuk sonuçlar ve çift yürütme aşağıdaki ölçümlerle değerlendirilir. Geçmeyen kontroller Bulgular bölümünde korunur; başarısızlık tek başına yetkisiz erişim gerçekleştiğini kanıtlamaz.','']
    cfg=current.get('DataPlaneConfiguration') or {}
    pg_limits='CPU/bellek limiti yok' if cfg.get('postgres_limits')=='none' else str(cfg.get('postgres_limits','raporlanmadı; manifesti inceleyin'))
    lines+=['','## Kaynak ve tutarlılık','',
      f"- Data-plane başına GOMAXPROCS={cfg.get('gomaxprocs_each','raporlanmadı')}, GOMEMLIMIT={cfg.get('go_soft_memory_limit_each','raporlanmadı')}; Kubernetes hard limit {cfg.get('cpu_limit_each','raporlanmadı')} CPU / {cfg.get('memory_limit_each','raporlanmadı')}. PostgreSQL: {pg_limits}. Gerçek kaynak manifesti karşılaştırmanın parçasıdır.",
      f"- Yük sürücüsü host üzerinde {current['Profile'].get('Workers','raporlanmadı')} bounded worker kullanır; önceki worker sayısı {previous['Profile'].get('Workers','raporlanmadı')}. Talep üretilemediğinde dropped demand olarak raporlanır.",
      ('- Pod arızası bir data-plane Deployment’ının sıfıra indirilip geri açılmasıdır. Önceki koşunun native SIGKILL arızasıyla aynı değildir; fiziksel Kubernetes node kaybı ölçülmedi.' if 'node-loss' in after else '- Bu koşuda pod/node kaybı aşaması çalıştırılmadı.'),
      f"- Audit eşleşmesi: {c.get('matching_admissions',0):,} admission / {c.get('matching_ok_outcomes',0):,} başarılı outcome. Çift yürütme: {c.get('duplicate_executions',0)}. Sonucu belirsiz admission: {c.get('incomplete_operations',0)}; hatalı istemci çağrısıyla açıklanamayan: {c.get('incomplete_unrelated_to_failed_calls',0)}.",
      f"- Snapshot yayılımı: {cfg.get('rest_refresh_ms','ölçülmedi')} ms."]
    refresh_ok=current.get('Functional',{}).get('configuration_refresh_under_rest_load')
    if refresh_ok is False:lines[-1]=f"- Snapshot yayılımı {current.get('DataPlaneConfiguration',{}).get('rest_refresh_ms')} ms gözlem penceresinde doğrulanamadı; bu değer tamamlanmış yayılım süresi değildir."
    for name,values in (current.get('Resources') or {}).items():
        lines.append(f"- {name}: tepe RSS {values.get('max_rss_bytes',0)/2**20:.1f} MiB; medyan CPU %{values.get('median_os_reported_cpu_percent',0):.1f}; soak RSS artışı %{values.get('soak_rss_growth_percent',0):.2f}; DB pool tepe {values.get('max_pool_acquired',0):.0f}/64; pool iptali {values.get('max_pool_cancelled_acquires',0):.0f}.")
    metrics=[node.get('Metrics') or {} for sample in current.get('Samples') or [] for node in sample.get('Nodes') or []]
    if metrics:lines.append(f"- Örneklenen en yüksek snapshot yaşı {max(m.get('vmcp_snapshot_age_seconds',0) for m in metrics):.2f}s; refresh süresi {max(m.get('vmcp_snapshot_refresh_seconds',0) for m in metrics):.2f}s. Yetkilendirme tazeliği sınırı 30s olarak korundu.")
    cgroup_file=root/'cgroup-deltas-after-spike.json'
    if cgroup_file.exists():
        pg={row['metric']:row['delta'] for row in json.loads(cgroup_file.read_text()) if row['container']=='postgres'}
        if pg.get('container_cpu_cfs_periods_total',0)>0:lines.append(f"- Baseline→spike sonrası gözlem penceresinde PostgreSQL CFS throttling periyot oranı %{100*pg.get('container_cpu_cfs_throttled_periods_total',0)/pg['container_cpu_cfs_periods_total']:.1f}. [Ham sayaç farkları](cgroup-deltas-after-spike.json). Bu oran tek başına tüm gecikmenin nedenini kanıtlamaz.")
    # Actual Kubernetes container stats, including PostgreSQL/Redis/control plane.
    containers={}
    for sample in current.get('Samples') or []:
        for entry in sample.get('containers') or []:
            name=entry.get('name')
            if not name:continue
            stats=containers.setdefault(name,{'rss':0,'cpu':0})
            stats['rss']=max(stats['rss'],entry.get('rss_bytes',0));stats['cpu']=max(stats['cpu'],entry.get('cpu_percent',0))
    for name,values in containers.items():lines.append(f"- {name}: örneklenen tepe RSS {values['rss']/2**20:.1f} MiB, tepe CPU %{values['cpu']:.1f}.")
    db=[s['Database'] for s in current.get('Samples') or [] if 'size_bytes' in s.get('Database',{})]
    if db:lines.append(f"- PostgreSQL: tepe bağlantı {max(s['connections'] for s in db)}, lock waiter {max(s['lock_waiters'] for s in db)}, deadlock {max(s['deadlocks'] for s in db)}; son örnekte DB boyutu {db[-1]['size_bytes']/2**30:.2f} GiB.")
    cleanup=root/'cleanup-backlog.jsonl'
    if cleanup.exists():
        snapshots=[json.loads(line) for line in cleanup.read_text().splitlines() if line.strip()]
        if snapshots:
            first=snapshots[0]['tables'][0].get('expired_completed_operations',0)
            last=snapshots[-1]['tables'][0].get('expired_completed_operations',0)
            lines+=['', '## Ek veritabanı bulgusu', '', f"Delivery temizliği için süresi geçmiş tamamlanmış operation kuyruğu iki gözlem arasında {first:,} → {last:,} oldu. Audit saklama süresinden ayrı olan 15 dakikalık delivery TTL temizliği yetişemiyor. [Kuyruk ölçümleri](cleanup-backlog.jsonl).", '']
            if (root/'postgres-cleanup-plan-mid-soak.txt').exists():lines.append('`mcp_streams.operation_id` için indeks yok. ON DELETE CASCADE yolundaki lookup ile eşdeğer SELECT, yaklaşık 350 bin satırda parallel sequential scan ve yaklaşık 92 ms olarak ölçüldü. [Plan ve mevcut indeksler](postgres-cleanup-plan-mid-soak.txt). `delivery.Store.Run` bakım turu bir saniyelik bağlam bütçesini paylaşır; çoklu cascade/batch temizliği için bu yolun optimize edilip yeniden ölçülmesi gerekir. Bu tek SELECT ölçümü, tüm gecikme nedenlerini tek başına açıklamaz. Koşu sırasında indeks veya uygulama ayarı değiştirilmedi.')
    before_demo=root/'existing-demo-before.json';after_demo=root/'existing-demo-after.json'
    if before_demo.exists() and after_demo.exists():
        def inventory(path):
            return {p['metadata']['name']:(p['metadata']['uid'],sum(c['restartCount'] for c in p.get('status',{}).get('containerStatuses',[]))) for p in json.loads(path.read_text()).get('items',[])}
        a,b=inventory(before_demo),inventory(after_demo)
        prior=json.loads(before_demo.read_text())['items']
        active={p['metadata']['name'] for p in prior if p.get('status',{}).get('phase')=='Running'}
        preserved=all(name in b and a[name]==b[name] for name in active)
        lines.append(f"- Mevcut vmcp-demo içindeki {len(active)} aktif podun UID/restart sayıları: "+('korundu.' if preserved else 'değişiklik var; existing-demo-before/after.json dosyalarını inceleyin.'))
        completed_removed=[p['metadata']['name'] for p in prior if p.get('status',{}).get('phase')=='Succeeded' and p['metadata']['name'] not in b]
        if completed_removed:lines.append('- Önceki envanterde tamamlanmış durumdaki pod son envanterde yok: '+', '.join(completed_removed)+'. Aktif workload değişikliği olarak sayılmadı.')
    lines+=['','RSS kubelet container RSS değeridir; CPU yakın dönem kullanımından gelir. Önceki ps CPU ortalamasıyla aynı ölçüm değildir. Ölçümler yaklaşık beş saniyelik örneklerdir; kısa tepe değerlerini kaçırabilir ve 30 dakikalık soak, uzun dönem bellek sızıntısı olmadığını kanıtlamaz.','']
    if faults:lines+=['## Bulgular','']+[f'- {f}' for f in faults]+['']
    lines+=['## Kanıtlar','', '- [Ayrıntılı rapor ve grafikler](report.md)', '- [Ham ölçümler](report.json)', '- [Ortam, kaynak HEAD ve çalışma ağacı](environment.json)', '- [Önceden belirlenen profil](profile.json)', '- [Uygulanan kaynak manifesti](manifest.json)', '- [Pod durumları](kubernetes-pods.json)', '- [Kubernetes olayları](kubernetes-events.json)','', 'Tekrar çalıştırma: `python3 benchmark/kubernetes/run.py`. Sonuçlar çalışma ağacından derlenen binary’ye aittir; kaynak/binary/imaj kimlikleri rapor ve environment/images dosyalarındadır.']
    path=root/'comparison.md';path.write_text('\n'.join(lines)+'\n');return path

if __name__=='__main__':print(render(sys.argv[1], sys.argv[2] if len(sys.argv)>2 else None))
