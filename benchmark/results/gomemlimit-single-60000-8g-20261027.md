# 60.000 tool / tek data-plane / 8 GiB Go hedefi — 27 Ekim 2026

**Sonuç:** 600 saniye boyunca 1.000 TPS hedefinde **999,90 başarılı TPS**, **sıfır hata**, p95 **81,18 ms**, p99 **106,62 ms** elde edildi. Canlı heap **782 MiB**, GC sıklığı **0,100/s**, GC CPU işi **0,115 çekirdek eşdeğeri**. Önceki 768MiB profilde görülen GC baskısı bu koşuda görülmedi. Key iptali snapshot'a **2,985 saniyede** yayıldı; iptal edilen key 401, readiness 200 döndü.

**Genel acceptance sonucu FAIL:** Tek başarısız kapı, testte sabit kalan **1.536 MiB RSS bütçesi**. Ölçülen tepe RSS **1.745,19 MiB**. Trafik SLO'su ve fonksiyonel kontroller geçti. Eşik sonuçları iyileştirmek için değiştirilmedi; bu profil Kubernetes'in 12Gi hard limitine yaklaşmadı.

## Test düzeni

300 upstream × 200 tool = 60.000 tool; 300 vMCP, 600 kalıcı oturum, 64 key, ekip sahipliği ve approved tool grant'leri. Tek aktif data-plane instance; CPU limiti ve GOMAXPROCS 8, DB pool 64, GOMEMLIMIT **8GiB**, Kubernetes hard memory limiti **12Gi**, request 2Gi. GOGC değiştirilmedi. GiB birimi önceki deneylerle tutarlı seçildi; 8GiB rezervasyon veya fiilen kullanılacak bellek değildir.

İzole MicroK8s namespace'inde tuned PostgreSQL, Redis, strict audit ve kalıcı delivery kullanıldı. 15 saniye / 50 TPS ısınma ardından 600 saniye / 1.000 TPS; 1.024 worker. Sentetik upstream ve test proxy'sinden doğrudan PodIP/CNI erişimi kullanıldı. Önceki NodePort bağlantı reddi sorunu bu ağ yolunda tekrar sınanmış değildir. Bu deney tek data-plane kapasitesini bu hedefte doğrular; maksimum TPS araması ve çok fiziksel node ölçekleme testi değildir.

Ürün binary SHA-256 önceki deneylerle aynı: `821f108e62ab4dd49d0d8f4be299115b742005c45183f0dae5976773ad4d12e1`. Bu koşu için ürün kodu veya GC algoritması değiştirilmedi.

## Ölçümler

| Ölçüm | Sonuç |
|---|---:|
| Planlanan / gönderilen / başarılı çağrı | 600.000 / 599.988 / 599.988 |
| Başarılı TPS | 999,904 |
| Gönderilen çağrılarda başarı | %100 |
| Hata / bozuk sonuç | 0 / 0 |
| Yük üreticisinin gönderemediği çağrı | 12 (%0,002) |
| p50 / p95 / p99 | 54,05 / 81,18 / 106,62 ms |
| En yavaş başarılı çağrı | 181,87 ms |
| Medyan / tepe RSS | 1.743,18 / 1.745,19 MiB (yaklaşık 1,70 GiB) |
| Medyan data-plane CPU | 1,808 çekirdek |
| Canlı heap / GC heap hedefi | 782 / 1.566 MiB |
| GC sıklığı | 0,0998/s — yaklaşık 10 saniyede bir |
| GC CPU işi | 0,1149 çekirdek eşdeğeri |
| GC çevrimi başına toplam STW, medyan / maksimum | 0,288 / 0,968 ms |
| Medyan PostgreSQL CPU | 3,599 çekirdek |
| En fazla kullanılan DB pool bağlantısı | 50 / 64 |
| DB bağlantısı edinme iptali | 0 |
| Toplam bağlantı edinme beklemesi, steady örnek farkı | 19,71 saniye |

Trafik kapıları: başarı ≥%99,9, başarılı TPS hedefin ≥%95'i, p95 ≤250 ms, p99 ≤1.000 ms ve bozuk sonuç yok. Gönderilemeyen istekler hata sayısından ayrı tutuldu.

