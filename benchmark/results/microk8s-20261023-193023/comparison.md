# MicroK8s yük testi — Compose karşılaştırması

Sonuç: **Kabul bulguları var**. 41.9 dakika aşama süresinde 1,174,500 planlanan çağrı; 453,932 başarılı ve audit ile eşleşen çağrı.

Önceki REST koşusu macOS/arm64 ve native data-plane + Compose PostgreSQL/Redis kullanıyordu. Bu koşu Linux/amd64, MicroK8s podları ve Kubernetes Service/CNI yolunu kullanıyor. Yazılım ağacı, Go sürümü ve kaynak limitleri de farklıdır; gecikme farkları tek başına Kubernetes veya kod optimizasyonu kazancı sayılamaz.

Envanter her iki koşuda 300 REST upstream, 300 virtual server, 60.000 tool, 600 oturum ve 64 agent key. Strict metadata audit açık; payload capture, aktif rate limit ve JWT/IdP yükü kapsam dışı.

| Aşama | Önceki hedef/sn · süre | Önceki başarı/sn · p95 / p99 ms | MicroK8s hedef/sn · süre | MicroK8s başarı/sn · p95 / p99 ms | Yeni hata / gönderilemeyen |
|---|---:|---:|---:|---:|---:|
| baseline | 200 · 90s | 199.9 · 45.6 / 51.3 | 200 · 90s | 199.1 · 143.7 / 360.8 | 0 / 0 |
| steady | 200 · 90s | 199.9 · 45.6 / 51.3 | 600 · 300s | 191.2 · 3280.5 / 3629.1 | 2008 / 120232 |
| spike | 600 · 20s | 584.7 · 193.1 / 356.8 | 2000 · 60s | 184.0 · 5091.7 / 5521.0 | 649 / 107964 |
| recovery | 200 · 45s | 199.8 · 45.8 / 51.0 | 600 · 45s | 191.4 · 3312.9 / 3675.6 | 278 / 17633 |
| config-refresh | 200 · 30s | 199.6 · 78.2 / 211.1 | 600 · 30s | 190.1 · 4542.4 / 4891.3 | 351 / 11562 |
| node-loss | 200 · 90s | 199.8 · 47.9 / 53.6 | 600 · 90s | 44.9 · 431.1 / 682.7 | 49955 / 3 |
| node-rejoin | 200 · 60s | 200.0 · 46.8 / 52.4 | 600 · 60s | 597.1 · 631.4 / 842.1 | 1 / 59 |
| soak | 100 · 300s | 100.0 · 48.4 / 55.6 | 400 · 1800s | 172.1 · 5583.8 / 5981.1 | 20197 / 389676 |

## Sonuç ve öncelikler

200 çağrı/sn referansı geçti. 600 çağrı/sn sabit yük ve 400 çağrı/sn uzun süreklilik bu kaynak profilinde kabul edilmedi. Hedef hızın artırılması, bu hızın gerçekten üretildiği/karşılandığı anlamına gelmez: gönderilemeyen talep tabloya ayrı yazıldı.
Tüm başarılı yanıtlar kalıcı audit admission/outcome kayıtlarıyla eşleşti; bozuk sonuç ve çift yürütme yok. Sonucu belirsiz işlemler başarısız istemci çağrılarıyla ilişkili; başarılı kabul edilmedi ve yeniden yürütülmedi.
Konfigürasyon yayılımı ve key iptalinin iki node’da doğrulanması geçmedi. Key iptali kontrolünde snapshot revizyonu 30 saniyelik pencerede iki node’da doğrulanamadı; rapordaki genel “accepted or timed out” mesajı tek başına yetkisiz çağrının kabul edildiğinin kanıtı değildir.
Öncelikli inceleme: delivery cascade/temizlik indeksleri ve bakım bütçesi; DB pool/lock beklemeleri; büyük katalog altında snapshot tazeliği. Düzeltme sonrasında aynı Kubernetes profili yeniden çalıştırılmalı.


## Kaynak ve tutarlılık

