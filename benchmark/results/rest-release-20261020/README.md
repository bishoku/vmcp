# REST release öncesi doğrulama — 20 Ekim 2026

Yerel tam REST kabul profili ve regresyon kontrolleri geçti. Yayın kararı için release commit’inin GitHub Actions kapıları da yeşil olmalı; bu çalışma bir image/tag yayınlamadı.

## Düzeltmeler

- REST istek gövdesi tekrar oynatılamayan bir akış olarak taşınıyor. HTTP bağlantı havuzları korunuyor; yanıt kaybında GET veya idempotency header’lı yazma isteklerinin otomatik tekrar yürütülmesi engelleniyor. HTTP/1.1 ve HTTP/2, beş metot ve üç header seçeneğiyle 30 regresyon senaryosu var. Eski kod aynı testte POST’u iki kez çalıştırdı; düzeltilmiş kod tek kez çalıştırıyor.
- Büyük REST kataloglarında snapshot yenilemesi her şemayı yeniden derliyordu. 60.000 tool ile mevcut 5 saniyelik yenileme sınırı aşılıyor ve yetkilendirme tazeliği dolunca node güvenli biçimde HTTP 503 veriyordu. Değişmeyen kaynak tanımı, URL ve header’lar için önceki snapshot’ın salt okunur şemaları yeniden kullanılıyor. Şifre çözme, revizyon, iptal ve 30 saniyelik yetkilendirme tazeliği kontrolleri gevşetilmedi. Global, sınırsız bir şema önbelleği eklenmedi.
- Kaynak/URL/header/auth değişikliği, kapatma, eski snapshot’ın değişmezliği ve bozuk şifreli veri için regresyon testleri eklendi. Migration 28’in mevcut MCP kaydını koruması ve REST kullanılmadan rollback/upgrade senaryosu doğrulandı.
- Ortak kabul testi altyapısına REST profili ve trafik altında snapshot yayılım kontrolü eklendi. Başlatıcının macOS Bash boş dizi uyumu düzeltildi.

## Ölçek ve sonuçlar

300 REST upstream, 300 virtual server, 60.000 tool, 600 kalıcı oturum ve 64 agent key. İki gerçek data-plane süreci; her node GOMAXPROCS=2 ve GOMEMLIMIT=768MiB. PostgreSQL 2 CPU/2GiB; Redis ayrı. Sentetik REST servis yanıtı 5 ms.

| Aşama | Hedef/s | Başarılı/s | Başarı % | p95 ms | p99 ms | Hata | Gönderilemeyen |
|---|---:|---:|---:|---:|---:|---:|---:|
| warmup | 50 | 49.9 | 100.000 | 52.1 | 63.3 | 0 | 0 |
| steady | 200 | 199.9 | 100.000 | 45.6 | 51.3 | 0 | 0 |
| spike | 600 | 584.7 | 100.000 | 193.1 | 356.8 | 0 | 285 |
| recovery | 200 | 199.8 | 100.000 | 45.8 | 51.0 | 0 | 0 |
| config-refresh | 200 | 199.6 | 99.950 | 78.2 | 211.1 | 3 | 0 |
| node-loss | 200 | 199.8 | 99.944 | 47.9 | 53.6 | 10 | 0 |
| node-rejoin | 200 | 200.0 | 100.000 | 46.8 | 52.4 | 0 | 0 |
| soak | 100 | 100.0 | 100.000 | 48.4 | 55.6 | 0 | 0 |

Bütün aşamalar koşudan önce tanımlanan kabul eşiklerini karşıladı. Yüzdelikler başarılı çağrılara aittir; hata ve gönderilemeyen istekler ayrıca gösterilmiştir. Ani yükte gönderilen çağrıların %100’ü başarılı olsa da 285 planlı istek gönderilemedi; “tüm planlanan istekler başarılı” sonucu çıkarılmamalı.