RSS/CPU değerleri steady fazındaki örneklerden hesaplandı. GC değerleri process zamanı **199,174–599,174 saniye** arasındaki yaklaşık 400 saniyelik, steady yük içindeki pencereden hesaplandı. GC CPU işi logdaki idle worker sürelerini de içerir; uygulama CPU profili veya doğrudan request bekleme süresi değildir. STW değerleri her çevrimin iki duraklamasının toplamıdır; logdaki eşzamanlı mark süresi bunlara dahil değildir. Kaynak medyanları ve GC ortalamaları farklı örnekleme yöntemlerinden geldiği için oranları kesin CPU yüzdesi olarak yorumlanmamalı.

## Önceki koşularla karşılaştırma

| Profil | Toplam hedef TPS / instance | Canlı heap MiB | Heap hedefi MiB | GC/s | GC CPU, çekirdek/instance | Medyan RSS MiB/instance |
|---|---|---:|---:|---:|---:|---:|
| Önceki 60.000 tool, 768MiB | 1.000 / 2 instance | ~775 | ~775 | 5,86–6,02 | ~4,93 | 934–943 |
| Bu koşu: 60.000 tool, 8GiB | 1.000 / 1 instance | 782 | 1.566 | 0,100 | 0,115 | 1.743 |
| Önceki 6.000 tool, 8GiB | 1.200 / 1 instance | 99 | 201 | 0,994 | 0,119 | 241 |

60.000 tool eski profilde canlı heap zaten 768MiB Go hedefi civarındaydı. Geçici allocation için yeterli alan kalmaması, neredeyse canlı veri büyüklüğüne sıkışmış heap hedefi ve sürekli GC ile örtüşüyordu. Yeni profilde Go heap hedefini yaklaşık iki kat canlı heap düzeyine çıkarabiliyor. **Daha yüksek RSS karşılığında çok daha az GC** görülüyor; 8GiB'nin tamamı kullanılmıyor.

Eski 60.000 tool koşusuna göre gözlenen GC sıklığı yaklaşık 59–60 kat, GC CPU işi yaklaşık 43 kat düşük. Ancak eski deney iki instance, instance başına yaklaşık yarı trafik, NodePort yolu ve 3Gi hard limit kullanıyordu. Dolayısıyla bu oranlar eş kontrollü tek değişken deneyi değildir; yalnız GOMEMLIMIT değişikliğinin kesin nedensel hızlanma oranı olarak sunulamaz. Eski iki instance'ın her biri aynı tam kataloğu taşıyordu.

6.000 tool koşusunda canlı heap çok daha küçük olduğu için GC çevrimleri daha sık olmasına rağmen GC CPU maliyeti benzer düzeyde. **GC sayısı tek başına baskı ölçüsü değildir:** taranan canlı veri, çevrim CPU maliyeti, duraklamalar ve uygulama gecikmeleri birlikte değerlendirilmelidir.

## Key iptali ve doğruluk

Key iptali `06:08:51.396993986Z`, revision 80 snapshot yayını `06:08:54.381935385Z`: fark **2,984941399 s**. Sonraki kontrol iptal edilen key için **401**, readiness için **200** verdi. Önceki 60.000 tool / 768MiB koşusundaki refresh timeout ve stale snapshot sorunu bu koşuda tekrarlanmadı. Bu kontrol yük bittikten sonra yapıldı; sürekli yük altında iptal yayılım SLA'sı ölçülmedi.

Isınma dahil **600.738** başarılı istemci çağrısı, aynı sayıda upstream yürütme, audit admission ve başarılı outcome ile eşleşti. Çift yürütme, eksik operasyon ve başarısız çağrı sıfır. Tek instance oturum ve maskelenmiş/sayfalanmış metadata kontrolleri geçti. Raporun bazı eski kontrol isimlerinde “both nodes” ifadesi bulunsa da koşu tek aktif data-plane ile çalıştı; çoklu replica davranışı bu deneyin kanıtı değildir.

