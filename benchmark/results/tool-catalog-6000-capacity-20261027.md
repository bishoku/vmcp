# 6.000 tool: bellek, GC ve data-plane kapasitesi — 27 Ekim 2026

**Sonuç:** 60.000 → 6.000 tool geçişi canlı heap medyanını yaklaşık %87, pod RSS medyanını yaklaşık %75 azalttı. Aynı 6.000 tool workload’unda 600 saniye boyunca doğrulanan kapasite tek instance ile **1.200 TPS**, iki instance ile **2.000 TPS**: yaklaşık **%67 artış / 1,67×**. Her iki doğrulamada sıfır hata, key iptali ve audit tutarlılığı başarılı. 1.300 / 2.200 TPS uzun koşuları kabul edilmedi.

| 600 saniyelik doğrulama | Tek data-plane | İki data-plane |
|---|---:|---:|
| Hedef / başarılı TPS | 1.200 / 1.199,8 | 2.000 / 1.999,7 |
| p95 / p99 | 108,9 / 152,8 ms | 104,2 / 143,4 ms |
| Hata | 0 | 0 |
| Gönderilemeyen | 50 / 720.000 | 85 / 1.200.000 |
| Medyan RSS, pod başına | 237 MiB | 244 / 243 MiB |
| Medyan CPU, pod başına | 2,00 çekirdek | 1,86 / 1,88 çekirdek |
| Key iptali → snapshot yayınlama | 385 ms | 342 / 357 ms |

## Yöntem ve kapasite tanımı

300 upstream × 20 tool = **6.000 tool**; 300 vMCP, 600 kalıcı MCP oturumu, 64 agent key, ekipler arası 6.000 approved grant. Önceki 60.000 tool koşusundaki upstream/vMCP/oturum/key sayıları korundu; upstream başına tool sayısı 200'den 20'ye indirildi. Sentetik REST upstream yaklaşık 5 ms yanıt gecikmesi kullanır. Strict kalıcı audit, delivery/session persistence ve sahiplik denetimleri açık.

Her data-plane: **8 CPU**, GOMAXPROCS=8, **GOMEMLIMIT=768MiB**, Kubernetes hard memory limit=3GiB, DB pool=64. PostgreSQL tuned ayarları, fsync ve synchronous commit açık; PostgreSQL CPU/bellek hard limiti yok. Aynı MicroK8s host üzerinde ardışık koşular; eşzamanlı yük testi yapılmadı. Yük sürücüsü 1.024 worker kullanır. Bir-instance koşusunda ikinci Deployment sıfır replica'dadır; iki-instance koşusunda iki ayrı data-plane podu çalışır. PostgreSQL ve Redis paylaşılır.

Önceki NodePort `connection refused` hatasını uygulama kapasitesinden ayırmak için bu koşular **test round-robin proxy → doğrudan PodIP/CNI** yolunu kullanır. POST retry yok. Bu, production ingress/NodePort kapasite testi veya trafik sırasında canlı scale-out geçiş testi değildir. Instance sayısı koşu başlangıcında ayarlanır.

Kapasite, mevcut acceptance SLO ile tanımlanır: gönderilen isteklerde başarı ≥%99,9; başarılı TPS hedefin ≥%95'i; başarılı yanıtların p95'i ≤250 ms, p99'u ≤1.000 ms; bozuk sonuç yok. Gönderilemeyen talep ayrıca raporlanır. Kısa arama: her basamak 60 saniye; ilk başarısızlıktan sonra en fazla üç daraltma, aralarda 30 saniye/100 TPS toparlanma. En yüksek kısa başarılı hız ardından **600 saniye** doğrulanır. Kısa test başarısı uzun süreli kapasite sayılmaz.

## Tek instance

| Aşama | Süre | Hedef TPS | Başarılı TPS | p95 / p99 ms | Hata | Gönderilemeyen | SLO |
|---|---:|---:|---:|---:|---:|---:|---|
| Tarama | 60 s | 500 | 499,7 | 37,9 / 46,5 | 0 | 0 | Geçti |
| Tarama | 60 s | 1.000 | 999,1 | 79,4 / 102,9 | 0 | 11 | Geçti |
| Tarama | 60 s | 1.500 | 1.318,5 | 778,6 / 802,5 | 10.101 | 98 | Geçmedi |
| Daraltma | 60 s | 1.200 | 1.198,9 | 110,3 / 134,9 | 0 | 0 | Geçti |
| Daraltma | 60 s | 1.300 | 1.298,6 | 159,3 / 186,2 | 0 | 0 | Geçti |
| Uzun doğrulama | 600 s | 1.300 | 1.299,1 | 644,7 / 701,4 | 372 | 54 | Geçmedi |
| Ayrı uzun doğrulama | 600 s | **1.200** | **1.199,8** | **108,9 / 152,8** | **0** | 50 | **Geçti** |

