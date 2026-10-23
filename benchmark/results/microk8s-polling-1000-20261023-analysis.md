# Polling ve PostgreSQL performans doğrulaması

23 Ekim 2026. Dört koşunun da hedefi 300 saniye boyunca 1.000 tool çağrısı/sn. Aynı hostta sırayla çalıştırılan koşularda 300 REST upstream, 300 virtual server, 60.000 tool, 600 session ve 64 key var. İki data-plane podunun her biri GOMAXPROCS=8, GOMEMLIMIT=768MiB, 64 bağlantılık pool ve 8 CPU / 3GiB Kubernetes limitini kullanıyor. Strict metadata audit açık; upstream gecikmesi sentetik 5ms. PostgreSQL CPU/bellek limitsiz. Ingress, JWT/IdP, aktif rate limit ve gerçek upstream kapasitesi kapsam dışı.

| Koşu | Başarılı TPS | p95 ms | p99 ms | Hata | Gönderilemeyen | Steady SLO |
|---|---:|---:|---:|---:|---:|---|
| Önce: NOTIFY + varsayılan PG | 306.6 | 4394.1 | 4716.6 | 19,938 | 187,553 | Geçmedi |
| C: polling + varsayılan PG | 933.4 | 162.0 | 1046.4 | 9,311 | 10,633 | Geçmedi |
| D: polling + optimize PG | 999.8 | 128.9 | 196.3 | 5 | 7 | Geçti |
| E: optimize PG, probe bağlantı hazırlığı düzeltildi | 998.7 | 135.7 | 485.4 | 3 | 358 | Geçti |

TPS kuyruk boşaltma dahil gerçek süreye göre hesaplanır; yüzdelikler başarılı isteklerindir. Gönderilemeyen talep gerçek trafik sayılmaz. Steady kabul eşikleri değişmedi: hedefin ≥%95’i, gönderilenlerde ≥%99,9 başarı, p95≤250ms, p99≤1000ms. 15 saniye 50TPS ısınma tabloya dahil değil.

Son koşuda yük ve 500ms polling kontrolleri geçti. **Genel kabul testi tamamen yeşil değil:** daha önce de görülen büyük katalog/key iptali yayılımı kontrolü hâlâ başarısız. E koşusunda 300.389 başarılı çağrının tüm audit admission/outcome eşleşmeleri doğrulandı.

## Yapılan runtime değişikliği

`vmcp_delivery` PostgreSQL NOTIFY yazımları ve delivery LISTEN bağlantısı kaldırıldı. Kalıcı event/outcome yazımları, fencing, session doğrulama ve replay korundu. Yazım başarıyla commit edildikten sonra aynı Store/pod içindeki okuyucular bounded local kanalla hemen uyandırılır. Başka poddaki SSE okuyucusu 250ms; pending client reply/subscription readiness okuyucusu 50ms aralıkla veritabanını kontrol eder. Config/key/policy snapshot’larının bağımsız vmcp_config LISTEN/NOTIFY yolu değiştirilmedi.

Cross-node regression harness’lerinde podlar artık aynı delivery.Store nesnesini paylaşmıyor. Böylece replay, sampling/client reply ve resource subscription testleri örtük local wake avantajından yararlanmıyor. Subscription failover ve fiziksel upstream bağlantı kopması sonrasında event teslimi de doğrulandı.

## Uzak polling: 500ms bütçesi

| Koşu | Örnek | p50 ms | p95 ms | Max ms | 500ms kontrolü |
|---|---:|---:|---:|---:|---|
| C: polling + varsayılan PG | 24 | 250.044 | 260.127 | 304.337 | Geçti |
| D: polling + optimize PG | 0 | — | — | — | Ölçüm tamamlanamadı / geçmedi |
| E: optimize PG, probe bağlantı hazırlığı düzeltildi | 24 | 219.381 | 250.363 | 256.179 | Geçti |