Yük sonrası host snapshot tanılama ölçümü: ilk kurulum **25,951 s**, önceki snapshot'ı kullanan refresh **2,934 s**, 5 saniyelik bütçe içinde başarılı. Host profili podun bellek limiti altında alınmadığı için pod içi GC ölçümü yerine kullanılmadı. Büyük katalogun ilk oluşturulma maliyeti devam ediyor; bellek hedefini artırmak bu yapısal maliyeti ortadan kaldırmıyor.

Operation metric label overflow sayacı 1.177.348'e ulaştı. [Telemetri kodu](../../core/internal/telemetry/operations.go) 2.048 farklı label setinden sonra yeni serileri `__overflow__` kovasında birleştiriyor. Bu çağrı hatası değildir; geniş katalogda tool bazında metrik ayrıntısının sınırlı olduğunu gösterir.

## Bellek tool sayısına mı TPS'ye mi bağlı?

Bu deneylerde **kalıcı canlı heap'in baskın belirleyicisi katalog büyüklüğü**: 6.000 tool'da yaklaşık 99 MiB, 60.000 tool'da yaklaşık 782 MiB. Ancak tool tanımının/schema'nın büyüklüğü, binding'ler, derlenmiş routing bilgisi ve snapshot yapısı da önemlidir; tool başına sabit byte katsayısı çıkarılamaz.

TPS ise geçici allocation hızını, eşzamanlı istek sayısını ve GC'nin ne kadar sık çalışacağını etkiler. Daha uzun upstream/audit beklemeleri aynı TPS'de daha çok isteği bellekte tutabilir. Snapshot yenilemesi sırasında eski/yeni verinin birlikte bulunması da ek alan gerektirir. Bu nedenle “bellek yalnız tool sayısına bağlı, TPS'den bağımsız” sonucu doğru değildir.

Bu workload için 8GiB hedefinin GC baskısını giderdiği ve 1.000 TPS'yi taşıdığı doğrulandı. **8GiB'nin gerekli minimum olduğu doğrulanmadı**; gözlenen ~1,70GiB RSS'ye bakarak 2GiB'nin refresh anında yeterli olacağı da garanti edilemez. 60.000 tool için daha düşük bellek hedefinin sınırı ayrıca ölçülmelidir. Maksimum TPS veya üretim kaynak önerisi bu tek koşudan çıkarılmadı.

## Tekrar çalıştırma ve kanıtlar

```sh
VMCP_ACCEPTANCE_SERVERS=300 VMCP_ACCEPTANCE_TOOLS=200 \
VMCP_ACCEPTANCE_NODES=1 VMCP_ACCEPTANCE_ROUTING=pod \
VMCP_ACCEPTANCE_RPS=1000 VMCP_ACCEPTANCE_STEADY_SECONDS=600 \
VMCP_ACCEPTANCE_WORKERS=1024 VMCP_ACCEPTANCE_OWNERSHIP=1 \
VMCP_ACCEPTANCE_DIAGNOSTICS=1 VMCP_ACCEPTANCE_GOMEMLIMIT=8GiB \
python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8 \
  --data-plane-memory-limit 12Gi --postgres-tuned \
  --output artifacts/benchmark-runs/memory-60000-single-8g-repeat
```

- [Bu koşunun raporu](microk8s-20261027-085716/report.md), [özeti](microk8s-20261027-085716/summary.json), [yeniden üretim ve arşiv checksum bilgisi](microk8s-20261027-085716/reproduction.json).
- [Önceki 60.000 tool tanı raporu](team-project-performance-diagnosis-20261027.md).
- [6.000 tool / 2–4–8GiB raporu](gomemlimit-single-6000-20261027.md).

Ham kanıtlar: `artifacts/benchmark-runs/memory-60000-single-8g-1000-20261027/`. Arşiv: `memory-60000-single-8g-1000-20261027-cee16dcc1162.tar.gz`; SHA-256: `df4ef6cacafec1ec94b50e9bcd153b99a7d4bf9ebe993d2959e49347f394f478`.

İzole test namespace'i kaldırıldı. `vmcp-demo` aktif pod UID'leri ve restart sayıları öncesi/sonrası aynı. Tek Kubernetes warning, yük/key kontrollerinden sonra temizlik sırasında control-plane readiness 503 kaydıydı.