- Snapshot’ın iki node’a yayılması: **2346 ms**. İlk koşuda 25 saniyelik gözlem sınırında yayılamadı ve ardından tazelik süresi doldu.
- Başarılı 105.702 çağrının tamamında kalıcı admission ve başarılı outcome eşleşti. 300 upstream ve 300 virtual server çağrı aldı; 57.424 farklı virtual tool çalıştırıldı.
- Yinelenen upstream yürütmesi: **0**. Toplam 13 başarısız çağrı: yapılandırma güncellemesinde 3, node kaybı aşamasında 10.
- Node kaybında **1** işlemin admission kaydı olup outcome’u yok. Hatalı istemci çağrısıyla eşleşiyor; sonucu belirsiz, başarılı sayılmadı ve tekrar yürütülmedi. Gerçek yan etkili servislerde bu durum uygulama düzeyinde uzlaştırılmalı.
- Geçersiz key, key iptalinin yayılımı, ortak oturumun iki node’da sonlandırılması, katalog sayfalama ve PII kontrolleri geçti. İlk başarısız rapordaki “accepted/not rejected” metinleri beklenen durum kodunun alınmadığını ifade ediyor; gözlenen toplu yanıt HTTP 503 idi, yetkisiz çağrı kabul edildiğine dair bulgu değildir.

- acceptance-node-1: tepe RSS **1205.4 MiB**, medyan OS CPU **%168.4**, soak bellek artışı **%0.012**, gözlenen DB pool tepe kullanımı **13/64**, pool edinim iptali **0**.
- acceptance-node-2: tepe RSS **1214.7 MiB**, medyan OS CPU **%168.2**, soak bellek artışı **%0.003**, gözlenen DB pool tepe kullanımı **26/64**, pool edinim iptali **0**.

768MiB bir Go yumuşak bellek hedefidir, RSS sınırı değildir. Bu envanter için ölçülen yaklaşık 1,2GiB/node RSS üretim bellek planında payla birlikte dikkate alınmalı. CPU kullanımı iki çekirdeğe yaklaşabiliyor; bu sonuç sınırsız kapasite payı iddiası değildir. Beş dakikalık soak uzun dönem bellek sızıntısı yokluğunu kanıtlamaz.

## Release kontrolleri

- 500 Go testi, sıfır skip, 42 paket; race, go vet ve tüm cmd derlemeleri geçti.
- 179 UI testi, 120 tarayıcı testi, lint ve production build geçti. Mevcut büyük bundle uyarısı sürüyor.
- OpenAPI: 56 operasyon ve 6 yakalanmış örnek doğrulandı.
- Güncel govulncheck: açık bulunmadı. CI kapsamıyla aynı data-plane image HIGH/CRITICAL taraması: 0 bulgu.
- Data-plane, control-plane, migrate ve web image’ları yerel olarak üretildi.
- 10 Helm şablon testi, 9 kurulum testi ve 8 release yardımcı testi geçti. Broker bağımlılık izolasyonu korundu.
- Mevcut MCP ve REST kabul smoke senaryoları race ile geçti.

## Sınırlar ve yayın sırası

- Yerel tek fiziksel makinede iki node ve tek PostgreSQL kullanıldı. Üretim ingress’i, Kubernetes canlı upgrade/smoke, PostgreSQL HA, gerçek REST/OAuth servis yükü ve rate limiter kapasitesi ölçülmedi.
- Envanter toplu SQL seed ile hazırlandı. UI/import kapasitesi, toplu REST kaydetme transaction maliyeti ve bütün REST tanımlarının aynı anda değiştirilmesi ölçülmedi. Bu testte trafik altında değiştirilen veri bir virtual server’ın metadata’sıdır; REST şemalarının değişmediği snapshot yenileme yolu ölçülür.
- Yerel image güvenlik taraması data-plane kapsamındadır. Diğer image taramaları ve yayınlanmış artifact doğrulaması bu koşuya dahil değildir.
- Önce migration 28, ardından tüm control/data plane node’larını güncelleyin; REST upstream’leri bundan sonra etkinleştirin. Migration rollback’i REST konfigürasyon kolonlarını siler; REST kullanıldıktan sonra veri kaybı olmadan rollback varsayılmamalı.
- Release commit’ine yeni dosyalar ve migration dahil edilmeli. Tag öncesi aynı commit üzerinde tüm GitHub Actions kapıları, özellikle canlı deployment smoke, başarılı olmalı.

## Kanıtlar

- [Düzeltme sonrası ayrıntılı rapor](after-fix/report.md) ve [ham ölçümler](after-fix/report.json).
- [İlk başarısız koşu](before-fix/report.md) ve [ham ölçümler](before-fix/report.json).
- [Kontrol özeti](checks.json). Binary SHA-256 ve kaynak HEAD ham raporlarda yer alır; ölçümler çalışma ağacındaki değişikliklerle derlenen binary’lere aittir.
- Tekrar çalıştırma: `VMCP_ACCEPTANCE_REST=1 ./scripts/acceptance-core.sh`.