**Doğrulanmış tek-instance kapasite 1.200 TPS.** Denenen 1.300 TPS uzun koşuda gecikme eşiğini aştı. Bu, bu workload/altyapı/SLO için gözlenen 1.200–1.300 TPS geçiş aralığıdır; evrensel veya istatistiksel güven aralığı değildir. 1.200 doğrulaması yeni, aynı fixture'larla hazırlanmış namespace'te çalıştı; 1.300 uzun koşusu kapasite taramasını izledi. Koşu yaşı ve önceki aşamaların DB üzerindeki etkisi tamamen eşit değildir.

1.300 uzun koşusunda 372 hata ve 1.500 kısa koşusunda 10.101 hata HTTP 429 `request concurrency limit exceeded`. Bu mesaj [handler admission kontrolünden](../../core/internal/virtual/handler.go) gelir: instance başına global 1.024, vMCP başına 128, principal başına 16 eşzamanlı istek sınırı. Hangi alt sayacın her reddi tetiklediği ayrı ölçülmedi. Bunlar TCP/proxy hatası değildir.

İlk tek-instance taramasının ham `Resources` özetinde eski sabit iki-node özetleyicisinden kalan sıfır değerli node-2 satırı vardır; gerçek örneklerde yalnız node-1 çalışır. Bu satır kapasite/kaynak hesabına katılmadı ve özetleyici sonraki koşular için düzeltildi.

## İki instance

| Aşama | Süre | Hedef TPS | Başarılı TPS | p95 / p99 ms | Hata | Gönderilemeyen | SLO |
|---|---:|---:|---:|---:|---:|---:|---|
| Tarama | 60 s | 500 | 499,6 | 35,8 / 44,9 | 0 | 5 | Geçti |
| Tarama | 60 s | 1.000 | 999,3 | 57,6 / 79,3 | 0 | 0 | Geçti |
| Tarama | 60 s | 1.500 | 1.498,9 | 71,5 / 96,5 | 0 | 0 | Geçti |
| Tarama | 60 s | 2.000 | 1.998,5 | 98,2 / 120,6 | 0 | 0 | Geçti |
| Tarama | 60 s | 3.000 | 2.367,7 | 475,0 / 535,8 | 31.976 | 5.314 | Geçmedi |
| Daraltma | 60 s | 2.500 | 2.393,6 | 438,6 / 467,3 | 4.758 | 1.021 | Geçmedi |
| Daraltma | 60 s | 2.200 | 2.195,6 | 123,5 / 148,4 | 0 | 115 | Geçti |
| Daraltma | 60 s | 2.300 | 2.287,4 | 277,1 / 483,2 | 522 | 89 | Geçmedi |
| Uzun doğrulama | 600 s | 2.200 | 2.189,7 | 374,2 / 523,4 | 4.994 | 943 | Geçmedi |
| Ayrı uzun doğrulama | 600 s | **2.000** | **1.999,7** | **104,2 / 143,4** | **0** | 85 | **Geçti** |

2.200 TPS uzun koşusunda başarı %99,6214. 4.978 hata HTTP 503 `request delivery capacity unavailable`, 16 hata HTTP 429. [Delivery başlangıç yolu](../../core/internal/virtual/streams.go) `Store.Begin` hatalarını bu 503 ile döndürür. [Store.Begin](../../core/internal/delivery/store.go) veritabanında paylaşılan running-operation sayılarını global 1.024 / principal 16 / vMCP 128 limitleriyle denetler. Audit kuyruğu ve artan gecikme nedeniyle açık operation birikmesi, bu kapasite reddiyle uyumludur. Yanıt aynı zamanda diğer Begin hatalarını da kapsar; mevcut telemetry reddin alt nedenini ayrı saymadığı için her 503'ün hangi limite/SQL hatasına ait olduğu kesinleştirilmedi. Pool iptali, audit write failure ve transport/proxy hatası görülmedi.

