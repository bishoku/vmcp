# Conversation correlation — 500 TPS öncesi/sonrası (30 Ekim 2026)

Her iki koşu kabul kontrollerini geçti. 500 TPS seviyesinde kapasite kaybı veya yeni hata görülmedi; p95/p99 gecikmesi, bellek ve özellikle audit depolama/WAL maliyeti arttı. Tek öncesi/sonrası koşu istatistiksel anlamlılık veya maksimum kapasite iddiası değildir.

## Yöntem

- Doğrudan virtual MCP; Omni üzerinden yük yok. Aynı MicroK8s hostunda ardışık, bağımsız test namespace/veritabanları. Mevcut vmcp-demo kurulumu değiştirilmedi.
- Her koşu 15 saniye/50 TPS ısınma + **300 saniye/500 TPS**; sentetik REST upstream, 300 upstream, 300 virtual server, 6.000 tool, 600 kalıcı MCP session, 64 agent key ve ekipler arası approved grant.
- Tek data-plane: GOMAXPROCS=8, CPU request/limit=8, GOMEMLIMIT=4GiB, pod limit=6Gi, DB pool=64. PostgreSQL tuned; fsync/synchronous commit açık, CPU/bellek hard limiti yok.
- Strict audit açık, payload capture kapalı. İki koşuda da aynı conversation/root metadata şekli gönderildi. Önceki sürüm bu alanları korelasyona çıkarmıyordu; sonraki koşuda bütün agent key mappingleri açık ve yeni audit indeksleri etkin. Turn gönderilmedi.
- Host test proxy → PodIP/CNI. Production ingress, Omni veya yerel GPU modellerinin performansı ölçülmedi. Başka uygulamalar aynı hostta çalışmaya devam etti. Ölçüm sırasında ek derleme/test çalıştırılmadı.
- İlk koşuda yalnız trafik sürücüsüne metadata eklendi; uygulama kodu değiştirilmeden baz alındı. İkinci koşuda mapping seed ve capture doğrulaması eklendi. İmajlar/kaynak ağaç hashleri ayrı evidence dosyalarında mevcut.

## Sonuçlar

| Metrik | Önce | Sonra | Fark |
|---|---:|---:|---:|
| Başarılı TPS | 499.939 | 499.938 | -0.00% |
| p50 (ms) | 36.724 | 40.036 | +9.02% |
| p95 (ms) | 52.926 | 58.736 | +10.98% |
| p99 (ms) | 69.593 | 79.158 | +13.74% |
| Medyan CPU (çekirdek) | 0.912 | 0.924 | +1.40% |
| Medyan RSS (MiB) | 215.621 | 224.996 | +4.35% |
| Maksimum RSS (MiB) | 225.090 | 235.207 | +4.49% |
| WAL üretimi (MiB/s) | 2.441 | 3.189 | +30.67% |
| Son DB boyutu (MiB) | 414.615 | 599.107 | +44.50% |
| Audit INSERT ortalama SQL süresi (ms) | 0.227 | 0.268 | +18.00% |
| Audit INSERT başına WAL (byte) | 1229.297 | 2006.545 | +63.23% |

Başarılı çağrı: **150,000 → 149,998**. HTTP/tool hatası: **0 → 0**. Gönderilemeyen: **0 → 2 / 150.000** (%0,0013); bunlar gateway hatası değil, yük sürücüsünün bounded gönderim/scheduling drop sayısıdır. Gelen isteklerde başarı %100. Retry yapılmadı.

SLO: p95 ≤250 ms, p99 ≤1.000 ms, başarı ≥%99,9 ve başarılı TPS hedefin ≥%95’i. Her iki koşu geçti. Sonraki koşuda `conversation_metadata_captured=true`; başarılı virtual tools/call outcome kayıtlarında conversation/root/group alanlarının dolduğu doğrulandı. Key iptali, session sonlandırma, geçersiz key reddi ve audit tutarlılığı kontrolleri de geçti.

## Etkinin yorumu

- Gecikme artışı: p95 +5,81 ms (%10,98), p99 +9,57 ms (%13,74). Bu yükte kabul bütçesi içinde; “sıfır maliyet” sonucu değildir. Daha küçük farkların anlamlılığını değerlendirmek için tekrarlı A/B koşuları gerekir.
- Medyan CPU yaklaşık 0,9115 → 0,9243 çekirdek (+%1,40), RSS +9,375 MiB (+%4,35). Go GC döngüsü 219 → 221; toplam STW 28.327 → 28.067 ms. Belirgin bir GC baskısı artışı görülmedi. GC sayıları ~332 saniyelik pod yaşamı boyunca başlangıç/son kontrolleri de içerir; yalnız steady fazı sayacı değildir.
- Ana ek maliyet audit yazımı/depolamasında: yeni ID kolonları ve üç partial B-tree indeksi audit INSERT başına WAL’i yaklaşık %63 artırıyor. Tüm DB WAL hızı yaklaşık %31, son DB boyutu yaklaşık %44,5 arttı. Bunlar audit/session/delivery dahil sentetik DB ölçümleridir; salt audit tablo boyutu veya evrensel retention katsayısı değildir.
- Audit INSERT SQL ortalaması ~0,2275 → ~0,2684 ms (+%18). Bu SQL ölçümü commit/durability beklemelerinin tamamını temsil etmez. Daha yüksek WAL ve indeks yazımı, gözlenen maliyetle uyumlu; tek çift koşuyla gecikme farkının tamamını tek bir nedene bağlamıyoruz.
- PostgreSQL deadlock ve gözlenen lock waiter sayısı 0; pool acquire iptali 0. Maksimum acquired bağlantı 21 → 32 (limit 64). Bu seviyede yeni bir kapasite darboğazı saptanmadı; daha yüksek TPS için sonuç çıkarılmadı.
- Mevcut telemetry cardinality bütçesi iki koşuda da doldu (label overflow ~255 bin). Bu nedenle bu koşu per-tool metric ayrıntısı için tam kapsam sağlamaz; client sonuçları ve PostgreSQL/audit kontrolleri ayrıca doğrulandı.

## Kapsam ve release değerlendirmesi

Bu veri, varsayılan kapalı mappingin değil, mapping **açık** durumunun depolama ve hot-path etkisini kapsar. Eksik/bozuk metadata, farklı key izolasyonu, root/child gruplaması, optional turn, Omni aktarımı ve yetki sınırlaması fonksiyonel testlerle doğrulandı. Conversation listeleme sorgularının eşzamanlı yüksek okuma yükü, payload capture maliyeti ve uzun süreli retention bu yük testinin kapsamında değildir.

Geriye uyumlu yeni config/API/UI ve migration için önerilen sürüm **v1.3.0 (minor)**. Migration 35 önce uygulanmalı; mevcut audit tablolarında indeks inşası yazımları bekletebileceğinden kontrollü upgrade penceresi planlanmalı. Release akışı/yayın bu çalışma kapsamında başlatılmadı.

## Kanıtlar

- [Önceki koşu](microk8s-20261030-111358/report.md)
- [Sonraki koşu](microk8s-20261030-112917/report.md)
- [Sayısal karşılaştırma](conversation-correlation-500tps-20261030.json)
- Ham evidence: `artifacts/benchmark-runs/conversation-before-20261030/` ve `conversation-after-20261030/`; doğrulanmış sıkıştırılmış arşivler `artifacts/benchmark-archives/` altında.
