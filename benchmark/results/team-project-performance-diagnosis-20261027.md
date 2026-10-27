# Ekip/proje: performans ve key iptali teşhisi — 27 Ekim 2026

**Sonuç:** 10 dakikalık tekrar yük eşiğini geçti. Key iptali problemi yeniden üretildi ve bellek hedefiyle yapılan kontrollü karşılaştırmada ayrıştırıldı. NodePort bağlantı reddi yeniden yakalandı; bağlantıyı reddeden ağ bileşeninin kesin nedeni bu ölçümle belirlenemedi. Ürün runtime kodu değiştirilmedi; benchmark teşhis kayıtları genişletildi.

## 10 dakikalık tekrar

Önceki sahipli senaryo korundu: 300 upstream, 300 vMCP, 60.000 tool ve ekipler arası approved grant, 600 kalıcı oturum, 64 agent key; 15 saniye ısınma ardından 600 saniye 1.000 çağrı/s. İki data-plane: GOMAXPROCS=8, CPU limiti 8, GOMEMLIMIT=768MiB, Kubernetes bellek limiti 3GiB. PostgreSQL tuned, audit strict. İzole MicroK8s namespace'i koşu sonunda kaldırıldı.

| Ölçüm | Önceki sahipli koşu | Teşhis tekrarı |
|---|---:|---:|
| Başarılı çağrı/s | 989,9 | **999,8** |
| Gönderilenlerde başarı | %99,001 | **%99,99967** |
| p95 / p99 | 123,0 / 146,6 ms | **123,1 / 147,6 ms** |
| Hata | 5.993 | **2** |
| Gönderilemeyen | 33 | **59** |
| Yük SLO | Geçmedi | **Geçti** |
| Genel kabul | Geçmedi | **Key yayılım kontrolü nedeniyle geçmedi** |

Tekrarda iki hata HTTP 502; HTTP 503 dalgası oluşmadı. Isınma dahil 600.689 başarılı çağrının tamamı audit admission ve outcome ile eşleşti; çift çalıştırma, eksik outcome veya bozuk sonuç yok. Ek teşhis gözleminin küçük bir maliyeti olabilir; ölçümler tek koşudur.

## 1. Key iptalinin kök nedeni: snapshot yenileme bütçesi ve bellek baskısı

İptal sonrasında PostgreSQL revision **80** oldu. Her iki node'un readiness çıktısı `observedRevision:80`, `revision:79` gösterdi. Dolayısıyla bildirim/revision okuması çalışıyor; sorun yeni snapshot'ın tamamlanıp yayımlanamaması.

- Yenileme süresi yaklaşık **5,27 saniye**; [`Syncer.refresh`](../../core/internal/snapshot/syncer.go) toplam işi en fazla 5 saniyelik context ile sınırlar.
- Mevcut snapshot'ın son doğrulaması eskir. Yaklaşık 35. saniyede readiness: `authorization snapshot confirmation expired`; `maxAgeNs=30000000000`.
- Key çağrısı **401 yerine 503 `authorization configuration unavailable`** döner. Bu gözlemde eski key'in başarılı kabulü yoktur; yetkilendirme tazelik sınırı aşılınca sistem erişimi kapatır. İptalin ilk 30 saniyesindeki tüm anlar ayrı ayrı denenmedi.
- Süre aşımında context hatası oluştuğundan `refresh` içindeki `if ctx.Err() == nil` koşulu uyarı logunu da bastırır. Önceki testin birleşik hata mesajı bu ayrımı gizliyordu.

### Kontrollü doğrulama

Aynı data-plane binary SHA-256: `821f108e62ab4dd49d0d8f4be299115b742005c45183f0dae5976773ad4d12e1`.

Aynı envanter/CPU/pod limitleriyle yalnızca Go bellek hedefi **1536MiB** yapıldı. Bu takip koşusu 15 saniye ısınma + **5 saniye yük** içerir; 10 dakikalık performans sonucu yerine kullanılamaz.

| Key kontrolü | 768MiB / ana koşu | 1536MiB / kısa doğrulama |
|---|---|---|
| Hedef / yayımlanan revision | 80 / 79 | 80 / 80 |
| Snapshot yayılımı | 30 saniye içinde tamamlanmadı | Node 1: **3,184 s**, node 2: **3,101 s** |
| Readiness | 503 | 200 |
| İptal edilen key | 503: konfigürasyon kullanılamıyor | **401: invalid agent key** |
| Key kabul kontrolü | Başarısız | **Başarılı** |

