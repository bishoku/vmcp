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
    failures = report.get('Failures') or []
    phases = report.get('Phases') or []
    lines = ['# Ölçek ve dayanıklılık kabul testi', '', f"Başlangıç: {report.get('StartedUTC', '')}. Sonuç: **{'GEÇTİ' if report.get('Pass') else 'BULGU VAR / KABUL EŞİKLERİ KARŞILANMADI'}**.", '']
    if smoke:
        lines += ['Bu kısa koşu yalnızca test düzeneğinin fonksiyonel doğrulamasıdır; kapasite kabulü değildir.', '']
    if profile.get('REST'):
        lines += ['Profil: **REST upstream**. Kaydedilmiş JSON operasyonları, request/response şema doğrulaması ve mevcut MCP istemci yolu kullanıldı.', f"Trafik altında iki node'a snapshot yayılımı: **{report.get('DataPlaneConfiguration', {}).get('rest_refresh_ms', 'ölçülmedi')} ms**.", '']
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
    if chart(root/'throughput.svg', 'Çağrı hızı ve node kapanışı', [('Başarılı çağrı/s','#4f46e5',throughput),('Hedef çağrı/s','#94a3b8',target),('Hata + gönderilemeyen/s','#dc2626',errors)], 'çağrı / saniye', markers):
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
        lines += ['', f"PostgreSQL: örneklenen bağlantı tepe sayısı **{max(d['connections'] for d in db_samples)} / 200**, deadlock **{max(d['deadlocks'] for d in db_samples)}**, lock bekleyen oturum tepe sayısı **{max(d['lock_waiters'] for d in db_samples)}**. Test veritabanı boyutu ilk/son örnekte **{db_samples[0]['size_bytes']/1048576:.1f} / {db_samples[-1]['size_bytes']/1048576:.1f} MiB**. Audit ve dayanıklı delivery kayıtları bu büyümeye dahildir.", '']
    docker = [v for s in samples for v in s.get('containers') or []]
    lines += ['', 'RSS yaklaşık beş saniyede bir örneklenir; Go heap ile aynı değildir. CPU yüzdesi işletim sisteminin süreç için raporladığı değerdir; birden çok çekirdekte %100’ü aşabilir. Node yeniden başladığında sayaçlar sıfırlanır. Bu profilde rate-limit değerleri sınırsızdır; Redis limiter maliyeti ölçülmemiştir. Kısa soak süresi uzun dönemli bellek sızıntısının olmadığını kanıtlamaz.', '', f"Kaynak örneği: {len(samples)}. Konteyner kaynağı örneği: {len(docker)}. PostgreSQL connection, lock wait, deadlock, transaction ve disk boyutu örnekleri ham raporda bulunur. Metrik etiket taşmaları beklenen cardinality sınırıdır; tüm tool’lara ayrı seri ayrıldığı anlamına gelmez.", '', '## Eşikler ve sınırlamalar', '', '- Sabit yük: hedefin en az %95’i, gönderilenlerde en az %99,9 başarı; p95 ≤250 ms, p99 ≤1000 ms.', '- Yük artışı: hedefin en az %80’i, en az %80 başarı; p99 <10 saniye ve bozuk sonuç olmaması.', '- Node geçişi: aşamada en az %99 başarı; 10 saniyelik geçişten sonra hata ≤%0,1, p99 ≤1000 ms. Pasif sağlık kontrolü isteğinin hatası da sayılır.', '- Node RSS ≤1,5 GiB; soak ilk/son üçte birlik medyanları arasında artış ≤%20.', '- Yerel, tek fiziksel makine ve tek PostgreSQL örneği. Üretim ağları, JWT/IdP yükü, kontrol düzlemi discovery/config churn, uzun çalışan gerçek araçlar ve veritabanı HA dahil değildir.', '']
    if not smoke and report.get('Pass'):
        total = sum(p['Scheduled'] for p in phases)
        lines += ['', '## Değerlendirme', '', f"Bu yerel profil, önceden belirlenen kabul eşiklerini karşıladı. Toplam {total:,} planlanan isteğin {c.get('successful_client_calls',0):,} adedi doğru yanıt ve kalıcı audit eşleşmesiyle tamamlandı. Tüm {c.get('active_virtual_servers',0)} virtual server ve {c.get('active_upstreams',0)} upstream gerçek çağrı aldı; {c.get('active_virtual_tools',0):,} farklı virtual tool çalıştırıldı.", '', 'Mevcut yapı bu profil için yeterli kapasite payı gösteriyor; ikinci optimizasyon paketini zorunlu kılan bir darboğaz gözlenmedi. Bu sonuç mikro tuning yerine kod basitliğini korumayı destekliyor. İlk mikrobenchmarkdan farklı olarak bu koşu tam üretim data-plane’inin dayanıklı MCP delivery/SSE yolunu da çalıştırır; bu iki koşunun gecikmeleri doğrudan karşılaştırılmamalı. Üretim boyutlandırması için farklı gerçek upstream gecikmeleri, JWT/rate-limit politikaları ve ingress ayrıca doğrulanmalı.', '']
        if c.get('incomplete_operations'):
            lines += [f"Node kapanışından sonra {c['incomplete_operations']} çağrının audit outcome’u yoktur. Bu kayıt hatalı istemci isteğiyle eşleşir; yürütme sonucu belirsizdir. Başarılı sayılmadı ve otomatik tekrar yürütülmedi. İş açısından yan etkili gerçek tool kullanımlarında bu belirsizlik ayrıca uzlaştırılmalıdır.", '']
    if failures:
        lines += ['## Kabul bulguları', ''] + [f'- {f}' for f in failures] + ['']
    lines += ['## Tekrarlanabilirlik', '', f"Kaynak HEAD: `{report.get('SourceRevision','')}`; çalışma ağacındaki değişiklikleri içeren gerçek binary SHA-256: `{report.get('BinarySHA256','')}`.", f"{report.get('GoVersion','')}, {report.get('HostOS','')}/{report.get('HostArch','')}. Ham örnekler ve sayaçlar: [report.json](report.json).", '', '```sh', ('VMCP_ACCEPTANCE_REST=1 ' if profile.get('REST') else '') + './scripts/acceptance-core.sh', '```', '']
    (root/'report.md').write_text('\n'.join(lines), encoding='utf-8')
    return root/'report.md'


if __name__ == '__main__':
    print(render(sys.argv[1]))
