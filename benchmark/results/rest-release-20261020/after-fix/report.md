# Ölçek ve dayanıklılık kabul testi

Başlangıç: 2026-10-20T12:19:51Z. Sonuç: **GEÇTİ**.

Profil: **REST upstream**. Kaydedilmiş JSON operasyonları, request/response şema doğrulaması ve mevcut MCP istemci yolu kullanıldı.
Trafik altında iki node'a snapshot yayılımı: **2346 ms**.

Envanter: 300 upstream, 300 virtual server, 60,000 tool tanımı/seçimi, 0 resource tanımı/seçimi; 600 kalıcı session, 64 yapılandırılmış agent key.

İki ayrı gerçek data-plane süreci; her birinde GOMAXPROCS=2, 768 MiB Go yumuşak bellek limiti ve 64 bağlantılık DB pool. Ayrı PostgreSQL 17 konteyneri 2 CPU / 2 GiB; ayrı Redis konteyneri 0,5 CPU / 256 MiB. Ana uygulama ortamına dokunulmadı.

300 mantıksal upstream adresi 16 sentetik HTTP fixture dinleyicisine dağıtılır. Tool yanıtı 5 ms geciktirilir. Her yanıtta doğru upstream rotası, tool adı, çağrı nonce’u ve PII maskesi doğrulanır. Bu gerçek upstream servislerinin kapasite veya ağ gecikmesi testi değildir.

Test proxy’si round-robin dağıtır, arızalı node için 10 saniyelik pasif bekleme uygular ve POST isteğini başka node’a tekrar göndermez. Üretim ingress’i/nginx bu koşunun parçası değildir. Node kaybı SIGKILL ile yapılır; PostgreSQL HA/failover kapsam dışıdır.

## Aşamalar

| Aşama | Hedef çağrı/s | Başarılı çağrı/s | Başarı % | p95 ms | p99 ms | Hata | Gönderilemeyen | Eşik |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| Isınma | 50 | 49.9 | 100.000 | 52.1 | 63.3 | 0 | 0 | Geçti |
| Sabit yük | 200 | 199.9 | 100.000 | 45.6 | 51.3 | 0 | 0 | Geçti |
| Ani yük artışı | 600 | 584.7 | 100.000 | 193.1 | 356.8 | 0 | 285 | Geçti |
| Yük sonrası toparlanma | 200 | 199.8 | 100.000 | 45.8 | 51.0 | 0 | 0 | Geçti |
| config-refresh | 200 | 199.6 | 99.950 | 78.2 | 211.1 | 3 | 0 | Geçti |
| Node kaybı | 200 | 199.8 | 99.944 | 47.9 | 53.6 | 10 | 0 | Geçti |
| Node geri dönüşü | 200 | 200.0 | 100.000 | 46.8 | 52.4 | 0 | 0 | Geçti |
| Süreklilik | 100 | 100.0 | 100.000 | 48.4 | 55.6 | 0 | 0 | Geçti |

Yüzdelikler yalnızca başarılı çağrıların servis gecikmesidir; hatalar ayrıca sayılır. Başarı oranı gönderilen isteklere göre, çağrı/s ise kuyruk boşaltma dahil gerçek aşama süresine göre hesaplanır. Gönderilemeyen istekler de hedefin karşılanması değerlendirmesine dahildir. İstemci retry yapmaz.

![Çağrı hızı](throughput.svg)

## Tutarlılık ve güvenlik

- Başarılı istemci çağrısı: **105702**.
- Eşleşen kalıcı audit admission: **105702**.
- Eşleşen başarılı audit outcome: **105702**.
- Çift upstream çalıştırma: **0**.
- Outcome’u olmayan admission: **1**.
- Hatalı istemci çağrısıyla açıklanamayan eksik outcome: **0**.
- Çağrı alan farklı upstream: **300**.
- Çağrı alan farklı virtual server: **300**.
- Çağrı alan farklı virtual tool: **57424**.
- Çağrı yapan farklı agent key: **64**.

Node kapanışında kalıcı admission’dan sonra outcome yazılamamış olabilir. Bunlar yalnızca hatalı istemci isteğiyle eşleşiyorsa açıklanmış belirsiz işlemler olarak tutulur; başarılı sayılmaz veya yeniden çalıştırılmaz. Başarılı her istemci yanıtı için admission ve başarılı outcome ayrı ayrı doğrulanır.

- `configuration_refresh_under_rest_load`: geçti.
- `cross_node_session_and_paginated_masked_metadata`: geçti.
- `invalid_keys_rejected_on_both_nodes`: geçti.
- `key_revocation_propagated_and_rejected_on_both_nodes`: geçti.
- `session_termination_authoritative_on_both_nodes`: geçti.

