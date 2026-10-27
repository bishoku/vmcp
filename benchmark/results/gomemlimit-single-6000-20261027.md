# Tek data-plane / 6.000 tool: Go bellek hedefi — 27 Ekim 2026

**Sonuç:** Tek data-plane, 6.000 tool ve 1.200 TPS yükte Go bellek hedefini **2 / 4 / 8 GiB** yapmak belirgin bellek, GC veya gecikme kazancı sağlamadı. Her profil 10 dakika çalıştı ve kabul eşiklerini geçti. Canlı heap **99 MiB**, gerçek GC heap hedefi yaklaşık **201 MiB**, GC sıklığı yaklaşık **1/s** olarak kaldı. 8GiB koşusunda bir istemci `unexpected EOF` hatası var; sıfır hata olarak sunulmamalı.

## Karşılaştırma düzeni

Önceki doğrulanmış tek-instance workload'u korundu: 300 upstream × 20 tool = 6.000 tool, 300 vMCP, 600 kalıcı oturum, 64 key, ekipler arası approved grant, strict audit ve kalıcı delivery. Tek data-plane, GOMAXPROCS=8, CPU limiti 8, DB pool=64; tuned PostgreSQL ve Redis. Her koşu yeni izole MicroK8s namespace'inde, 15 s / 50 TPS ısınma ardından **600 s / 1.200 TPS**. 1.024 worker; doğrudan PodIP/CNI üzerinden aynı test proxy'si. Koşular seri çalıştı; yük sırasında runtime/DB ayarı değiştirilmedi.

Değişken: `GOMEMLIMIT=2GiB`, `4GiB`, `8GiB`. GB isteği önceki MiB/GiB ayarlarıyla tutarlı biçimde ikili GiB biriminde uygulandı. Üç koşuda Kubernetes hard memory limiti **12Gi**, request **2Gi** olarak sabit. Bu hard limit, 8GiB Go hedefinin container tarafından önceden kesilmesini engeller; ayrılmış/kullanılan bellek değildir. Host başlangıçta yaklaşık 140 GiB kullanılabilir belleğe sahipti.

Önceki 768MiB referans koşusunun hard limiti 3Gi idi; dolayısıyla yeni üç profil kendi aralarında aynı hard limit ile, eski referansla bu fark belirtilerek karşılaştırılır. Referans RSS'si 250 MiB altında olduğundan 3Gi hard limite yaklaşmıyordu. CPU, tool envanteri ve trafik aynı. Ürün runtime binary hash'i her koşu sonunda karşılaştırıldı.

Bu deney sabit **1.200 TPS'deki davranışı** ölçer; 2/4/8GiB için maksimum TPS sınırı yeniden aranmaz. Önceki 1.300 TPS başarısızlığının ortadan kalktığı veya daha fazla maksimum TPS elde edildiği bu testten çıkarılamaz. Geçerli SLO: başarı ≥%99,9, başarılı hız hedefin ≥%95'i, p95 ≤250 ms, p99 ≤1.000 ms, bozuk sonuç yok. Gönderilemeyen talep ayrıca gösterilir.

## Sonuçlar

| Go hedefi | Başarılı TPS | p95 / p99 ms | Hata | Gönderilemeyen | Genel kabul |
|---|---:|---:|---:|---:|---|
| 768 MiB — önceki referans | 1.199,8 | 108,9 / 152,8 | 0 | 50 | Geçti |
| 2 GiB | 1.199,8 | 109,4 / 155,8 | 0 | 41 | Geçti |
| 4 GiB | 1.199,9 | 107,6 / 143,2 | 0 | 13 | Geçti |
| 8 GiB | 1.199,9 | 108,8 / 152,6 | **1 EOF** | 10 | Geçti |

| Go hedefi | Medyan / tepe RSS MiB | Canlı heap / heap goal MiB | GC/s | GC CPU çekirdek eşdeğeri | Medyan data-plane CPU, çekirdek |
|---|---:|---:|---:|---:|---:|
| 768 MiB | 237,1 / 248,9 | 99 / 201 | 1,004 | 0,122 | 2,000 |
| 2 GiB | 243,6 / 263,9 | 99 / 201 | 0,994 | 0,121 | 2,005 |
| 4 GiB | 242,2 / 258,0 | 99 / 200,5 | 0,996 | 0,119 | 1,983 |
| 8 GiB | 241,3 / 249,4 | 99 / 201 | 0,994 | 0,119 | 1,955 |