Aynı toplam **1.000 TPS / 60 s** basamağında p95 79,4 → 57,6 ms (**%27,5 azalma**). Toplam medyan data-plane CPU yaklaşık 1,60 → 1,99 çekirdek; toplam medyan RSS yaklaşık 240 → 429 MiB. Yatay ölçekleme daha fazla toplam kaynak kullanır; katalog her podda bulunur. Tek-instance 1.500 TPS başarısızken iki-instance 1.500 ve 2.000 TPS kısa basamakları geçti. Ayrı 2.000 TPS / 600 s koşusu sıfır hatayla geçti. **Doğrulanmış hız oranı 1.999,7 / 1.199,8 = 1,67×**; iki instance için denenen geçiş aralığı 2.000–2.200 TPS. 2.100 TPS ayrıca ölçülmedi. Kaynak/SLO profili dışında genel bir maksimum hız iddiası değildir.

Başarılı iki-instance uzun koşusunda medyan RSS 244 / 243 MiB, CPU 1,86 / 1,88 çekirdek; canlı heap 98 MiB, GC 0,80/s ve yaklaşık 0,11 çekirdek GC CPU işi. PostgreSQL medyan CPU 8,48 çekirdek. Audit bekleyen kayıt medyanı 52 / 47, tepe 199 / 148; pool tepe 63/64 ve 64/64, acquisition iptali sıfır. Bu koşunun namespace’i de yeni fixture’larla kuruldu; uzun başarısız koşunun geçmiş yükü taşınmadı.

## Bellek ve GC

| Ölçüm, pod başına | 60.000 tool, önceki 2-instance / toplam 1.000 TPS | 6.000 tool, 1-instance / 1.200 TPS |
|---|---:|---:|
| Go soft memory hedefi | 768 MiB | 768 MiB |
| Canlı heap medyanı | 775 MiB | 99 MiB |
| GC heap hedefi medyanı | 775 MiB | 201 MiB |
| GC sıklığı | 5,86–6,02 /s | 1,00 /s |
| GC CPU işi, çekirdek eşdeğeri | ~4,93 | ~0,12 |
| Yük sırasında medyan data-plane CPU | ~7,67 çekirdek | 2,00 çekirdek |
| Yük sırasında medyan RSS | 934 / 943 MiB | 237 MiB |
| Yük sırasında tepe RSS | 954 / 975 MiB | 249 MiB |

Canlı heap yaklaşık **%87 azaldı**. Katalog on kat küçülse de oturumlar, runtime, HTTP, audit/delivery ve bağlantı havuzlarının sabit/yükle değişen maliyetleri kaldığından toplam RSS'nin tam on kat azalması beklenmez. GOMEMLIMIT RSS/hard limit değildir. GC CPU hesabı GC logunun CPU bileşenlerini toplar; idle GC worker sürelerini de içerir, uygulama CPU profili değildir.

GC karşılaştırması her logun son kaydından 60 saniye önce biten, önceki 400 saniyelik pencereyi kullanır; eski koşuda logun yalnız son 3.000 satırı saklandığı için tüm koşu GC ortalaması değildir. 6.000 tek-instance koşusunda süreç zamanı yaklaşık 178–578 s, önceki node'larda yaklaşık 320–720 / 280–680 s. Topoloji, TPS/pod ve ağ yolu farklıdır; bu tablo katalog küçülmesinin güçlü bellek etkisini gösterir, tüm CPU farkını yalnız katalog boyutuna atfetmez.

Yük sonrasındaki ayrı host snapshot profili: ilk yükleme **2,72 s**, mevcut derlenmiş REST şemalarıyla yenileme **324 ms**. Önceki 60.000 koşusunda 27,76 s / 3,22 s idi. Host profili pod limitleri altında alınmadı ve trafik gecikmesi değildir.

## Darboğaz bulguları

Tek instance 1.200 TPS'de medyan CPU 2,00 çekirdek; 1.300 TPS'de 2,21 çekirdek. 8 CPU limitinin tükenmesi gözlenmedi. 1.300 koşusunda pool 64/64'e ulaştı; yaklaşık 595 saniyelik yük örneklerinde toplam acquisition bekleme artışı 310 saniye. Bu toplam, eşzamanlı bekleyen çağrıların sürelerini toplar; 310 saniye kesinti anlamına gelmez. Pool acquisition iptali yok.

Daha belirgin bulgu **strict audit yazma kuyruğu**: tek-instance 1.500 TPS basamağında medyan yaklaşık 952, en çok 979 kayıt bekliyor; 1.300 uzun koşusunda medyan 131, tepe 947. Başarılı 1.200 koşusunda medyan 69, tepe 399. Bunlar readiness `audit.pendingBytes / 4096` hesabıdır; bu payload-capture kapalı fixture'da kayıt rezervasyonu 4.096 bayttır ve işlemdeki batch'i de kapsar.