## Kaynak kullanımı

| Node | En yüksek RSS MiB | Medyan OS CPU % | Örneklenen DB pool tepe kullanımı | Pool edinim iptali (gözlenen en yüksek sayaç) | Soak RSS değişimi % |
|---|---:|---:|---:|---:|---:|
| acceptance-node-1 | 1205.4 | 168.4 | 13/64 | 0 | 0.0 |
| acceptance-node-2 | 1214.7 | 168.2 | 26/64 | 0 | 0.0 |

![Bellek](memory.svg)

PostgreSQL: örneklenen bağlantı tepe sayısı **138 / 200**, deadlock **0**, lock bekleyen oturum tepe sayısı **1**. Test veritabanı boyutu ilk/son örnekte **157.8 / 400.9 MiB**. Audit ve dayanıklı delivery kayıtları bu büyümeye dahildir.


RSS yaklaşık beş saniyede bir örneklenir; Go heap ile aynı değildir. CPU yüzdesi işletim sisteminin süreç için raporladığı değerdir; birden çok çekirdekte %100’ü aşabilir. Node yeniden başladığında sayaçlar sıfırlanır. Bu profilde rate-limit değerleri sınırsızdır; Redis limiter maliyeti ölçülmemiştir. Kısa soak süresi uzun dönemli bellek sızıntısının olmadığını kanıtlamaz.

Kaynak örneği: 132. Konteyner kaynağı örneği: 264. PostgreSQL connection, lock wait, deadlock, transaction ve disk boyutu örnekleri ham raporda bulunur. Metrik etiket taşmaları beklenen cardinality sınırıdır; tüm tool’lara ayrı seri ayrıldığı anlamına gelmez.

## Eşikler ve sınırlamalar

- Sabit yük: hedefin en az %95’i, gönderilenlerde en az %99,9 başarı; p95 ≤250 ms, p99 ≤1000 ms.
- Yük artışı: hedefin en az %80’i, en az %80 başarı; p99 <10 saniye ve bozuk sonuç olmaması.
- Node geçişi: aşamada en az %99 başarı; 10 saniyelik geçişten sonra hata ≤%0,1, p99 ≤1000 ms. Pasif sağlık kontrolü isteğinin hatası da sayılır.
- Node RSS ≤1,5 GiB; soak ilk/son üçte birlik medyanları arasında artış ≤%20.
- Yerel, tek fiziksel makine ve tek PostgreSQL örneği. Üretim ağları, JWT/IdP yükü, kontrol düzlemi discovery/config churn, uzun çalışan gerçek araçlar ve veritabanı HA dahil değildir.


## Değerlendirme

Bu yerel profil, önceden belirlenen kabul eşiklerini karşıladı. Toplam 106,000 planlanan isteğin 105,702 adedi doğru yanıt ve kalıcı audit eşleşmesiyle tamamlandı. Tüm 300 virtual server ve 300 upstream gerçek çağrı aldı; 57,424 farklı virtual tool çalıştırıldı.

Mevcut yapı bu profil için yeterli kapasite payı gösteriyor; ikinci optimizasyon paketini zorunlu kılan bir darboğaz gözlenmedi. Bu sonuç mikro tuning yerine kod basitliğini korumayı destekliyor. İlk mikrobenchmarkdan farklı olarak bu koşu tam üretim data-plane’inin dayanıklı MCP delivery/SSE yolunu da çalıştırır; bu iki koşunun gecikmeleri doğrudan karşılaştırılmamalı. Üretim boyutlandırması için farklı gerçek upstream gecikmeleri, JWT/rate-limit politikaları ve ingress ayrıca doğrulanmalı.

Node kapanışından sonra 1 çağrının audit outcome’u yoktur. Bu kayıt hatalı istemci isteğiyle eşleşir; yürütme sonucu belirsizdir. Başarılı sayılmadı ve otomatik tekrar yürütülmedi. İş açısından yan etkili gerçek tool kullanımlarında bu belirsizlik ayrıca uzlaştırılmalıdır.

## Tekrarlanabilirlik

Kaynak HEAD: `1df9dab41964cb69354150b0a860dc1b6d998bb7`; çalışma ağacındaki değişiklikleri içeren gerçek binary SHA-256: `ef2aa67f864cdb3c87594963a500281456092d786d5475109e512b61520d4d3e`.
go1.27.2, darwin/arm64. Ham örnekler ve sayaçlar: [report.json](report.json).

```sh
VMCP_ACCEPTANCE_REST=1 ./scripts/acceptance-core.sh
```