Probe, yükün ilk penceresinde saniyede yaklaşık bir event yazar. Yazan native host ve okuyan gerçek DP podu farklı process’lerdir; local wake paylaşamazlar. Ölçüm yazım başlangıcından client’ın GET SSE event’ini okumasına kadardır; DB commit ve ağ/scheduling maliyeti dahildir. Bunlar tool yanıtlarıyla karıştırılmamalı. D’de ilk GET bağlantısı reddedildiğinden hiç event ölçülemedi; D’den polling gecikmesi iddiası yapılmaz. E’de yalnızca stream açılışı için en fazla yaklaşık 10 saniye boyunca güvenli GET reconnect uygulanır; connection_attempts ve connection_setup_ms ayrıca kaydedilir. E’de bağlantı ilk denemede 4,608ms’de açıldı; bu tekrar sırasında reconnect gerekmedi. Event yazımı/teslimi veya tool çağrıları retry edilmez; ana yük jeneratörünün hataları aynen sayılır. 24 örnek, her ağ/yük koşulunda 500ms SLA garantisi değildir. Aynı podda tamamlanan normal tool cevabına sabit polling beklemesi eklenmez.

## PostgreSQL kurulumu

Demo ve eski benchmark ayarları doğrudan pg_settings üzerinden okundu: büyük ölçüde PostgreSQL varsayılanlarıydı. Demo max_connections=100, benchmark=256 idi. Demo değiştirilmedi. D/E koşularına yalnızca izole benchmark namespace’inde açık bir OLTP profili uygulandı:

| Ayar | Önceki benchmark | Optimize profil |
|---|---:|---:|
| shared_buffers | 128MiB | 1GiB |
| effective_cache_size | 4GiB | 4GiB (planner tahmini) |
| work_mem | 4MiB | 4MiB |
| maintenance/autovacuum work memory | 64MiB/default | 128MiB |
| wal_buffers | 4MiB | 32MiB |
| min/max_wal_size | 80MiB / 1GiB | 1GiB / 4GiB |
| checkpoint_timeout | 5 dakika | 15 dakika |
| autovacuum_naptime | 60s | 10s |
| vacuum / analyze scale factor | 0,20 / 0,10 | 0,05 / 0,02 |
| /dev/shm | 64MiB | 512MiB tmpfs |
| max_connections | 256 | 256 |
| fsync / synchronous_commit / full_page_writes | on / on / on | on / on / on |

Cache boyutu sınırsız container’ın tüm 157GiB host belleğine göre oranlanmadı; host uygulamalarla paylaşıldığı için 1GiB seçildi. Work memory her işlem/sort için çoğalabildiğinden körlemesine artırılmadı. Disk/root filesystem NVMe/ext4; benchmark veri alanı ephemeral emptyDir, üretim kalıcı disk testi değildir. shared-memory alanının kullanım anında 68MiB’ye çıktığı gözlendi; önceki 64MiB sınırını gerçekten aşıyor. D/E etkin pg_settings ve mount/cgroup bilgileri artifact olarak kaydedildi. PostgreSQL throttle sayacı 0; dayanıklılık gevşetilmedi.

C koşusunda audit tutarlılık sorgusu shared-memory resize hatasıyla başarısız oldu. Bu yüzden C’nin başarılı yanıtlarının tüm audit eşleşmeleri doğrulanmış sayılamaz. Yeni mount ile D’de bu kontrolün gerçek sonucu aşağıda gösterilir.