[`Durable.flushPendingBatches`](../../core/internal/audit/durable.go) instance başına tek flush döngüsüyle en fazla 64 kaydı transaction içinde kalıcılaştırır; admission/outcome çağrıları bu onayı bekler. Böylece kalıcı yazma geciktiğinde audit kuyruğu ve açık HTTP istekleri birikir, admission limiti 429 üretir. Ölçüm ve kod birlikte bu yolu güçlü biçimde işaret eder. Batch kuyruk bekleme süresi, batch commit süresi ve pool beklemesi ayrı trace edilmediği için toplam gecikmedeki kesin payları belirlenmedi.

PostgreSQL 1.200 TPS ölçümünün yaklaşık 593 saniyesinde **3,81 GB WAL**, 237.849 sync, toplam **578 s WAL sync süresi** üretti; ortalama 2,43 ms/sync. WALWrite bekleyen oturum sayısı medyan 13, tepe 62. Medyan PostgreSQL CPU 4,43 çekirdek. Deadlock yok. SQL işlem süreleri tek başına COMMIT/fsync ve uygulama kuyruğu gecikmesini açıklamaz.

İki-instance 2.200 TPS uzun koşusunda data-plane başına medyan CPU **2,08 / 2,10 çekirdek**, RSS **248 / 253 MiB**, canlı heap **101 MiB**, GC yaklaşık **0,91/s**. Audit bekleyen kayıt medyanı 77 / 80, tepe 483 / 447; pool her iki podda 64/64'e ulaştı. Pool acquisition bekleme toplamı pod başına yaklaşık **859 / 861 s**. Tek-instance'dan iki-instance'a geçiş audit flush döngüsü ve pool kapasitesini paralelleştirir; ancak paylaşılan delivery limitlerini ve PostgreSQL maliyetini kaldırmaz.

Aynı 2.200 TPS koşusunda PostgreSQL medyan CPU **11,36 çekirdek**, yaklaşık 595 saniyede **9,54 GB WAL**, 200.607 sync, toplam **582,5 s sync** (ortalama 2,90 ms/sync). WALWrite bekleyen oturumlar medyan 34, tepe 118. Deadlock yok; bir relation-extension wait örneği var, blocker listesi boş. Bu, sürekli row-lock/notification-lock tıkanması kanıtı değildir.

İlk 591 hata, Prometheus sayaç farklarında **04:52:17–23 UTC** penceresinde; PostgreSQL checkpoint **04:52:17.956 UTC**'de tamamlandı (write 174,6 s, sync 0,637 s). Sonrasında yeni WAL checkpoint'leri var. Zamansal örtüşme kalıcı yazma baskısıyla uyumlu; checkpoint'i tek başına kanıtlanmış neden saymak için batch/pool/commit gecikmelerini ayrı ölçmek gerekir. 60 saniyelik basamakların uzun doğrulamadan daha iyi çıkması bu dalgalanmaların önemini gösterir.

## Key iptali ve doğruluk

Tek-instance iki uzun koşuda da yeni revision 80 yayımlandı, readiness 200 ve iptal edilen key **401** verdi. 1.200 TPS koşusunda iptal kaydı 04:38:14.878560801 UTC, `snapshot updated` 04:38:15.263985127 UTC: yaklaşık **385 ms**. 60.000 tool / 768MiB koşusundaki observed=80 / published=79 ve yenileme timeout'u yeniden oluşmadı. Bu, katalog küçültmenin bu senaryodaki sorunu kaldırdığını gösterir; 60.000 tool için ürün mimarisini düzeltmez. İptal kontrolü yük sonrasındadır, trafik altında mutation testi değildir.

İki-instance 2.000 TPS doğrulamasında key iptali 05:12:23.552967146 UTC, snapshot yayınları 05:12:23.895358481 / 05:12:23.909982984 UTC: **342 / 357 ms**. Her iki podda readiness 200 ve key 401; çapraz-node session/metadata, polling ve session termination kontrolleri geçti. Isınma dahil **1.200.665** başarılı çağrı aynı sayıda kalıcı admission/outcome ve upstream yürütmesiyle eşleşti; çift yürütme, eksik outcome, bozuk sonuç yok. Tek-instance başarılı koşuda da tüm kontroller geçti.

## Kanıtlar

- [Tek-instance tarama ve 1.300 TPS uzun koşu](microk8s-20261027-070807/report.md), [özet](microk8s-20261027-070807/summary.json).
- [Tek-instance 1.200 TPS / 600 s](microk8s-20261027-072708/report.md), [özet](microk8s-20261027-072708/summary.json).
- [İki-instance tarama ve 2.200 TPS uzun koşu](microk8s-20261027-073909/report.md), [özet](microk8s-20261027-073909/summary.json).
- [İki-instance 2.000 TPS / 600 s](microk8s-20261027-080107/report.md), [özet](microk8s-20261027-080107/summary.json).
- [Önceki 60.000 tool teşhisi](team-project-performance-diagnosis-20261027.md).