Bu karşılaştırma bellek baskısının 5 saniyelik yenileme bütçesini aşırdığını güçlü biçimde doğrular. Süre aşımı sınırını büyütmek tek başına kalıcı çözüm değildir: key değişimi halen tüm snapshot'ın yeniden yüklenmesine bağlıdır. Takip koşusu kısa olduğundan 24-event polling probe bitmedi; genel acceptance bu nedenle başarısızdır. Bu durum key kontrolünün başarılı olduğu gerçeğinden ayrı tutulmalıdır. Kısa koşunun CPU medyanları çoğunlukla ısınmayı yansıtır ve ana koşuyla throughput/CPU karşılaştırması olarak kullanılmaz.

## 2. Darboğaz: büyük canlı snapshot ve sürekli GC

Ana koşuda pod başına medyan CPU yaklaşık **7,67 çekirdek**, tepe RSS **954 / 975 MiB**. Korunan GC log penceresinde canlı heap medyanı **775 MiB**, heap hedefi de yaklaşık **775 MiB**. 768MiB Go hedefi canlı veri için yeterli boşluk bırakmıyor; soft limit RSS veya Kubernetes hard limit değildir.

GC saniyede yaklaşık **5,8–6,0** kez çalışıp çok az bellek geri kazanıyor. Aynı log penceresindeki CPU sürelerinden yaklaşık **4,9 çekirdek eşdeğeri GC işi** hesaplanıyor (idle GC worker süreleri dahil; tüm koşuya ait uygulama CPU profili değildir). Ana koşunun GC logunda eski 3.000 satır sınırı nedeniyle yalnızca son bölüm kaldı; teşhis modunda bu sınır sonraki koşular için kaldırıldı.

Yük sonrasındaki ayrı host profilinde:

- İlk snapshot yüklemesi **27,76 s**; mevcut REST şemalarını kullanan yenileme **3,22 s**.
- Birleşik profil CPU örneklerinin kümülatif yaklaşık %55'i `restapi.Compile`, %49'u `compileSchema`, %28'i arka plan GC yolunda. İç içe kümülatif yüzdeler toplanmaz.
- Bu profil ilk yükleme + yenilemeyi içerir; her tool çağrısında schema derlendiği anlamına gelmez. Host, podun Go bellek hedefine tabi değildir.

Sahiplik/grant sorgusu yalnızca iki başlangıç snapshot'ında çalıştı: 60.000 grant okuması ortalama **44,5 ms**. Bu gözlemde tool çağrısı başına ownership SQL maliyeti veya grant tablosunda darboğaz görülmedi.

## 3. HTTP hataları: NodePort bağlantı reddi ve proxy büyütmesi

Teşhis koşusunda 02:49:37 ve 02:49:47 UTC'de test proxy'si node 1 için şu hatayı kaydetti:

```text
dial tcp 192.168.1.106:30883: connect: connection refused
```

Bu, HTTP/tool/authorization işlemi başlamadan TCP bağlantı kurulumunda oluşuyor. Proxy her transport hatasında ilgili node'u 10 saniye devre dışı bırakıyor; her iki node devre dışıysa anında 503 üretiyor ve POST'u tekrar göndermiyor. Önceki koşudaki iki 502 ardından 28–34. saniyelerde 5.991 adet 503 deseni bu mekanizmayla uyumlu. Önceki koşu transport nedenlerini kaydetmediğinden oradaki iki 502'nin kesin ağ nedeni geriye dönük kanıtlanamaz.

Yüksek bellek hedefli kısa koşuda da farklı NodePort üzerinde bir `connection refused` oluştu. Bu nedenle bu hatayı yalnızca GC/bellek problemine bağlamak doğru değildir.

Yük aşamasında pod restart yok. Kubernetes warning kayıtlarında yük sırasında readiness kaybı yok; kayıtlı pod condition zamanları başlangıçtaki Ready geçişini gösteriyor. Endpoint watch erken iki hatadan sonra başlatıldı ve sonraki yük bölümünde endpoint kaybı göstermedi. Host sayaçlarında listen overflow/drop ve bellek nedeniyle TCP abort artışı yok. Bunlar NodePort/conntrack/firewall arızasını tek başına dışlamaz.

**Açık kalan nokta:** RST/connection-refused üreten tam bileşen belirlenemedi. Yalnız iki test portunun SYN/RST başlıklarını yakalama denemesi, tcpdump capability'si ve parolasız sudo olmadığı için çalışmadı. Yeni ayrıcalıklı pod oluşturma veya cluster ağ ayarını değiştirme yapılmadı. Bir sonraki ağ incelemesi, hata anında yetkili SYN/RST yakalamasıyla NodePort ve doğrudan PodIP yollarını birlikte gözlemlemeli.