## Hata, key iptali ve doğruluk

2GiB ve 4GiB koşularında hata yok. 8GiB koşusunda **719.990 gönderilen isteğin biri** `unexpected EOF` ile başarısız; başarı oranı **%99,999861**. İlgili çağrı HTTP 200 aldı ancak istemci yanıt gövdesini tamamlayamadı; bu nedenle `vmcp_http_errors_total` sayacı sıfır kalırken acceptance istemci sayacı hatayı yakaladı. Hata yükün 22. saniyesindeki kovada. Bu durum HTTP durum kodu ile başarılı MCP sonucunun ayrı ölçülmesinin önemini gösterir.

Data-plane loglarında karşılık gelen error/warning veya proxy transport-error kaydı yok. Test proxy'sinin `ErrorLog` çıktısı discard edildiğinden response-body kopyalama hatası ayrıntısı saklanmamış olabilir; mevcut [proxy kodu](../../core/internal/e2e/acceptance_runtime_test.go) ve [istemci okuma yolu](../../core/internal/e2e/acceptance_fixture_test.go) bu gözlem boşluğunu açıklar. Kesin stream/ağ kök nedeni belirlenemedi; tek örneği 8GiB hedefinin neden olduğu bir regresyon olarak yorumlamak için kanıt yok. Takip incelemesi için diagnostic modda response-body/proxy logu ve hata başına request ID/okuma aşaması kaydı gerekir. Bu testte isteğe otomatik retry uygulanmadı.

8GiB koşusunda ısınma dahil 720.739 başarılı istemci çağrısı aynı sayıda admission ve başarılı outcome ile eşleşti. Upstream yürütme sayacı 720.740; istemci başarısızlığı, işlemin upstream'de hiç çalışmadığı anlamına gelmez. Eksik audit operation ve çift yürütme sayısı sıfır. 2GiB / 4GiB koşularında sırasıyla 720.709 / 720.737 başarılı çağrı, audit eşleşmesi ve upstream yürütme sayıları aynı. Tüm profillerde bozuk sonuç, audit write failure ve pool acquisition iptali sıfır.

| Go hedefi | Key iptali → snapshot yayınlama | İptal edilen key / readiness |
|---|---:|---|
| 2 GiB | 365 ms | 401 / 200 |
| 4 GiB | 345 ms | 401 / 200 |
| 8 GiB | 363 ms | 401 / 200 |

Key kontrolleri yük sonrasında yapıldı; tablo yayınlama süresini gösterir, her an ayrı istemci probuyla ölçülmüş revocation SLA'sı değildir. Yük örneklerinde data-plane readiness kaybı yok. Kubernetes warning kayıtları yük/verification sonrasındaki test temizliğinde control-plane readiness 503'lerine aittir; 8GiB erken EOF anıyla örtüşmez.

## Ölçüm yöntemi

- RSS ve CPU: 600 saniyelik `steady` örneklerinden medyan/tepe; kubelet container RSS ve yakın dönem CPU.
- Canlı heap, heap goal, GC sıklığı ve GC CPU işi: GC logunun son kaydından 60 s önce biten, önceki 400 s pencere. Aynı yöntem önceki kapasite raporunda kullanıldı. GC CPU işi idle worker sürelerini de içerir; uygulama CPU profili değildir.
- p95/p99 başarılı çağrılara aittir. Hatalar ve gönderilemeyen istekler ayrı sayılır.
- Yük sonrasında key iptali/revision/readiness, audit admission/outcome eşleşmesi, duplicate execution ve session kontrolleri.
- Her profil için tek koşu: küçük gecikme/CPU farkları istatistiksel olarak kanıtlanmış optimizasyon sayılmaz; PostgreSQL/WAL, checkpoint, OS ve koşu değişkenliği bulunur.

## GOMEMLIMIT neden daha fazla bellek kullanımı anlamına gelmez?

Go bellek hedefi soft üst sınırdır; runtime'ın doldurması gereken rezervasyon değildir. Bu uygulamada `GOGC` değiştirilmedi (runtime varsayılanı 100). Bellek sınırı devreye girmediğinde GC'nin heap hedefi canlı veri ve GOGC tarafından belirlenir. Yaklaşık 100 MiB canlı heap ile yaklaşık 200 MiB heap hedefi zaten 768MiB sınırından küçüktür; yalnız GOMEMLIMIT'i yükseltmek bu durumda GC aralığını büyütmek zorunda değildir. Kurulu Go runtime kaynakları: `/usr/local/go/src/runtime/extern.go`, `/usr/local/go/src/runtime/debug/garbage.go`; uygulamada `SetGCPercent`/`SetMemoryLimit` override'ı bulunmadı.