Ürün runtime binary'si değişmedi: `821f108e62ab4dd49d0d8f4be299115b742005c45183f0dae5976773ad4d12e1`. Benchmark'a instance sayısı, PodIP routing ve kapasite araması eklendi. Ham kanıtlar `artifacts/benchmark-runs/capacity-6000-*`; doğrulanabilir arşiv kimlikleri ilgili `reproduction.json` dosyalarındadır.

## Sonraki performans adımları

1. **Audit kuyruğunu ölçülebilir hale getirmek:** queue-wait, batch boyutu, batch/commit süresi ve pool-wait histogramlarını ayırmak. Strict admission'ın dış yan etkiden önce kalıcılaşması ve outcome garantisi korunmalı.
2. **Kapasite reddini sınıflandırmak:** yerel global/principal/vMCP limitleri ile delivery DB limiti, DB deadline ve SQL hatası için ayrı sayaçlar. Mevcut genel 503 mesajı kesin alt nedeni gizliyor.
3. **Tek-instance darboğazını kontrollü deneyle ayırmak:** önce yalnız pool büyüklüğü, sonra yalnız bounded audit flush paralelliği değiştirilerek aynı 6.000 tool workload'u tekrarlanmalı. Sadece pool veya admission limitini artırmak, kalıcı yazma kapasitesi artmadan daha büyük kuyruk oluşturabilir.
4. **Paylaşılan PostgreSQL kapasitesini ayrıca boyutlandırmak:** WAL/fsync gecikmesi, checkpoint/full-page-image hacmi ve audit/delivery transaction sayısı. fsync veya strict audit'i kapatıp elde edilen hız aynı hizmet garantisi değildir.

Bu koşular 10 dakikalık doğrulamalardır; uzun dönem sızıntı, üretim upstream dağılımı, JWT/IdP yükü, payload capture veya tüm retention döngüleri için kapasite garantisi vermez. Tarama esnasında düşen talep load-generator worker bütçesini de yansıtır; başarısız yüksek hedefler ham data-plane maksimum TPS'si olarak okunmamalı.

## Tekrar çalıştırma

Aşağıdaki komut tek instance / 1.200 TPS uzun doğrulamasını tekrarlar; yeni output dizini kullanılmalıdır:

```sh
VMCP_ACCEPTANCE_SERVERS=300 VMCP_ACCEPTANCE_TOOLS=20 \
VMCP_ACCEPTANCE_NODES=1 VMCP_ACCEPTANCE_ROUTING=pod \
VMCP_ACCEPTANCE_RPS=1200 VMCP_ACCEPTANCE_STEADY_SECONDS=600 \
VMCP_ACCEPTANCE_WORKERS=1024 VMCP_ACCEPTANCE_OWNERSHIP=1 \
VMCP_ACCEPTANCE_DIAGNOSTICS=1 \
python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8 \
  --postgres-tuned --output artifacts/benchmark-runs/capacity-6000-single-repeat
```

İki-instance doğrulaması için `VMCP_ACCEPTANCE_NODES=2`, `VMCP_ACCEPTANCE_RPS=2000` ve farklı output dizini kullanılır. Kapasite taraması için `VMCP_ACCEPTANCE_RPS` yerine `VMCP_ACCEPTANCE_CAPACITY_RATES=500,1000,1500,2000,3000,4000,6000,8000` ve `VMCP_ACCEPTANCE_CAPACITY_SECONDS=60` eklenir. İlk başarısız basamaktan sonra üst basamaklara çıkılmaz; bu koşularda 4.000/6.000/8.000 TPS çalıştırılmadı.

## Çalışma ve temizlik doğrulaması

Kapasite arama birim testi, 11 benchmark Python testi, kapasite raporu üretim kontrolü ve `git diff --check` geçti. Dört koşunun migration/delivery integration testleri kendi izole veritabanlarında çalıştı. Başarısız tarama koşularında genel acceptance'in tek başarısız kapısı uzun yük SLO'suydu; başarılı 1.200/2.000 TPS koşularında bütün acceptance kapıları geçti. Geçici `vmcp-load-*` namespace'leri kaldırıldı; her koşuda mevcut `vmcp-demo` aktif podlarının UID/restart sayıları korundu. Ürün runtime davranışı bu kapasite çalışmasında değiştirilmedi.
