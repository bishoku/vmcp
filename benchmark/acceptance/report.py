#!/usr/bin/env python3
"""Render a portable acceptance report from the harness JSON, without dependencies."""
import html
import json
import sys
from pathlib import Path


def chart(path, title, series, ylabel, markers=()):
    points = [point for _, _, values in series for point in values]
    if not points:
        return False
    left, top, width, height = 72, 54, 940, 260
    xmax = max(1, max(x for x, _ in points))
    ymax = max(1, max(y for _, y in points) * 1.12)
    parts = [f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1040 385" role="img" aria-label="{html.escape(title)}">', '<rect width="1040" height="385" rx="16" fill="#f8fafc"/>', f'<text x="24" y="30" font-family="sans-serif" font-size="18" fill="#0f172a">{html.escape(title)}</text>']
    for i in range(5):
        y = top + height - i * height / 4
        value = ymax * i / 4
        parts += [f'<path d="M{left} {y}H{left+width}" stroke="#e2e8f0"/>', f'<text x="{left-8}" y="{y+4}" text-anchor="end" font-family="sans-serif" font-size="12" fill="#475569">{value:.0f}</text>']
    for i in range(6):
        x = left + width * i / 5
        parts.append(f'<text x="{x}" y="{top+height+22}" text-anchor="middle" font-family="sans-serif" font-size="12" fill="#475569">{xmax*i/5:.0f}s</text>')
    for second, label in markers:
        x = left + second / xmax * width
        parts += [f'<path d="M{x} {top}V{top+height}" stroke="#dc2626" stroke-dasharray="5 4"/>', f'<text x="{min(x+4, left+width-125)}" y="{top+12}" font-family="sans-serif" font-size="11" fill="#b91c1c">{html.escape(label)}</text>']
    for index, (label, color, values) in enumerate(series):
        data = ' '.join(f'{left+x/xmax*width:.2f},{top+height-y/ymax*height:.2f}' for x, y in values)
        parts += [f'<polyline points="{data}" fill="none" stroke="{color}" stroke-width="2"/>', f'<text x="{24+index*250}" y="365" font-family="sans-serif" font-size="12" fill="{color}">{html.escape(label)}</text>']
    parts += [f'<text x="{left}" y="{top-10}" font-family="sans-serif" font-size="11" fill="#475569">{html.escape(ylabel)}</text>', '</svg>']
    path.write_text('\n'.join(parts), encoding='utf-8')
    return True


def render(source):
    source = Path(source)
    report = json.loads(source.read_text())
    root = source.parent
    profile = report['Profile']
    smoke = profile.get('Smoke', False)
    kubernetes = report.get('DataPlaneConfiguration',{}).get('platform') == 'MicroK8s Kubernetes'
    failures = report.get('Failures') or []
    phases = report.get('Phases') or []
    lines = ['# Ölçek ve dayanıklılık kabul testi', '', f"Başlangıç: {report.get('StartedUTC', '')}. Sonuç: **{'GEÇTİ' if report.get('Pass') else 'BULGU VAR / KABUL EŞİKLERİ KARŞILANMADI'}**.", '']
    if smoke:
        lines += ['Bu kısa koşu yalnızca test düzeneğinin fonksiyonel doğrulamasıdır; kapasite kabulü değildir.', '']
    if profile.get('REST'):
        refresh_ms = report.get('DataPlaneConfiguration', {}).get('rest_refresh_ms')
        refresh_ok = report.get('Functional', {}).get('configuration_refresh_under_rest_load')
        refresh_text = f"Trafik altında iki node'a snapshot yayılımı: **{refresh_ms} ms**." if refresh_ok else f"Snapshot yayılımı **{refresh_ms} ms gözlem penceresinde doğrulanamadı**." if refresh_ok is False else 'Snapshot yayılımı bu koşuda ölçülmedi.'
        lines += ['Profil: **REST upstream**. Kaydedilmiş JSON operasyonları, request/response şema doğrulaması ve mevcut MCP istemci yolu kullanıldı.', refresh_text, '']
    lines += [f"Envanter: {profile['Servers']} upstream, {profile['Servers']} virtual server, {profile['Servers']*profile['Tools']:,} tool tanımı/seçimi, {profile['Servers']*profile['Resources']:,} resource tanımı/seçimi; {profile['Sessions']} kalıcı session, 64 yapılandırılmış agent key.", '', 'İki ayrı gerçek data-plane süreci; her birinde GOMAXPROCS=2, 768 MiB Go yumuşak bellek limiti ve 64 bağlantılık DB pool. Ayrı PostgreSQL 17 konteyneri 2 CPU / 2 GiB; ayrı Redis konteyneri 0,5 CPU / 256 MiB. Ana uygulama ortamına dokunulmadı.', '', f"{profile['Servers']} mantıksal upstream adresi 16 sentetik HTTP fixture dinleyicisine dağıtılır. Tool yanıtı 5 ms geciktirilir. Her yanıtta doğru upstream rotası, tool adı, çağrı nonce’u ve PII maskesi doğrulanır. Bu gerçek upstream servislerinin kapasite veya ağ gecikmesi testi değildir.", '', 'Test proxy’si round-robin dağıtır, arızalı node için 10 saniyelik pasif bekleme uygular ve POST isteğini başka node’a tekrar göndermez. Üretim ingress’i/nginx bu koşunun parçası değildir. Node kaybı SIGKILL ile yapılır; PostgreSQL HA/failover kapsam dışıdır.', '', '## Aşamalar', '', '| Aşama | Hedef çağrı/s | Başarılı çağrı/s | Başarı % | p95 ms | p99 ms | Hata | Gönderilemeyen | Eşik |', '|---|---:|---:|---:|---:|---:|---:|---:|---|']
    throughput, target, errors, markers = [], [], [], []
    offset = 0
    phase_names = {'warmup':'Isınma','steady':'Sabit yük','spike':'Ani yük artışı','recovery':'Yük sonrası toparlanma','node-loss':'Node kaybı','node-rejoin':'Node geri dönüşü','soak':'Süreklilik'}
    for p in phases:
        lines.append(f"| {phase_names.get(p['name'],p['name'])} | {p['target_rps']} | {p['CompletedRPS']:.1f} | {p['SuccessPercent']:.3f} | {p['P95MS']:.1f} | {p['P99MS']:.1f} | {p['Errors']} | {p['Dropped']} | {'Geçti' if p['slo_pass'] else 'Karşılanmadı'} |")
        for b in p.get('Seconds') or []:
            x = offset + b['second']
            throughput.append((x, b['Success']))
            target.append((x, p['target_rps'] if b['second'] < p['PlannedSeconds'] else 0))
            errors.append((x, b['Errors'] + b['Dropped']))
        if p.get('fault_second') is not None:
            markers.append((offset+p['fault_second'], 'Konfigürasyon güncellemesi' if p['name'] == 'config-refresh' else 'Node kapanışı'))
        if p['name'] == 'node-rejoin':
            markers.append((offset, 'Node geri dönüşü'))
        offset += p['ElapsedSeconds']
    lines += ['', 'Yüzdelikler yalnızca başarılı çağrıların servis gecikmesidir; hatalar ayrıca sayılır. Başarı oranı gönderilen isteklere göre, çağrı/s ise kuyruk boşaltma dahil gerçek aşama süresine göre hesaplanır. Gönderilemeyen istekler de hedefin karşılanması değerlendirmesine dahildir. İstemci retry yapmaz.', '']
    if chart(root/'throughput.svg', 'Çağrı hızı ve hata/gönderilemeyen talep' if report.get('DataPlaneConfiguration', {}).get('database_focus') else 'Çağrı hızı ve node kapanışı', [('Başarılı çağrı/s','#4f46e5',throughput),('Hedef çağrı/s','#94a3b8',target),('Hata + gönderilemeyen/s','#dc2626',errors)], 'çağrı / saniye', markers):
        lines += ['![Çağrı hızı](throughput.svg)', '']
    lines += ['## Tutarlılık ve güvenlik', '']
    c = report.get('Consistency') or {}
    for name, label in [('successful_client_calls','Başarılı istemci çağrısı'),('matching_admissions','Eşleşen kalıcı audit admission'),('matching_ok_outcomes','Eşleşen başarılı audit outcome'),('duplicate_executions','Çift upstream çalıştırma'),('incomplete_operations','Outcome’u olmayan admission'),('incomplete_unrelated_to_failed_calls','Hatalı istemci çağrısıyla açıklanamayan eksik outcome'),('active_upstreams','Çağrı alan farklı upstream'),('active_virtual_servers','Çağrı alan farklı virtual server'),('active_virtual_tools','Çağrı alan farklı virtual tool'),('active_agent_keys','Çağrı yapan farklı agent key')]:
        lines.append(f"- {label}: **{c.get(name, 'ölçülemedi')}**.")
    lines += ['', 'Node kapanışında kalıcı admission’dan sonra outcome yazılamamış olabilir. Bunlar yalnızca hatalı istemci isteğiyle eşleşiyorsa açıklanmış belirsiz işlemler olarak tutulur; başarılı sayılmaz veya yeniden çalıştırılmaz. Başarılı her istemci yanıtı için admission ve başarılı outcome ayrı ayrı doğrulanır.', '']
    for name, passed in (report.get('Functional') or {}).items():
        lines.append(f"- `{name}`: {'geçti' if passed else 'karşılanmadı'}.")
    lines += ['', '## Kaynak kullanımı', '', '| Node | En yüksek RSS MiB | Medyan OS CPU % | Örneklenen DB pool tepe kullanımı | Pool edinim iptali (gözlenen en yüksek sayaç) | Soak RSS değişimi % |', '|---|---:|---:|---:|---:|---:|']
    resources = report.get('Resources') or {}
    for node, values in resources.items():
        growth = values.get('soak_rss_growth_percent')
        lines.append(f"| {node} | {values['max_rss_bytes']/1048576:.1f} | {values['median_os_reported_cpu_percent']:.1f} | {values['max_pool_acquired']:.0f}/64 | {values['max_pool_cancelled_acquires']:.0f} | {f'{growth:.1f}' if growth is not None else 'örnek yetersiz'} |")
    memory = []
    for i, name in enumerate(resources):
        points = [(s['Seconds'],n['RSSBytes']/1048576) for s in report.get('Samples') or [] for n in s['Nodes'] if n['Name']==name and n['Alive'] and n['RSSBytes']>0]
        memory.append((name, ['#4f46e5','#0891b2'][i%2], points))
    if chart(root/'memory.svg', 'Data-plane RSS', memory, 'MiB'):
        lines += ['', '![Bellek](memory.svg)']
    samples = report.get('Samples') or []
    db_samples = [s['Database'] for s in samples if 'connections' in s.get('Database',{})]
    if db_samples:
        lines += ['', f"PostgreSQL: örneklenen bağlantı tepe sayısı **{max(d['connections'] for d in db_samples)} / {256 if kubernetes else 200}**, deadlock **{max(d['deadlocks'] for d in db_samples)}**, lock bekleyen oturum tepe sayısı **{max(d['lock_waiters'] for d in db_samples)}**. Test veritabanı boyutu ilk/son örnekte **{db_samples[0]['size_bytes']/1048576:.1f} / {db_samples[-1]['size_bytes']/1048576:.1f} MiB**. Audit ve dayanıklı delivery kayıtları bu büyümeye dahildir.", '']
    docker = [v for s in samples for v in s.get('containers') or []]
    lines += ['', 'RSS yaklaşık beş saniyede bir örneklenir; Go heap ile aynı değildir. CPU yüzdesi işletim sisteminin süreç için raporladığı değerdir; birden çok çekirdekte %100’ü aşabilir. Node yeniden başladığında sayaçlar sıfırlanır. Bu profilde rate-limit değerleri sınırsızdır; Redis limiter maliyeti ölçülmemiştir. Kısa soak süresi uzun dönemli bellek sızıntısının olmadığını kanıtlamaz.', '', f"Kaynak örneği: {len(samples)}. Konteyner kaynağı örneği: {len(docker)}. PostgreSQL connection, lock wait, deadlock, transaction ve disk boyutu örnekleri ham raporda bulunur. Metrik etiket taşmaları beklenen cardinality sınırıdır; tüm tool’lara ayrı seri ayrıldığı anlamına gelmez.", '', '## Eşikler ve sınırlamalar', '', f"- Sabit yük: hedefin en az %{report.get('Thresholds', {}).get('steady_min_target_completion_percent', 95)}’i, gönderilenlerde en az %99,9 başarı; p95 ≤250 ms, p99 ≤1000 ms.", '- Yük artışı: hedefin en az %80’i, en az %80 başarı; p99 <10 saniye ve bozuk sonuç olmaması.', '- Node geçişi: aşamada en az %99 başarı; 10 saniyelik geçişten sonra hata ≤%0,1, p99 ≤1000 ms. Pasif sağlık kontrolü isteğinin hatası da sayılır.', f"- Node RSS ≤{report.get('Thresholds', {}).get('per_node_max_rss_bytes', 1536 << 20) / (1 << 30):g} GiB; soak/etkin steady bellek büyümesi ≤%20. Container usage bütçesi: {report.get('Thresholds', {}).get('per_node_max_container_usage_bytes', 'tanımlı değil')} byte.", '- Yerel, tek fiziksel makine ve tek PostgreSQL örneği. Üretim ağları, JWT/IdP yükü, kontrol düzlemi discovery/config churn, uzun çalışan gerçek araçlar ve veritabanı HA dahil değildir.', '']
    if not smoke and report.get('Pass'):
        total = sum(p['Scheduled'] for p in phases)
        lines += ['', '## Değerlendirme', '', f"Bu yerel profil, önceden belirlenen kabul eşiklerini karşıladı. Toplam {total:,} planlanan isteğin {c.get('successful_client_calls',0):,} adedi doğru yanıt ve kalıcı audit eşleşmesiyle tamamlandı. Tüm {c.get('active_virtual_servers',0)} virtual server ve {c.get('active_upstreams',0)} upstream gerçek çağrı aldı; {c.get('active_virtual_tools',0):,} farklı virtual tool çalıştırıldı.", '', 'Mevcut yapı bu profil için yeterli kapasite payı gösteriyor; ikinci optimizasyon paketini zorunlu kılan bir darboğaz gözlenmedi. Bu sonuç mikro tuning yerine kod basitliğini korumayı destekliyor. İlk mikrobenchmarkdan farklı olarak bu koşu tam üretim data-plane’inin dayanıklı MCP delivery/SSE yolunu da çalıştırır; bu iki koşunun gecikmeleri doğrudan karşılaştırılmamalı. Üretim boyutlandırması için farklı gerçek upstream gecikmeleri, JWT/rate-limit politikaları ve ingress ayrıca doğrulanmalı.', '']
        if c.get('incomplete_operations'):
            lines += [f"Node kapanışından sonra {c['incomplete_operations']} çağrının audit outcome’u yoktur. Bu kayıt hatalı istemci isteğiyle eşleşir; yürütme sonucu belirsizdir. Başarılı sayılmadı ve otomatik tekrar yürütülmedi. İş açısından yan etkili gerçek tool kullanımlarında bu belirsizlik ayrıca uzlaştırılmalıdır.", '']
    if failures:
        lines += ['## Kabul bulguları', ''] + [f'- {f}' for f in failures] + ['']
    lines += ['## Tekrarlanabilirlik', '', f"Kaynak HEAD: `{report.get('SourceRevision','')}`; çalışma ağacındaki değişiklikleri içeren gerçek binary SHA-256: `{report.get('BinarySHA256','')}`.", f"{report.get('GoVersion','')}, {report.get('HostOS','')}/{report.get('HostArch','')}. Ham örnekler ve sayaçlar: [report.json](report.json).", '', '```sh', ('VMCP_ACCEPTANCE_REST=1 ' if profile.get('REST') else '') + './scripts/acceptance-core.sh', '```', '']
    if kubernetes:
        replacements = {
            'İki ayrı gerçek data-plane süreci; her birinde GOMAXPROCS=2, 768 MiB Go yumuşak bellek limiti ve 64 bağlantılık DB pool. Ayrı PostgreSQL 17 konteyneri 2 CPU / 2 GiB; ayrı Redis konteyneri 0,5 CPU / 256 MiB. Ana uygulama ortamına dokunulmadı.': 'MicroK8s üzerinde ayrı test namespace’i: iki data-plane podu ve bir control-plane podu. Data plane başına GOMAXPROCS=2, GOMEMLIMIT=768MiB, 64 bağlantılık pool; Kubernetes CPU limiti 4, bellek limiti 3GiB. PostgreSQL limiti 4 CPU / 4GiB, Redis 1 CPU / 512MiB. Envanter ve yük sürücüsü host üzerinde; PostgreSQL/Redis ve uygulama Kubernetes üzerinde. Mevcut vmcp-demo değiştirilmedi.',
            'Node kaybı SIGKILL ile yapılır; PostgreSQL HA/failover kapsam dışıdır.': 'Tek data-plane Deployment sıfır replikaya indirilip yeniden açılır. Bu fiziksel Kubernetes node arızası veya zorunlu SIGKILL değildir; PostgreSQL HA/failover kapsam dışıdır.',
            'CPU yüzdesi işletim sisteminin süreç için raporladığı değerdir; birden çok çekirdekte %100’ü aşabilir.': 'RSS kubelet container RSS değeridir. CPU, kubelet usageNanoCores değerinden hesaplanan yakın dönem kullanımdır; önceki native ps ortalamasıyla aynı ölçüm değildir ve birden çok çekirdekte %100’ü aşabilir.',
            'Kısa soak süresi uzun dönemli bellek sızıntısının olmadığını kanıtlamaz.': 'Bu süre uzun dönemli bellek sızıntısının olmadığını kanıtlamaz.',
            'Mevcut yapı bu profil için yeterli kapasite payı gösteriyor; ikinci optimizasyon paketini zorunlu kılan bir darboğaz gözlenmedi. Bu sonuç mikro tuning yerine kod basitliğini korumayı destekliyor. İlk mikrobenchmarkdan farklı olarak bu koşu tam üretim data-plane’inin dayanıklı MCP delivery/SSE yolunu da çalıştırır; bu iki koşunun gecikmeleri doğrudan karşılaştırılmamalı. Üretim boyutlandırması için farklı gerçek upstream gecikmeleri, JWT/rate-limit politikaları ve ingress ayrıca doğrulanmalı.': 'Bu sonuç yalnızca belirtilen Kubernetes profili ve sentetik upstream gecikmesi için geçerlidir. Artan RPS ve süre, önceki Compose koşusundan farklıdır; CPU, bellek ve ağ altyapısı farkları yazılım hızlanması olarak yorumlanmamalıdır. Gerçek servis gecikmeleri, JWT/rate-limit politikaları ve üretim ingress’i ayrıca doğrulanmalıdır.',
            './scripts/acceptance-core.sh': 'python3 benchmark/kubernetes/run.py' + (' --smoke' if smoke else ''),
            'Node kaybı': 'Pod devreden çıkarma',
            'Node geri dönüşü': 'Pod geri dönüşü',
        }
        lines = [line for line in lines]
        for i,line in enumerate(lines):
            for old,new in replacements.items():line=line.replace(old,new)
            lines[i]=line
        lines[0]='# MicroK8s üzerinde ölçek ve dayanıklılık testi'
    if kubernetes:
        memory = report.get('DataPlaneConfiguration', {}).get('go_soft_memory_limit_each', '768MiB')
        lines = [line.replace('GOMEMLIMIT=768MiB', f'GOMEMLIMIT={memory}') for line in lines]
        hard_memory = report.get('DataPlaneConfiguration', {}).get('memory_limit_each', '3Gi')
        lines = [line.replace('bellek limiti 3GiB', f'bellek limiti {hard_memory}B') for line in lines]
    configuration = report.get('DataPlaneConfiguration', {})
    if kubernetes and configuration.get('nodes') == 1:
        lines = [line.replace('iki data-plane podu', 'tek data-plane podu') for line in lines]
    if configuration.get('loaded_revocation'):
        lines = [line.replace('pod kapatma veya konfigürasyon mutasyonu yapılmaz', 'pod kapatma yapılmaz; yük sırasında ayrı test key iptal edilir') for line in lines]
        loaded = configuration['loaded_revocation']
        lines += ['', '## Yük sırasında key iptali', '', f"İlk 401: {loaded.get('first_401_seconds')} s; bütçe 5 s. Readiness: {loaded.get('readiness_healthy')}; reddin kararlılığı: {loaded.get('rejection_stable')}. Ham prob gözlemleri report.json içindedir."]
    if configuration.get('routing') == 'pod':
        lines += ['', 'Bu kapasite koşusunda host test proxy doğrudan PodIP/CNI yolunu kullanır; NodePort, Service ve üretim ingress kapasitesi ölçülmez.']
    if configuration.get('capacity_search'):
        lines += ['', 'Bu koşu artan yük, daraltma ve son sabit-yük doğrulamasından oluşur. Bilerek sınırın üstünde denenen kapasite aşamaları son doğrulama sonucundan ayrı değerlendirilmelidir.', str(configuration['capacity_search'])]
    cpus = report.get('DataPlaneConfiguration', {}).get('gomaxprocs_each', 2)
    if kubernetes and cpus != 2:
        lines = [line.replace('GOMAXPROCS=2', f'GOMAXPROCS={cpus}').replace('Kubernetes CPU limiti 4', f'Kubernetes CPU limiti {max(4, cpus)}') for line in lines]
    if report.get('DataPlaneConfiguration', {}).get('postgres_limits') == 'none':
        lines = [line.replace('PostgreSQL limiti 4 CPU / 4GiB', 'PostgreSQL CPU/bellek limiti yok; requests 2 CPU / 2GiB') for line in lines]
    if report.get('DataPlaneConfiguration', {}).get('database_focus'):
        lines = [line.replace('PostgreSQL limiti 4 CPU / 4GiB', 'PostgreSQL CPU/bellek limiti yok; requests 2 CPU / 2GiB').replace('Tek data-plane Deployment sıfır replikaya indirilip yeniden açılır. Bu fiziksel Kubernetes node arızası veya zorunlu SIGKILL değildir;', ('Bu odaklı koşuda pod kapatma yapılmaz; yük sırasında ayrı test key iptal edilir;' if configuration.get('loaded_revocation') else 'Bu odaklı koşuda pod kapatma veya konfigürasyon mutasyonu yapılmaz;')).replace('python3 benchmark/kubernetes/run.py', 'python3 benchmark/kubernetes/run.py --database-focus') for line in lines]
        steady_target = next((p['target_rps'] for p in report['Phases'] if p['name'] == 'steady'), profile.get('SteadyRPS', 1000))
        lines.insert(4, f"PostgreSQL odaklı profil: 15 saniye ısınma ve {report.get('DataPlaneConfiguration', {}).get('steady_seconds',300)} saniye {steady_target} TPS hedefi; önceden süresi dolmuş 100.000 operation/stream ile delivery temizliği de çalıştırılır. Bu koşu uzun soak veya fault kabul testi değildir. SQL süreleri ve beklemeler postgres-*.json dosyalarında bulunur.")
    if kubernetes and cpus != 2:
        lines = [line.replace('python3 benchmark/kubernetes/run.py --database-focus',f'python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus {cpus}') for line in lines]
    if report.get('DataPlaneConfiguration', {}).get('postgres_tuned'):
        lines = [line.replace('python3 benchmark/kubernetes/run.py --database-focus','python3 benchmark/kubernetes/run.py --database-focus --postgres-tuned') for line in lines]
        lines += ['', 'PostgreSQL OLTP profili: shared_buffers=1GiB, effective_cache_size=4GiB, WAL buffers=32MiB, max_wal_size=4GiB, checkpoint=15min, daha sık autovacuum/analyze. fsync, synchronous_commit ve full_page_writes açık. Gerçek ayarlar: [postgres-effective-settings.json](postgres-effective-settings.json). Bu profil yalnızca izole benchmark kurulumu içindir.', '']
    probe = report.get('DataPlaneConfiguration', {}).get('remote_poll_probe')
    if probe:
        lines += ['', '## Yük altında farklı process üzerinden polling', '', f"Native host test veritabanına event yazar; gerçek data-plane podu GET SSE üzerinden okur. Local wake paylaşılmaz. Bu, tool çağrısı süresinden ayrı, yazım başlangıcından event’in client tarafından okunmasına kadar ölçümdür. İlk yük penceresinde yaklaşık saniyede bir {probe.get('samples',0)} event; p50={probe.get('p50_ms','ölçülmedi')} ms, p95={probe.get('p95_ms','ölçülmedi')} ms, max={probe.get('max_ms','ölçülmedi')} ms. Kabul bütçesi 500ms. Örnekleme, her koşulda 500ms garantisi değildir.", '']
    if probe and 'connection_attempts' in probe:
        lines += [f"GET bağlantı hazırlığı: {probe['connection_attempts']} deneme, {probe.get('connection_setup_ms','ölçülmedi')} ms. Bu süre event teslimi ölçümünden ayrıdır; tool çağrılarında retry yoktur.", '']
    (root/'report.md').write_text('\n'.join(lines), encoding='utf-8')
    return root/'report.md'


if __name__ == '__main__':
    print(render(sys.argv[1]))