Bu yorum 6.000 tool workload'u içindir. Önceki 60.000 tool / yaklaşık 775 MiB canlı heap koşusunda 768MiB hedefi baskı yaratıyordu; o envanterin davranışı farklıdır.

## Değerlendirme

Bu envanter/yük için **4 veya 8GiB'ye çıkmanın ölçülmüş bir avantajı yok**; 2GiB de 768MiB referansından belirgin hızlı değil. Daha fazla belleğe izin vermek tek başına Go'nun GC aralığını artırmıyor. Referansın yaklaşık 237MiB RSS'si ve 201MiB heap hedefi, mevcut bellek bütçesinin bu sabit yükte yeterli olduğunu gösteriyor. Önceki kapasite sınırındaki audit/delivery/WAL beklemelerinin giderildiği bu deneyle iddia edilemez; maksimum TPS araması yapılmadı.

Üç yeni koşuda ve referansta aynı ürün binary SHA-256 kullanıldı: `821f108e62ab4dd49d0d8f4be299115b742005c45183f0dae5976773ad4d12e1`. 2/4/8GiB arasında görülen yaklaşık 1–2 ms p95 ve küçük CPU/RSS farkları tek koşuluk oynaklık düzeyindedir; istatistiksel anlamlılık iddiası yok.

## Tekrar çalıştırma

Yeni output diziniyle aşağıdaki komut çalıştırılır; `2GiB` yerine `4GiB` veya `8GiB` yazılır. Profiller aynı host üzerinde ardışık çalıştırılmalıdır.

```sh
VMCP_ACCEPTANCE_SERVERS=300 VMCP_ACCEPTANCE_TOOLS=20 \
VMCP_ACCEPTANCE_NODES=1 VMCP_ACCEPTANCE_ROUTING=pod \
VMCP_ACCEPTANCE_RPS=1200 VMCP_ACCEPTANCE_STEADY_SECONDS=600 \
VMCP_ACCEPTANCE_WORKERS=1024 VMCP_ACCEPTANCE_OWNERSHIP=1 \
VMCP_ACCEPTANCE_DIAGNOSTICS=1 VMCP_ACCEPTANCE_GOMEMLIMIT=2GiB \
python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8 \
  --data-plane-memory-limit 12Gi --postgres-tuned \
  --output artifacts/benchmark-runs/memory-6000-single-2g-repeat
```

Benchmark'a yalnız izole data-plane hard memory limitini seçebilmek için `--data-plane-memory-limit` eklendi; varsayılan 3Gi korundu. Rapor artık gerçek Go/hard limitlerini ve steady hedef TPS'yi yazar. Üretim deployment ayarları değiştirilmedi.

## Kanıtlar

- [768 MiB referansı](microk8s-20261027-072708/report.md), [özet](microk8s-20261027-072708/summary.json).
- [2 GiB koşusu](microk8s-20261027-081515/report.md), [özet](microk8s-20261027-081515/summary.json).
- [4 GiB koşusu](microk8s-20261027-082711/report.md), [özet](microk8s-20261027-082711/summary.json).
- [8 GiB koşusu](microk8s-20261027-083911/report.md), [özet](microk8s-20261027-083911/summary.json).
- [Önceki 6.000 tool kapasite karşılaştırması](tool-catalog-6000-capacity-20261027.md).

Ham Prometheus/GC/PostgreSQL/host/cgroup/pod kayıtları `artifacts/benchmark-runs/memory-6000-single-{2,4,8}g-20261027/` dizinlerindedir. Arşiv checksum'ları ve yeniden üretim bilgileri koşu başına `reproduction.json` içinde korunur. Testler sentetik upstream ve PodIP yoluna aittir; üretim ingress, gerçek upstream gecikmeleri ve 60.000 tool katalog için genellenmemelidir.

Üç koşunun bütün acceptance kapıları geçti. Benchmark derleme/kapsam kontrolü ve 11 Python testi geçti. Geçici test namespace’leri kaldırıldı; mevcut vmcp-demo aktif podlarının UID/restart sayıları her koşuda korundu.
