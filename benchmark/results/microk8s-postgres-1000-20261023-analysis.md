# PostgreSQL odaklı 1.000 TPS değerlendirmesi

Sonuç: **1.000 TPS kabulü karşılanmadı.** CPU kotası sorunu giderildi; yüksek yükte NOTIFY/COMMIT kilit zinciri doğrulandı. Uygulama CPU artışının etkisi aşağıda ayrı gösterilir.

23 Ekim 2026. İki koşu aynı MicroK8s hostunda **sırayla** çalıştırıldı. PostgreSQL CPU/bellek limitleri kaldırıldı; requests 2CPU/2GiB kaldı. Üç yeni delivery indeksi iki koşuda da uygulandı. İlk koşuda data-plane başına 2, ikinci koşuda 8 scheduler CPU’su ve buna uygun Kubernetes kaynakları kullanıldı. GOMEMLIMIT=768MiB, pod bellek limiti 3GiB, pool=64/pod değişmedi.

Her koşu 15 saniye 50 TPS ısınma ve **300 saniye 1.000 TPS** hedefi içerir. 300 REST upstream, 300 virtual server, 60.000 tool, 600 session, 64 key ve strict metadata audit korundu. Upstream yanıtı sentetik 5 ms’dir. 100.000 süresi dolmuş operation/stream temizlik fixture’ı eklendi. Bunlar tool/audit başarısı olarak sayılmaz. Payload capture, JWT/IdP, aktif limiter, üretim ingress’i, pod arızası ve trafik altında config mutasyonu bu profilin kapsamında değildir.

| Ölçüm | A: 2 CPU/pod | B: 8 CPU/pod |
|---|---:|---:|
| Başarılı TPS | 312.1 | 306.6 |
| p95 ms | 4,633.3 | 4,394.1 |
| p99 ms | 5,060.7 | 4,716.6 |
| Başarılı çağrı | 94,220 | 92,509 |
| Hata | 24,229 | 19,938 |
| Gönderilemeyen talep | 181,551 | 187,553 |
| Gönderilenlerde başarı % | 79.545 | 82.269 |
| PG tepe CPU (çekirdek) | 6.3 | 20.3 |
| PG medyan CPU (çekirdek) | 2.4 | 2.4 |
| PG tepe bağlantı | 142.0 | 143.0 |
| PG tepe lock waiter | 126.0 | 125.0 |

Başarılı TPS, kuyruk boşaltma süresi dahil gerçek geçen süreye göre hesaplanır. Gecikmeler yalnızca başarılı çağrılardır. Gönderilemeyen talep bounded open-loop sürücünün karşılayamadığı taleptir; hedef TPS gerçekleşmiş gibi sayılmaz. Kabul: hedefin ≥%95’i, gönderilenlerde ≥%99,9 başarı, p95 ≤250ms ve p99 ≤1000ms. Testlerin nonzero çıkışı bu eşiklerin başarısızlığını korur.

## Kesinleşen bulgular

1. **CPU throttling kaldırıldı, tek başına yeterli olmadı.** `postgres-cgroup-*.json` dosyalarında `cpu.max=max 100000`, `memory.max=max`, `nr_throttled=0` var. Limitin kalkması PostgreSQL’in dört çekirdeğin üzerine çıkmasına izin verdi. İkinci koşudaki uygulama CPU artışı ayrı karşılaştırmadır; iki değişkeni eski Compose koşusuyla karıştırarak kesin hızlanma oranı çıkarmıyoruz.