## 4. PostgreSQL ve kaynak sınırları

- Yük örneklerinde **0 deadlock**, **0 lock nedeniyle bloklanan örnek**, en çok **143 bağlantı**.
- Data-plane pool tepe kullanımı **40/64 ve 46/64**; iptal edilen pool acquisition **0**.
- Yoğun audit/delivery sorgularının ortalamaları örneklerde yaklaşık 0,1–0,3 ms. Başlangıç upstream sorgusunun yaklaşık 20 s süresi, istemci tarafındaki büyük konfigürasyon işleme/akış beklemelerini de içerebilir; tek başına SQL plan darboğazı değildir.
- Yaklaşık 595 saniyelik SQL örnek aralığında 3,19 GB WAL, 218.837 sync ve toplam 518 s sync süresi: ortalama yaklaşık **2,37 ms/sync**. Durable audit maliyeti kapasite hesabında önemlidir, ancak bu ölçümde key/NodePort hatasının kanıtlanmış nedeni değildir.
- İç yük penceresinde cgroup throttled süre artışı node başına **0,81 / 0,64 s**; OOM yok. Host memory pressure yok; I/O pressure düşük ama sıfır değil. Mevcut kanıtlar ağır SQL lock/pool tükenmesini işaret etmiyor.

## Öncelikli düzeltme sırası

1. **Key/grant gibi yetki değişikliklerini büyük katalog yeniden yüklemesinden ayırmak.** Küçük yetki snapshot'ını atomik güncelleyip mevcut tazelik ve pre-dispatch kontrollerini korumak; 60.000 tool üzerinde iptal testini kabul ölçütü yapmak.
2. **Snapshot'ın canlı bellek ve geçici allocation maliyetini azaltmak.** Değişmeyen upstream/tool verisini yeniden kullanmak; immutable derlenmiş schema paylaşımını doğru tenant/auth/hash sınırlarıyla değerlendirmek. 1536MiB sonucu bu envanter için geçici kapasite önlemidir; uzun yük altında ayrıca doğrulanmadan üretim boyutlandırması sayılmamalı.
3. **Yenileme timeout'unu görünür kılmak.** Observed/published revision, başarısız yenileme nedeni ve süre aşımı sayaçları; log seli yaratmadan timeout kaydı. Deadline'ı yalnız büyütüp sorunu gizlememek.
4. **NodePort hatasını ağ düzeyinde izlemek ve test proxy'sinin amplifikasyonunu ayrı raporlamak.** Testi geçirmek için POST replay eklenmemeli; tekrar çalıştırma riski doğurur.

## Kanıtlar ve tekrar

- [10 dakikalık ölçüm](microk8s-20261027-054711/report.md), [özet](microk8s-20261027-054711/summary.json), [arşiv kimliği](microk8s-20261027-054711/reproduction.json).
- [Kısa bellek doğrulaması](microk8s-20261027-060146/report.md), [özet](microk8s-20261027-060146/summary.json), [arşiv kimliği](microk8s-20261027-060146/reproduction.json).
- Ham `run.log`, revision/readiness, sampled Prometheus, PostgreSQL, host/cgroup, pod/endpoint watch ve `snapshot-host.cpu.pprof`: `artifacts/benchmark-runs/ownership-diagnostic-20261027/`.
- Takip koşusu: `artifacts/benchmark-runs/ownership-memory-probe-20261027/`.
- Benchmark derleme kontrolü ve 11 Python birim testi geçti. Runtime için ürün davranışı değişmedi; bu çalışma teşhis ve ölçüm kapsamındadır.

```sh
VMCP_ACCEPTANCE_STEADY_SECONDS=600 VMCP_ACCEPTANCE_OWNERSHIP=1 VMCP_ACCEPTANCE_DIAGNOSTICS=1 python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8 --postgres-tuned --output artifacts/benchmark-runs/ownership-diagnostic-repeat
```

Yeni output dizini kullanılmalı. Kontrollü bellek deneyi için `VMCP_ACCEPTANCE_GOMEMLIMIT=1536MiB` eklenir. İki koşunun data-plane binary hash'i aynı; benchmark teşhis/raporlama dosyaları değiştiğinden kaynak ağacı hash'leri farklı olabilir. Test namespace'leri ve geçici watch süreçleri temizlendi.