- Data-plane başına Go limiti 2 CPU / 768MiB olarak korundu. Kubernetes hard limit 4CPU/3GiB; PostgreSQL hard limit 4CPU/4GiB, Redis 1CPU/512MiB. Önceki PostgreSQL 2CPU/2GiB ve Redis 0,5CPU/256MiB idi. Bu kapasite farkı karşılaştırmanın parçasıdır.
- Yük sürücüsü host üzerinde 8 Go scheduler CPU’su ve 512 bounded worker kullanır; önceki worker sayısı 128 idi. Talep üretilemediğinde dropped demand olarak raporlanır.
- Pod arızası bir data-plane Deployment’ının sıfıra indirilip geri açılmasıdır. Önceki koşunun native SIGKILL arızasıyla aynı değildir; fiziksel Kubernetes node kaybı ölçülmedi.
- Audit eşleşmesi: 453,932 admission / 453,932 başarılı outcome. Çift yürütme: 0. Sonucu belirsiz admission: 571; hatalı istemci çağrısıyla açıklanamayan: 0.
- Snapshot yayılımı 25560 ms gözlem penceresinde doğrulanamadı; bu değer tamamlanmış yayılım süresi değildir.
- acceptance-node-1: tepe RSS 1095.9 MiB; medyan CPU %199.9; soak RSS artışı %0.00; DB pool tepe 64/64; pool iptali 12633.
- acceptance-node-2: tepe RSS 1199.0 MiB; medyan CPU %199.8; soak RSS artışı %-0.06; DB pool tepe 64/64; pool iptali 83368.
- Örneklenen en yüksek snapshot yaşı 119.54s; refresh süresi 5.65s. Yetkilendirme tazeliği sınırı 30s olarak korundu.
- Baseline→spike sonrası gözlem penceresinde PostgreSQL CFS throttling periyot oranı %37.7. [Ham sayaç farkları](diagnostics.json). Bu oran tek başına tüm gecikmenin nedenini kanıtlamaz.
- redis: örneklenen tepe RSS 4.0 MiB, tepe CPU %1.6.
- control-plane: örneklenen tepe RSS 6.5 MiB, tepe CPU %0.2.
- postgres: örneklenen tepe RSS 395.2 MiB, tepe CPU %396.2.
- PostgreSQL: tepe bağlantı 143, lock waiter 127, deadlock 0; son örnekte DB boyutu 1.33 GiB.

## Ek veritabanı bulgusu

Delivery temizliği için süresi geçmiş tamamlanmış operation kuyruğu iki gözlem arasında 193,574 → 299,691 oldu. Audit saklama süresinden ayrı olan 15 dakikalık delivery TTL temizliği yetişemiyor. [Kuyruk ölçümleri — yerel arşiv](../README.md#raw-artifacts).

`mcp_streams.operation_id` için indeks yok. ON DELETE CASCADE yolundaki lookup ile eşdeğer SELECT, yaklaşık 350 bin satırda parallel sequential scan ve yaklaşık 92 ms olarak ölçüldü. [Plan ve mevcut indeksler](diagnostics.json). `delivery.Store.Run` bakım turu bir saniyelik bağlam bütçesini paylaşır; çoklu cascade/batch temizliği için bu yolun optimize edilip yeniden ölçülmesi gerekir. Bu tek SELECT ölçümü, tüm gecikme nedenlerini tek başına açıklamaz. Koşu sırasında indeks veya uygulama ayarı değiştirilmedi.
- Mevcut vmcp-demo içindeki 6 aktif podun UID/restart sayıları: korundu.
- Önceki envanterde tamamlanmış durumdaki pod son envanterde yok: vmcp-demo-migration-1-rszkj. Aktif workload değişikliği olarak sayılmadı.

RSS kubelet container RSS değeridir; CPU yakın dönem kullanımından gelir. Önceki ps CPU ortalamasıyla aynı ölçüm değildir. Ölçümler yaklaşık beş saniyelik örneklerdir; kısa tepe değerlerini kaçırabilir ve 30 dakikalık soak, uzun dönem bellek sızıntısı olmadığını kanıtlamaz.

## Bulgular

- steady capacity/latency SLO failed
- spike capacity/latency SLO failed
- recovery capacity/latency SLO failed
- REST configuration refresh did not reach both nodes
- config-refresh capacity/latency SLO failed
- node-loss capacity/latency SLO failed
- soak capacity/latency SLO failed
- revoked key accepted or snapshot propagation timed out on acceptance-node-1
- revoked key accepted or snapshot propagation timed out on acceptance-node-2

## Kanıtlar

- [Ayrıntılı rapor ve grafikler](report.md)
- [Özet sonuçlar](summary.json); ham örnekler [yerel arşivde](../README.md#raw-artifacts)
- [Ortam, kaynak HEAD ve çalışma ağacı — yerel arşiv](../README.md#raw-artifacts)
- [Önceden belirlenen profil — yerel arşiv](../README.md#raw-artifacts)
- [Uygulanan kaynak manifesti — yerel arşiv](../README.md#raw-artifacts)
- [Pod durumları — yerel arşiv](../README.md#raw-artifacts)
- [Kubernetes olayları — yerel arşiv](../README.md#raw-artifacts)

Tekrar çalıştırma: `python3 benchmark/kubernetes/run.py`. Sonuçlar çalışma ağacından derlenen binary’ye aittir; kaynak/binary/imaj kimlikleri rapor ve environment/images dosyalarındadır.


Ham örnekler ve loglar Git dışında tutulur. Arşiv kimliği ve SHA-256: [reproduction.json](reproduction.json). Bkz. [arşiv politikası](../README.md#raw-artifacts).