2. **NOTIFY transaction commit’lerini seri hale getiriyor.** Yük altındaki kilit örneği `object / AccessExclusiveLock / classid=1262 / objid=0 / database=0` ve bekleyen sorgu `commit`. Bu kimlik PostgreSQL 17’nin `PreCommit_Notify` içindeki global bildirim kilidiyle eşleşiyor. Bizim `delivery.Store.Append`, event INSERT ve operation UPDATE ile aynı transaction içinde `pg_notify` çağırıyor. Böylece WAL/COMMIT beklemesi sırasında operation satır kilitleri ve pool bağlantıları da tutuluyor. Bu, yalnızca eksik indeks veya CPU kotası açıklaması değildir. [PostgreSQL 17 kaynak kodu](https://github.com/postgres/postgres/blob/REL_17_STABLE/src/backend/commands/async.c#L851-L868).

   A: 53 yük örneğinde 5,402 aktif oturum gözleminin 4,854 adedi (%89.9) `Lock/object` bekliyor. Bu örnekleme dağılımıdır; CPU zamanı veya bütün isteklerin gecikme yüzdesi değildir.
   B: 53 yük örneğinde 5,287 aktif oturum gözleminin 4,762 adedi (%90.1) `Lock/object` bekliyor. Bu örnekleme dağılımıdır; CPU zamanı veya bütün isteklerin gecikme yüzdesi değildir.

3. **İndeks eksikliği giderildi; cascade lookup tam tarama yapmıyor.** Yeni migration `mcp_streams(operation_id)`, `mcp_pending_requests(operation_id)` ve tamamlanmış operation’lar için kısmi `created_at` indeksini ekliyor. A koşusunda gerçek veri üzerinde stream ilişki lookup’u Index Only Scan ile yaklaşık 0,10ms ve üç shared-buffer hit olarak ölçüldü. Önceki yaklaşık 350 bin satırlı tam tarama 92ms idi; tablo boyutları farklı olduğundan bu doğrudan aynı veriyle hızlanma testi değildir.

4. **Retention kuyruğu temizlendi, fakat seçici plan ve temizlik kapasitesi hâlâ iyileştirilebilir.** 100.000 eski operation iki koşuda da izlenir; sıfıra erişim UTC zamanları aşağıdadır. A koşusunda kuyruk boşaldıktan sonra mevcut LIMIT 200 sorgusu, yeni indeks mevcutken bile 85.422 satır taradı (~23,9ms). B koşusunda aynı veri üzerinde mevcut operation seçimi 22,07ms / 2.493 buffer hit iken `ORDER BY created_at` ile 0,067ms / 8 hit; stream seçimi 13,89ms iken `ORDER BY expires_at` ile 0,035ms oldu. Bu read-only SELECT plan karşılaştırmasıdır; DELETE veya uygulama kodu değiştirilmedi. İndeks sırasını kullanan, `FOR UPDATE SKIP LOCKED` ile replica’ların farklı batch seçtiği plan değerlendirilmelidir. Mevcut iki worker × 200/tur × yaklaşık 1tur/sn tasarımının ideal tavanı yaklaşık 400 operation/sn’dir; gerçek 1.000 TPS için batch/bakım bütçesi de artırılmalıdır. Beş dakikalık test yeni kayıtların 15 dakikalık TTL’sini aşmaz; fixture temizliği uzun vadeli 1.000 TPS retention kabulü değildir.

   A: 2026-10-23T17:35:00.776321+00:00; B: 2026-10-23T17:43:29.985312+00:00.

5. **Beklemeler ek SQL yükü oluşturuyor.** A’nın kümülatif sayaçlarında 94.970 başarılı cevaba karşı 440.622 event okuması, 190.532 session okuması ve 55.690 lease yenileme denemesi var. 250ms polling ve uzun süren completion, boş event okumalarını çoğaltıyor. Lease yenilemenin ortalama 24ms görünmesinde completion’ın tuttuğu operation kilidini bekleme de var; bu SELECT/UPDATE’lere indeks eklemek tek başına ana kuyruğu çözmez. `Append` içindeki event count ve toplam boyut iki alt sorguda aynı stream’i okuyor; bir agregasyonda toplanabilir. 24.234 boş subscription-claim sorgusu toplam ~0,56s olduğundan ana optimizasyon önceliği değildir. Bu sayaçlar kurulum/yük/doğrulama kümülatifidir ve doğrudan yalnızca 300 saniyeye bölünmemelidir.

6. **Büyük katalog yenilemesi ayrıca izlenmeli.** Post-load key revocation kontrolü sonuçları aşağıdadır. Başlangıçta upstream SELECT sürelerinin uzun olması tek başına SQL CPU problemi değildir: activity örneklerinde ClientWrite ve client tarafındaki decrypt/compile/backpressure da vardır. Snapshot refresh’in beş saniyelik bütçesi ile büyük katalog yeniden yüklemesi birlikte profillenmelidir. Başarısız revizyon yayılımı tek başına iptal edilmiş anahtarın yetkilendirildiğini kanıtlamaz.

## Düzeltme öncelikleri

1. Event’i ve outcome’u kalıcı yazdıktan sonra local okuyucuyu hemen uyandır; uzak node bildirimlerini bounded bir kuyruğa alıp küçük zaman penceresinde session bazında birleştir. Bildirim transaction’ını event/operation transaction’ından ayır ve tek commit içinde batch bildir. Polling/replay güvenlik ağı korunmalı; NOTIFY veri kaynağı veya exactly-once garantisi olarak kullanılmamalı. Kaybolan bildirim, yeniden başlama, reconnect, cross-node resume ve iptal akışları test edilmeli.
2. Temizlik seçimlerini indeks sırasına oturt; worker’ların aynı kayıtları seçmesini SKIP LOCKED ile engelle; 1.000 TPS’ye yetecek batch/döngü bütçesi ve backlog/error metrikleri ekle. PublishListChanged ile Purge’un aynı bir saniyelik context bütçesini paylaşması ve hataların sessizce yutulması giderilmeli.
3. Sonra delivery Begin/Append içindeki bağımsız sorguları batch et ve aynı stream’in count/size okumalarını birleştir; güvenlik/session doğrulamalarını kaldırarak veya WAL durability’yi gevşeterek hız kazanma.
4. NOTIFY kuyruğu giderildikten sonra Go CPU/heap/GC profili al. GOMEMLIMIT 768MiB sabit kaldı; RSS yaklaşık 1GiB olması tek başına GC darboğazını kanıtlamaz. Scheduler/heap sınırları, büyük katalog derleme maliyeti, WAL sync/storage ve pool boyutu yeniden ölçülmeli. Pool’u körlemesine büyütmek mevcut lock convoy’u büyütebilir.

Bu çalışma production runtime’da indeks migration’ını ekler; NOTIFY, temizlik worker’ı veya yetkilendirme semantiği henüz değiştirilmedi. 1.000 TPS’nin güvenilir kabulü, düzeltmelerden sonra aynı beş dakikalık profilin geçmesi ve ayrıca 15 dakikalık retention eşiğini geçen uzun koşuyla doğrulanması gerekir.

## Tutarlılık ve doğrulama

- A: toplam 94,970 başarılı çağrı (ısınma dahil), 94,970 admission ve 94,970 başarılı outcome; duplicate=0, sonucu belirsiz=0. Key revocation yayılımı: False. [Rapor](microk8s-postgres-1000-20261023-a/report.md), [SQL örnekleri — yerel arşiv](README.md#raw-artifacts), [SQL sayaçları — yerel arşiv](README.md#raw-artifacts), [migration/delivery testleri — yerel arşiv](README.md#raw-artifacts).
- B: toplam 93,259 başarılı çağrı (ısınma dahil), 93,259 admission ve 93,259 başarılı outcome; duplicate=0, sonucu belirsiz=0. Key revocation yayılımı: False. [Rapor](microk8s-postgres-1000-20261023-b/report.md), [SQL örnekleri — yerel arşiv](README.md#raw-artifacts), [SQL sayaçları — yerel arşiv](README.md#raw-artifacts), [migration/delivery testleri — yerel arşiv](README.md#raw-artifacts).

Üretim data-plane binary SHA-256 iki koşuda aynı: `ff16642693740606e9d0f42bc1fd72bdc3c50b8a6f1d2338303cbed18889516e`. Manifest/profile/environment dosyaları kaynakları ve çalışma ağacını saklar. PostgreSQL sürümü, shared_buffers=128MiB, work_mem=4MiB ve kalıcı WAL ayarları değişmedi; pg_stat_statements.track=all ve I/O timing gözlem maliyeti iki koşuda da var. Tek host, ephemeral test veritabanı, sabit 5ms upstream ve sentetik workload sonuçları gerçek bankacılık servislerinin uçtan uca SLA’sı değildir.

Tekrar: `python3 benchmark/kubernetes/run.py --database-focus`; ardından ilk koşu tamamen bitince `python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8`. Mevcut vmcp-demo korunur; test namespace’leri sonuç toplandıktan sonra kaldırılır. Commit/push/release yapılmadı.


Ham loglar ve zaman serileri Git dışında, checksum ile doğrulanmış yerel arşivlerde saklanır. Her koşunun `reproduction.json` dosyası arşiv adını ve SHA-256 değerini içerir. [Saklama ve geri açma politikası](README.md#raw-artifacts).