Bu profil birden fazla ayarı birlikte değiştirir; farkın tamamı shared_buffers’a veya tek bir parametreye atfedilemez. D ayrıca auto_explain ile ≥10ms sorguların %1’ini örnekler (ANALYZE/BUFFERS açık, node TIMING kapalı); bunun da gözlem maliyeti vardır. [PostgreSQL 17 kaynak tüketimi](https://www.postgresql.org/docs/17/runtime-config-resource.html), [WAL/checkpoint ayarları](https://www.postgresql.org/docs/17/runtime-config-wal.html).

## Darboğaz gözlemleri

- Önce: NOTIFY + varsayılan PG: 53 yük örneği; aktif oturum örneklerinin %90.1’i Lock/object. WALWrite oturum örneği 126. Bu dağılım CPU zamanı ya da isteklerin gecikme yüzdesi değildir.
- C: polling + varsayılan PG: 53 yük örneği; aktif oturum örneklerinin %0.0’i Lock/object. WALWrite oturum örneği 655. Bu dağılım CPU zamanı ya da isteklerin gecikme yüzdesi değildir.
- D: polling + optimize PG: 53 yük örneği; aktif oturum örneklerinin %0.0’i Lock/object. WALWrite oturum örneği 763. Bu dağılım CPU zamanı ya da isteklerin gecikme yüzdesi değildir.
- E: optimize PG, probe bağlantı hazırlığı düzeltildi: 53 yük örneği; aktif oturum örneklerinin %0.0’i Lock/object. WALWrite oturum örneği 800. Bu dağılım CPU zamanı ya da isteklerin gecikme yüzdesi değildir.

Önceki NOTIFY/COMMIT lock convoy’u polling koşularında gözlenmedi. Kalan baskın bekleme WAL yazım/senkronizasyon yolunda; kapasite sınırının tamamen ortadan kalktığı iddia edilmiyor. C’de ilk yük penceresinde stream Claim sorgusu çağrı başına 10–21 bin, event okuması yaklaşık 7 bin buffer hit üretti. Sonraki interval sayaçları Claim için yaklaşık 7, event okuması için 4–6 hit’e ve ~0,04–0,07ms’ye düştü. Canlı custom/generic EXPLAIN her ikisinde PK/replay indeksi kullandı. Erken pahalı plan C’de loglanmadığından tam olarak hangi planın seçildiği kanıtlanmadı; ölçüm, istatistik/plan değişimine duyarlı başlangıç davranışını gösteriyor. D’nin daha sık bakım ve genişletilmiş cache/SHM profili birlikte değerlendirilmeli.

## Tutarlılık ve kalan bulgular

- **Önce: NOTIFY + varsayılan PG** — genel test: bulgu var; key revocation yayılımı: False.
  Isınma dahil başarılı 93,259; matching admission 93,259, outcome 93,259; duplicate=0, incomplete=0.
  Bulgu: steady capacity/latency SLO failed
  Bulgu: revoked key accepted or snapshot propagation timed out on acceptance-node-1
  Bulgu: revoked key accepted or snapshot propagation timed out on acceptance-node-2
- **C: polling + varsayılan PG** — genel test: bulgu var; key revocation yayılımı: False.
  Audit tutarlılık sonucu üretilemedi; başarılı audit eşleşmesi iddiası yapılmıyor.
  Bulgu: steady capacity/latency SLO failed
  Bulgu: ERROR: could not resize shared memory segment "/PostgreSQL.3490393770" to 196736 bytes: No space left on device (SQLSTATE 53100)
  Bulgu: revoked key accepted or snapshot propagation timed out on acceptance-node-1
  Bulgu: revoked key accepted or snapshot propagation timed out on acceptance-node-2
- **D: polling + optimize PG** — genel test: bulgu var; key revocation yayılımı: False.
  Isınma dahil başarılı 300,738; matching admission 300,738, outcome 300,738; duplicate=0, incomplete=0.
  Bulgu: Get "http://192.168.1.106:31798/mcp/v1/virtual/accept-vs-0299": dial tcp 192.168.1.106:31798: connect: connection refused
  Bulgu: revoked key accepted or snapshot propagation timed out on acceptance-node-1
  Bulgu: revoked key accepted or snapshot propagation timed out on acceptance-node-2
- **E: optimize PG, probe bağlantı hazırlığı düzeltildi** — genel test: bulgu var; key revocation yayılımı: False.
  Isınma dahil başarılı 300,389; matching admission 300,389, outcome 300,389; duplicate=0, incomplete=0.
  Bulgu: revoked key accepted or snapshot propagation timed out on acceptance-node-1
  Bulgu: revoked key accepted or snapshot propagation timed out on acceptance-node-2

Key iptali kontrolünde iki snapshot revizyonunun 30 saniyede doğrulanamaması, tek başına iptal edilmiş anahtarla yetkisiz işlemin kabul edildiğinin kanıtı değildir. Büyük katalog yeniden yüklemesi / beş saniyelik snapshot refresh bütçesi önceki koşularda da bulguydu. Performans ve polling kontrollerinin geçmesi, bu ayrı bulgunun çözüldüğü anlamına gelmez.

Her koşu 100.000 eski operation/stream ile bakım yolunu çalıştırır. Yeni kayıtlar beş dakikada 15 dakikalık TTL’ye ulaşmadığından uzun vadeli retention kapasitesi doğrulanmaz. Mevcut iki worker × 200 kayıt/tur × ~1tur/sn yaklaşık 400 operation/sn ideal temizleme tavanına sahiptir. Gerçek 1.000 TPS sürekliliği için batch/bakım bütçesi ve indeks sırasını kullanan seçimler ayrıca iyileştirilmeli. Beş dakikalık D testi 15 dakikalık checkpoint ayarının tekrarlı döngülerini de kapsamaz; uzun koşu gereklidir.

## Doğrulama ve kanıtlar

- Önce: NOTIFY + varsayılan PG: [rapor](microk8s-postgres-1000-20261023-b/report.md), [ham ölçümler — özet](microk8s-postgres-1000-20261023-b/summary.json), [SQL örnekleri — yerel arşiv](README.md#raw-artifacts), [entegrasyon testleri — yerel arşiv](README.md#raw-artifacts). Binary SHA-256: `ff16642693740606e9d0f42bc1fd72bdc3c50b8a6f1d2338303cbed18889516e`.
- C: polling + varsayılan PG: [rapor](microk8s-polling-1000-20261023-c/report.md), [ham ölçümler — özet](microk8s-polling-1000-20261023-c/summary.json), [SQL örnekleri — yerel arşiv](README.md#raw-artifacts), [entegrasyon testleri — yerel arşiv](README.md#raw-artifacts). Binary SHA-256: `7bf9ad921e93022a299b5cdd8b59ec2257bea1780605cc7a808aab7fc42091b6`.
- D: polling + optimize PG: [rapor](microk8s-polling-1000-20261023-d/report.md), [ham ölçümler — özet](microk8s-polling-1000-20261023-d/summary.json), [SQL örnekleri — yerel arşiv](README.md#raw-artifacts), [entegrasyon testleri — yerel arşiv](README.md#raw-artifacts). Binary SHA-256: `7bf9ad921e93022a299b5cdd8b59ec2257bea1780605cc7a808aab7fc42091b6`.
- E: optimize PG, probe bağlantı hazırlığı düzeltildi: [rapor](microk8s-polling-1000-20261023-e/report.md), [ham ölçümler — özet](microk8s-polling-1000-20261023-e/summary.json), [SQL örnekleri — yerel arşiv](README.md#raw-artifacts), [entegrasyon testleri — yerel arşiv](README.md#raw-artifacts). Binary SHA-256: `7bf9ad921e93022a299b5cdd8b59ec2257bea1780605cc7a808aab7fc42091b6`.
- [D etkin PostgreSQL ayarları](microk8s-polling-1000-20261023-d/diagnostics.json)
- [D çalışma anındaki mount ve cgroup bilgileri](microk8s-polling-1000-20261023-d/diagnostics.json)
- [C pahalı/normal SQL interval sayaçları](microk8s-polling-1000-20261023-c/diagnostics.json)
- [C canlı custom/generic planlar](microk8s-polling-1000-20261023-c/diagnostics.json)
- [C benchmark ve demo gerçek ayarları](microk8s-polling-1000-20261023-c/diagnostics.json)
- [Race ve vet sonucu](microk8s-polling-1000-20261023-validation.json)

Tekrar: önce `python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8`; bittikten sonra `python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8 --postgres-tuned --postgres-explain`. 10 hedefli integration testi C/D/E koşularının öncesinde geçti. Rapor/profile Python testleri 6/6 geçti. Ek 14 hedefli test race detector ile geçti; go vet ve namespace guard geçti. Ayrıntılar ayrı validation artifact’indedir. Test namespace’leri ve ayrı regression PostgreSQL konteyneri temizlenir; mevcut demo podları korunur. Commit/push/release yapılmadı.


Ham loglar ve zaman serileri Git dışında, checksum ile doğrulanmış yerel arşivlerde saklanır. Her koşunun `reproduction.json` dosyası arşiv adını ve SHA-256 değerini içerir. [Saklama ve geri açma politikası](README.md#raw-artifacts).
