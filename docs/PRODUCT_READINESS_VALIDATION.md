# vMCP ürün hazırlık raporu: doğrulama ve uygulanabilir iyileştirme planı

**Tarih:** 8 Kasım 2026  
**Takip:** [#53](https://github.com/bishoku/vmcp/issues/53)  
**İncelenen kaynak:** Yerel `docs/PRODUCT_READINESS_REPORT.md`, sürüm 1.0.0; SHA-256 `b21f42ca5d1980a5435b9e26d5fece364d9f7dd179b7144a3ef7f48ed509d339`. Kaynak dosya kullanıcıya ait, Git tarafından izlenmeyen girdidir; değiştirilmedi ve bu raporun commitine alınmadı.  
**Kod tabanı:** İnceleme başlangıcı `3aa02a119f947985b3b55a852e7fe0d5ae500d92`; rapor dalının main tabanı `94bcccf93a80a193a066137f4153bb6e290d31a2`. İki commit arasındaki altı dosyalık fark sürüm metadata/API sürüm alanları ve release belgeleridir; burada değerlendirilen çalışma zamanı kodu aynıdır.  
**Kapsam dışı:** SAML 2.0 ve SCIM 2.0; bu özellikler puan, öncelik veya üretime geçiş koşulu olarak kullanılmadı.

## 1. Yönetici değerlendirmesi

Kaynak rapor faydalı bir risk envanteri içeriyor; ancak mevcut haliyle bütçe, takvim veya üretime çıkış engellerini belirlemek için yeterince güvenilir değil. Gerçek eksikler, tasarım tercihleri, önceki ölçümler ve teorik kapasite hesapları aynı kesinlikte sunulmuş. 74/72/78/68 gibi puanların ağırlıkları, ölçüm yöntemi ve kabul sınırları yok; bu puanları yeniden kullanmak doğru olmaz.

**En gerçekçi öncelik:** Half-open probe yaşam döngüsünü düzeltmek; araç annotation güvenini ve tekrar politikasını operatör kontrolüne almak; MCP argüman şeması doğrulaması eklemek; legacy ciphertext geçişini kapatmak; web/Omni ağ korumalarını güçlendirmek. Bunlarla birlikte hedef ortamda veritabanı failover/restore ve mevcut sürümün kapasite kabulü yapılmalı.

**Doğrudan uygulamaya alınmaması gereken öneriler:** Tüm goroutine panic'lerini yutup devam etmek, advisory kilidini yalnızca try-lock ile değiştirerek kapasite sorununun çözüldüğünü ilan etmek, heartbeat'i iptal/fencing semantiğinden bağımsız 5 saniyeye çıkarmak, oturum/iptal denetimine koşulsuz TTL cache koymak, SSE'yi denetim ve maskeleme sınırlarını atlayan zero-copy geçişe çevirmek.

Bu incelemede uzaktan tetiklenebilir bir süreç çökmesi, SQL injection istismarı veya genel veri kaybı gösterilmedi. Bu nedenle kaynak rapordaki bütün P0 etiketleri korunmuyor. Hassas yazma araçları ve güvenilmeyen upstream'ler söz konusuysa annotation güveni ve doğrulama açıkları ilgili kullanımın yayın kapısı olmalıdır. Ürün genelinde üretim hazır/uygunsuz kararı yerine, açık workload ve altyapı koşullarıyla kabul kararı verilmelidir.

## 2. Yöntem ve kanıt sınırı

- Çağrı yolları, ilgili testler, migration/Helm/nginx dosyaları ve operasyon belgeleri okundu. Özellikle `virtual`, `delivery`, `resilience`, `policy`, `secrets`, `omni`, `audit`, `telemetry`, `store`, `restapi` incelendi.
- Beş Go paketinin mevcut testleri, mevcut audit masking benchmarkı ve web üretim build'i çalıştırıldı. Half-open durum geçişi, aynı `breaker.go` dosyasının geçici kopyasında deterministik bir deneyle tekrarlandı.
- Kubernetes yük testi, HA failover, gerçek IdP/SIEM veya saldırı simülasyonu yapılmadı. Mevcut benchmark belgeleri tarihsel kanıt olarak kullanıldı; bu incelemenin yeni ölçümü olarak sunulmadı.
- Kaynağın atıf verdiği `CODE_REVIEW_REPORT.md` çalışma ağacında bulunamadı. Bu belgeye dayanan 454 ns, 16 ms, 35 ms, 20x gibi rakamlar bağımsız doğrulanmış kabul edilmedi.
- Kaynak metodoloji SEC-01…11, ASSUMP-01…05, PERF-01…10 bütününün doğrulandığını söylüyor. Ancak SEC-06/07/10 ve ASSUMP-02 için somut bulgu metni yok. Eksik bulgular türetilmedi; kaynakta açıklanan bütün kapsam içi başlıklar aşağıda değerlendirildi.
- “Doğrulandı” kod mekanizmasının veya belirtilen yerel deneyin doğrulandığını ifade eder; otomatik olarak istismar, üretim SLA'sı veya bağımsız sertifikasyon anlamına gelmez.

## 3. Bulguların doğrulama matrisi

### 3.1 Dayanıklılık ve performans

| Kaynak iddiası | Karar ve kanıt | Gerçekçi işlem / öncelik |
|---|---|---|
| `recover()` yok, dolayısıyla P0 | Uygulama Go dosyalarında recovery bulunmadı. HTTP handler ile bağımsız goroutine farklıdır; tüm istek panic'leri süreci düşürmez. Somut uzaktan tetikleme gösterilmedi. `virtual/streams.go:82` bağımsız işçiye örnektir. | P1: kritik işçi gözetimi, hata enjeksiyonu ve kontrollü kapanma. Her yere recover eklemek değil. |
| ASSUMP-05: probe iptalinden sonra 30 s ret | `resilience/breaker.go:38` ve `virtual/handler.go:713` doğruluyor. Half-open `Allow()` sonrası `Ignore` dönüşü probe'u serbest bırakmıyor. | P1: sahipliği belirli probe tamamlanması/abort. Hassas erişilebilirlik SLO'sunda yayın kapısı. |
| PERF-02: sanal sunucu başına 62,5 çağrı/s kesin tavan | `delivery/store.go:44` kabul transaction'ını sunucu anahtarında serileştiriyor. **16 ms kilit tutma ölçümü yok**; 62,5 yalnızca bu varsayım altında hesap. | P1 ölçüm; P2 tasarım. Lock wait ile lock hold ayrı ölçülmeli. |
| PERF-03: 1.000 işlem daima 2.000 SQL/s | 1 s Renew ve hata halinde cancel doğru (`virtual/streams.go:90`). Roots sorgusu koşullu (`virtual/roots.go:9`): özellik, negotiated capability ve açık upstream bağlantısı gerekli. | P1: uzun operasyon profili ve lease/iptal tasarımı. Evrensel 2.000 QPS değil. |
| PERF-01: tek mutex ana kapasite darboğazı | `Handler.acquire` tek mutex ve 1024/128/16 sınırlarını içeriyor; dağıtık kabulde de aynı limitler var. 454 ns ölçümü bu incelemede doğrulanmadı. | P2: mutex/block profili; önce limit sözleşmesi. 64 stripe varsayılan çözüm değil. |
| PERF-10: oturum ve JWT iptali DB sıcak yolu | `virtual/session.go:317`, `agentauth/verifier.go` mekanizmayı doğruluyor. Bu, authoritative iptal kontrolünün bilinçli maliyeti. 2–6 ms evrensel gecikme değil. | P2: sorgu/indeks ölçümü; cache ancak açık revocation SLA'sı ile. |
| PERF-07: PII 20x heap | JSON ağaç dönüşümü/regex maliyeti gerçek. Mevcut benchmarkta büyük yüklerde yaklaşık 5,3–5,8x **toplam tahsis** ölçüldü. Canlı heap/RSS farklıdır. | P2: workload tabanlı alloc/live-heap/CPU profili. 20x ve %80 kazanç taahhüdü kaldırılmalı. |
| PERF-09: her SSE 10 MiB ayırıyor | Yanlış: `upstream/client.go:251` ve `sse.go:206` başlangıç buffer'ı 64 KiB, maksimum event/token sınırı 10 MiB. Büyük içerikte büyüme ve ek kopyalar yine mümkün. | P2: maksimum payload ve eşzamanlılık testi. Körlemesine 2 MiB'ye düşürme yok. |
| PERF-04: disconnect sonrası 70 s çalışma kaçak | `context.WithoutCancel` ve 70 s üst sınır doğru; kalıcı teslimat/replay sözleşmesi için bilinçli. Her istek 70 s yaşamıyor. | Tasarımı koru; explicit cancellation, süre sonu, audit ve drain testleri. |
| PERF-06: JSON-RPC birden çok parse | `mcp/jsonrpc.go` çerçeve/kimlik/alan kontrolleri için birden fazla geçiş içeriyor. CPU darboğazı olduğuna dair profil yok. | P3: fuzz eşdeğerliğiyle optimize et; strict framing'i zayıflatma. |
| PERF-05: dinamik SQL plan çeşitliliği | Metin birleştirme gerçek; performans etkisi grup çeşitliliğine ve sürücü statement cache davranışına bağlı. | P2: ortak query builder ve parametreli issuer/grup dizisi. |
| PERF-08: 966 KB monolitik JS | Eager sayfa importları var. Yeni build: ana JS 1.016,56 kB, gzip 279,79 kB; ayrıca ayrı chunk'lar var. Eski boyut ve “tek paket” kesinliği güncel değil. | P2/P3: route lazy-loading; auth geçişi ve deep-link regresyonu. |
| Omni araması eksik / 50 bin üzeri çalışmaz | Arama var; `omni/index.go:223` filtreli lineer tarama ve bounded top-K kullanıyor. Kesin 50 bin eşiği kanıtsız. | P2 koşullu: 10k/50k/100k, boyut ve yetki yoğunluğu ile ölç; gerekirse ANN. |
| ASSUMP-03: her node değişiminde 15 s kilit | `delivery.Store.Claim` lease koyuyor; `serveStream` normal çıkışta `Release` çağırıyor. Crash/partition/cleanup hatasında artık lease beklenebilir. | P2: güvenli stream takeover; kimlik doğrulama tek başına fencing yerine geçmez. |
| PostgreSQL SPOF çünkü uygulama failover yapmıyor | DB zorunlu bağımlılık; fakat HA, dış primary endpoint/operator katmanında tasarlanmış (`PRODUCTION_DEPLOYMENT.md`). Read replica HA'nın eşanlamlısı değil. | P1 operasyonel kabul: gerçek failover, PITR, fencing, RPO/RTO. |
| ASSUMP-04: çok kiracılık yok | Paylaşımlı şema yok; **ayrı deployment/DB ile çok kiracılık var** (`TENANT_OPERATIONS.md`, `store/snapshot.go:109`). | Hata listesine alınmamalı. Shared-schema SaaS istenirse ayrı ürün programı. |
| REST health check eksik | `health/health.go:58` REST'i atlıyor. Rastgele REST çağrısı sağlık kontrolü olamaz. | P2: opsiyonel, yan etkisiz operatör probe'u; trafik tabanlı sağlıkla birlikte. |
| Uyarlamalı load shedding yok | Sabit kabul limitleri, timeout ve circuit breaker mevcut; uyarlamalı kuyruk yönetimi yok. | P2 koşullu: adalet ve kontrollü overload ölçüldükten sonra. |

### 3.2 Güvenlik, kimlik ve gözlemlenebilirlik

| Kaynak iddiası | Karar ve kanıt | Gerçekçi işlem / öncelik |
|---|---|---|
| SEC-01 rota yetki yükseltmesi | Güncel `configuration_authorization.go` granüler denetimi kullanıyor; eski route yükseltmesi mevcut açık olarak gösterilmemeli. | Mevcut deny/field-scope/revocation testlerini koru. |
| SEC-04 / ASSUMP-01 annotation güveni | `policy/policy.go:90` read-only filtreyi upstream hint'inden çıkarıyor. `callResilient` hint ile tek retry seçiyor. Approval/caller-context işaretli çağrılarda retry kapalı; bütün çağrılar körlemesine tekrar edilmiyor. | P1; güvenilmeyen/yazma upstream'lerinde yayın kapısı. Operatör izin listesi, explicit retry policy. |
| MCP InputSchema doğrulaması yok | `virtual/handler.go` JSON/metadata/PII/policy kontrolü yapıyor ama genel MCP argüman şemasını çalıştırmıyor. REST doğrulaması ve Omni schema-version kontrolü bunun yerine geçmez. | P1: ortak virtual/Omni çağrı yolunda bounded schema compilation/validation. |
| SEC-08 SQL injection | `ownership.go:29` tek tırnakları ikiye katlıyor. Salt konkatenasyon bulgusu, çalışır injection kanıtı değildir. `vsPredicate` yeni policy yolu da içeriyor. | P2 güçlendirme: parametreleştir; çağrı yollarını ve PostgreSQL string ayarlarını test et. P0 istismar iddiası doğrulanmadı. |
| SEC-05 REST matrix injection | `unsafePathValue` `;` reddetmiyor fakat çağrı yolu `url.PathEscape` uyguluyor (`execute.go:82`). Downstream decoding/route semantiği olmadan istismar doğrulanamaz. | P2 koşullu: encoded delimiter ve çift decode testleri; desteklenen path sözleşmesini tanımla. |
| SEC-02 Omni egress koruması | Embedder/reranker ham transport kullanıyor; yönlendirme reddediliyor ama netpolicy uygulanmıyor. Default transport klonu proxy ayarlarını da devralabilir. | P1: allowlist/CIDR/DNS/metadata/proxy politikası; özel CA desteği. Özel VPC, TLS doğrulamasını kapatmayı gerektirmez. |
| SEC-03 legacy ciphertext | AAD'siz eski format okunabiliyor. Üstelik `FromEnv` yalnızca açık `false` ile kapatıyor; boş env de izin veriyor. Yeni v1 ciphertext'in AAD denetimi kapatılmıyor. | P1: eski veri envanteri, kontrollü rotasyon, ardından `false`; geri dönüş anahtarlarını güvenli tut. |
| KMS/HSM/Vault yok | Uygulama yerel keyring kullanıyor; cloud wrapping/transit arayüzü yok. Helm existing Secret destekliyor: secret dağıtımı ile KMS kripto entegrasyonu ayrı konular. | P2, müşteri şartı varsa P1: tek sağlayıcıyla aşamalı KeyProvider tasarımı. |
| SEC-09 web güvenlik başlıkları | `web/nginx.conf` CSP/frame/nosniff içermiyor; chart'ın `templates/web-config.yaml` kopyası da ele alınmalı. Ingress ekliyor olabilir; bu incelemede canlı yanıt ölçülmedi. | P1 düşük maliyet: iki konfigürasyonu birlikte düzelt, gerçek browser/HTTP testleri. |
| Multi-IdP yok, B2B imkânsız | Console başına tek OIDC issuer doğru; agent OIDC ayrı ayarlanıyor. Identity broker veya tenant başına issuer ile B2B mümkün. | P2 koşullu: ihtiyaç varsa issuer registry, issuer+subject kimliği, session/JWKS izolasyonu. |
| Gelen mTLS / RFC 8705 yok | Native agent sertifika kimliği/token binding görülmedi. Edge mTLS yapılabilir; bu uygulamanın RFC 8705 doğruladığı anlamına gelmez. | P2 koşullu: önce ingress güven sınırı, sonra gerekiyorsa certificate-bound token doğrulaması. |
| Yerel TLS yok | Ana servislerde edge termination tasarımı var; tenant-router HTTPS dinleyicisi mevcut. Bütün Go servisleri TLS'siz genellemesi yanlış. | Güvenilir ingress, private plane, ağ izolasyonu; zorunlu uçtan uca TLS varsa ayrı karar. |
| ABAC/OPA eksik | Harici genel PDP yok; mevcut scoped RBAC, claim mapping, invocation ve IP denetimleri var. | P3 müşteri ihtiyacı: önce mevcut kurallarla boşluğu göster; sonra cache/revision/fail-closed sözleşmesi. |
| FPE/tokenization eksik | Redaction geri çevrilebilir tokenization değil; bu tek başına açık değil. Argument masking şema/iş anlamını değiştirebilir; audit-only masking ayrı değerlendirilir. | P3 koşullu: önce alan bazlı redaction ve maskeleme sonrası şema doğrulaması. |
| Log seviyesi/bağlam eksik | Main'lerde `slog.NewJSONHandler(..., nil)` var; merkezi log-level env ve otomatik context log enrichment yok. Audit trace ID mevcut. | P1/P2: startup level, request/trace bağlamı, güvenli hata sınıfları; canlı seviye değişimi ikinci adım. |
| Ham hata sızıntısı | `apierror.Write` bilinmeyen hatayı sunucu loguna basıyor; istemciye genel 500 gönderiyor. İstemciye ham hata sızdırdığı söylenemez. | P1: log redaction, URL/DSN/header içeren sentetik hata testleri. |
| SEC-11 hash-chain/WORM yok | DB içinde hash chain yok. Ancak restricted DB roller ve harici WORM/anchor yönergeleri zaten var (`AUDIT_OPERATIONS.md`, `audit-roles.sql`). | P1 operasyonel: bağımsız immutable sink ve doğrulama. Hash-chain ihtiyaç varsa P2; tek başına DBA tehdidini çözmez. |
| Syslog/Splunk HEC yok | Native yok; HTTPS/Kafka/RabbitMQ var. Genel webhook HEC şeması/ack davranışına otomatik eşit değil. | P2 müşteri talebi: mevcut sink adapter/collector, gerekirse native exporter. |
| Exporter metrik yok, lag izlenemez | `/metrics` yok; `auditexport/run.go` dakika bazında durum sayıları logluyor. “Hiç izlenemez” yanlış. | P1/P2: oldest-pending age, backlog/dead/retry/latency ve alarm. |
| Go runtime metrikleri yok | Özel registry standart `go_*` / `process_*` serilerini yayımlamıyor. Uygulamaya özgü gauge'lar var. | P2: bounded runtime/GC/process ve pool metrikleri. |
| Trace propagation tamamen yok | HTTP root span ve Omni→backend trace header iletimi var; genel upstream/DB child-span instrumentation eksik. | P2: pgx/upstream span ve outgoing propagation; payload/token/span cardinality sınırları. |
| Prompt injection denetimi yok | Semantik prompt savunması gateway'in genel JSON/TLS yeteneğinden çıkarılamaz; kaynak zaten kapsam dışı diyor. | Ayrı bir “garanti” vaat etme. Tool trust/izin/onay sınırını güçlendir. |

## 4. Kritik düzeltmelerin tasarımı ve kabul kriterleri

### 4.1 Half-open probe yaşam döngüsü

**Sorun:** `Allow()` probe'u başlatıyor. `Ignore`, authorization değişimi veya audit hatası gibi bazı erken dönüşler `Success/Failure` çalıştırmıyor. Bu nedenle probe bayrağı cooldown sonuna kadar kalabiliyor. Arka plan health checker'ın `Success()` çağrısı süreyi kısaltabilir. Kalıcı 30 s genel kesinti iddiası bu yüzden de fazla kesin. Stream HTTP bağlantısının kopması `WithoutCancel` nedeniyle her zaman upstream context'ini iptal etmez; iptal yolu explicit cancellation/deadline veya başka çağrı yolundan gelebilir.

**Öneri:** Probe acquisition sonucu bir token/generation döndürsün; completion/abort yalnızca kendi probe'unu bitirsin. En azından `AbortProbe` yarım açık durumla sınırlanmalı; eskimiş bir çağrının yeni probe'u kapatması engellenmeli. Breaker kapalıyken olan kullanıcı hatası sağlıklı upstream'i başarısız saymamalı. Tüm erken dönüşler gözden geçirilmeli.

**Kabul:** Fake clock ile open→half-open→ignore→yeni probe; authorization/audit/token error; 100 eşzamanlı Allow; gecikmiş eski probe sonucu; gerçek timeout ve upstream başarı ayrımı. Yarış testi geçmeli. Kullanıcı iptali breaker'ı gereksiz süre kilitlememeli, aynı anda birden fazla probe açılmamalı.

### 4.2 Araç şeması ve güven sözleşmesi

**Öneri:** Snapshot hazırlamada InputSchema derle; cache anahtarı upstream/tool/schema digest veya revision olsun. Harici `$ref` ağ erişimini kapat veya onaylı resolver ile sınırla; schema boyutu/derinliği, derleme zamanı ve cache cardinality sınırları tanımla. Desteklenmeyen/bozuk şema için yayımlamama veya açık policy kararı üret. `jsonschema/v6` zaten bağımlılık; yeni kütüphane eklemek tek başına çözüm değil.

MCP/Omni yolları aynı validation noktasına ulaşmalı. İlk argüman doğrulamasına ek olarak, PII argümanları değiştiriyorsa gönderilecek son değer doğrulanmalı. Onay hash'i ve upstream'e giden içerik aynı olmalı; invalid çağrı onay consume veya upstream yan etkisi üretmemeli. Hata mesajları hassas değeri geri basmamalı. Protokol framing hatası ile araç argüman/işletim hatasının JSON-RPC/tool-error gösterimi mevcut sözleşmeyle tutarlı seçilmeli.

`readOnlyHint` ve `idempotentHint` güvenlik otoritesi yapılmamalı. Operatör `trustedAnnotations`, `retryMode`, `maxAttempts` ve gerekiyorsa upstream/tool override yönetebilmeli. Varsayılan otomatik replay kapalı olmalı veya sadece açıkça güvenilen araçlara açılmalı. “Readonly” etiketi kötü niyetli bir sunucunun gerçekten yazmasını teknik olarak önleyemez; upstream credential kapsamı ve araç izin listesiyle desteklenmeli.

**Kabul:** Eksik/yanlış tür, sınır aşımı, oneOf/anyOf, nullable değerler, ek alanlar, schema revision değişimi, kötü niyetli `$ref`, maskelenmiş argümanlar, virtual/Omni eşdeğerliği. Ağ hatası sonrası yazma aracında ikinci execution olmamalı. Onaylı çağrıda otomatik retry yasağı korunmalı. İdempotency key ancak upstream uyguluyorsa güvence sayılmalı.

Tek retry varken üstel backoff yokluğu tek başına mimari kusur değildir; birden çok deneme eklenirse toplam deadline, jitter, Retry-After, retry bütçesi ve upstream rate-limit birlikte tasarlanmalı.

### 4.3 Lease, heartbeat ve kabul darboğazı

**Ölçüm:** Kısa REST fixture ile uzun MCP/SSE operasyonu ayrılmalı. 1.000 aktif uzun işlem yaklaşık 1.000 Renew/s üretebilir; roots koşulları sağlanırsa ilave yaklaşık 1.000 sorgu/s eklenebilir. Gerçek query service time 35 ms ise 2.000 × 0,035 = 70 bağlantı talebi bir senaryodur; RTT veya pool beklemesi doğrudan bu süre yerine konmamalı.

**Öneri:** Renewal/iptal/roots bildirimini ayrı gereksinimler olarak tasarla. Session başına roots revision kontrolünü birleştir; heartbeat'leri jitter/batch ile dağıt; aktif işlem sayısı, pool acquire, renew failure ve remaining lease ölç. Geçici hata retry'si lease hâlâ geçerliyken sınırlı olabilir; sahiplik/fence kaybında durulmalı. DB erişimi yokken sonsuz devam, aynı işin iki node'da çalışmasına yol açabilir. Ticker'ın 5 s olması aynı sorgudan alınan cancel bilgisini de geciktirir; 15 s lease için “10 s güvenli pay” GC/ağ/pool gecikmesini kanıtlamaz.

Kilit için önce tek sıcak sanal sunucu ile 300 sunucu dağılımını karşılaştır. `pg_try_advisory_xact_lock` kuyruk birikimini azaltabilir ama kapasiteyi artırmaz; mevcut kabul edilebilir burst'leri 503'e çevirebilir. Kısa transaction, bounded queue, atomik kota sayaçları/slot rezervasyonu seçeneklerini ölç. Global/principal kotalarının farklı sunucu kilitlerinde eşzamanlı uygulanması ayrıca test edilmeli; aynı limitler hem local hem DB katmanında var.

**Kabul:** 1/2/4 replica, tek-hot-server ve dağınık yük; 50 ms/1 s/30–60 s operasyonlar; Redis/DB gecikmesi; pool baskısı; process kill ve partition. Hedef SLO altında throughput, hata, p95/p99, cancel gecikmesi, double execution, stale writer ve eksik audit birlikte raporlanmalı. Güvenlik garantisi pahasına QPS kazancı kabul edilmemeli.

### 4.4 Panic davranışı

Her goroutine'e recover eklenmesi kritik bir worker'ın sessizce ölmesine veya bozulan state ile servisin ready kalmasına neden olabilir. Görev başına sınırlı recovery; payload içermeyen stack/error kaydı; pending işin belirsiz outcome olarak işaretlenmesi; worker sayaçları ve readiness ilişkisi tasarlanmalı. Snapshot/audit gibi güvenlik açısından kritik worker devam edemiyorsa readiness düşürüp kontrollü süreç yeniden başlatma tercih edilmeli.

HTTP recovery eklenecekse SSE başlamış yanıtı JSON 500'e çevirmemeli. Flush/Unwrap davranışı korunmalı. `recover` OOM, SIGKILL veya bütün runtime fatal hatalarına çözüm değildir; panic her ortamda SIGABRT anlamına gelmez. Standart HTTP sunucusunun handler panic davranışı için [Go net/http Handler belgesi](https://pkg.go.dev/net/http#Handler) esas alınmalıdır.

**Kabul:** Worker, audit, stream ve HTTP sınırlarında kontrollü panic injection; readiness, restart, goroutine sayısı, audit reconciliation ve SSE yeniden bağlantısı gözlenmeli. Test, yalnızca “process ayakta” kontrolü olmamalı.

### 4.5 SQL, Omni egress ve browser güvenliği

**SQL:** `ownerPredicate` SQL+args üreten bir builder'a dönüştürülebilir. `issuer=$n` ve `group_value=ANY($m::text[])`; kolon/alias yalnızca sabit koddan gelmeli. Mevcut placeholder sırası tüm çağrı yerlerinde korunmalı. Unicode, tek tırnak, ters eğik çizgi, boş/çok grup ve gerçek PostgreSQL parser testleri eklenmeli. Yeni configuration policy yolu ile eski Access yolunun farkı kaybolmamalı.

**Omni:** Endpoint değişikliğine yetkili kullanıcıları ve verinin hangi sağlayıcıya çıktığını belgelemek öncelikli. Netpolicy entegrasyonu private endpoint'leri açık izinle desteklemeli; metadata ve DNS rebinding koruması, redirect yasağı ve proxy politikası tutarlı olmalı. Özel CA için trust store sunulmalı. `InsecureSkipVerify` iç ağ gerekliliği gibi normalleştirilmemeli.

**Browser:** CSP'yi report-only ile keşfet, ardından uygun `default-src`, `script-src`, `connect-src`, `frame-ancestors` politikalarını uygula; `nosniff`, frame koruması ve TLS edge'de HSTS değerlendir. Docker nginx ve Helm nginx şablonu aynı davranmalı. Nginx location `add_header` kalıtımı nedeniyle runtime-config dahil bütün yollar test edilmeli. OIDC redirect/logout, API bağlantısı, fontlar ve SPA route refresh bozulmamalı.

### 4.6 Anahtarlar ve audit bütünlüğü

**Hemen yapılabilir:** Gerçek deployment'ta legacy veri sayımı, yedek/restore, reader-first rotation ve `VMCP_ALLOW_LEGACY_CIPHERTEXT=false` geçişi. Yeni format AAD cross-entity/cross-purpose/cross-tenant testleri korunmalı. Base64 bir şifreleme katmanı değildir; env yaklaşımı tek başına açık kanıtı da değildir. Secret erişimi ve process/crash-dump yetkileri önemlidir.

**KMS:** Mevcut format yerel keyring ve bağlama bağlı AEAD içeriyor; her kayıt için KMS ile sarılmış ayrı DEK varsayılmamalı. Önce bir backend ve envelope formatı seçilmeli; key ID/version, decrypt cache, outage davranışı, throughput/kota, multi-region restore ve maliyet kararı verilmeli. Dört sağlayıcıyı bir ayda tam desteklemek gerçekçi ilk hedef değil. Env yerine dosya/CSI secret sunmak ayrı, daha küçük iyileştirme olabilir.

**Audit:** DBA hash zincirini de yeniden yazabilir; bağımsız imza/anchor yoksa zincir yeterli değildir. Önce mevcut exporter ile bağımsız WORM/object-lock sink kurup kabul receipt'lerini, dedup ve retention/legal hold akışını doğrula. Hash zinciri istenirse tenant/partition sırası, concurrent insert, geç commit, silme/retention ve doğrulayıcı tasarlanmalı; yeni global serialization yaratılmamalı. Mevcut rol ayrımını yok sayarak bütün DELETE'leri kapatmak yasal retention işini bozabilir.

Hash-chain yokluğundan doğrudan SOC 2/HIPAA uygunsuzluğu sonucu çıkarılmadı; bu çalışma uyumluluk denetimi değildir. Teknik kanıt, organizasyonun kontrol hedefleriyle ayrıca eşlenmelidir.

## 5. Ölçümle önceliklendirilecek iyileştirmeler

### PII ve transport

Mevcut benchmark `audit.PreviewPayload` yolunu, iki custom rule ve büyük tek string içeren JSON ile ölçüyor; yalnız `pii.MaskJSON` veya 26 kural testi değil. `B/op` çağrı boyunca ayrılan toplam bayttır; eşzamanlı canlı heap değildir. 100 × B/op hesabı tek başına 100 isteğin anlık RSS'sini vermez.

Önce regex/policy compilation cache, tekrar JSON dönüşümü ve büyük geçici string kopyaları profillenmeli. Birleşik regex öncelik/checksum/overlap semantiğini değiştirebilir. “Rakam veya @ yoksa bütün kuralları atla” custom alphabetic PII kurallarını kaçırabilir. Optimizasyon eski/yeni redaction çıktısının eşdeğerliği ve false-negative testleriyle yapılmalı.

SSE sınırları event boyutu, JSON decode ve durable replay toplam bütçesiyle birlikte ele alınmalı. PII masking, invocation güvenliği ve kalıcı teslimat gerektiren içerik doğrudan zero-copy geçirilemez. 2 MiB'ye düşürmek ancak sözleşme ve büyük yanıt regresyonları kabul edildikten sonra düşünülebilir.

### Authorization cache ve read replica

Oturum/denylist kontrollerinin authoritative olması güvenlik özelliğidir (`session.go` bunu açıkça belgeliyor). TTL eklemek revocation penceresini büyütür. LISTEN/NOTIFY invalidation, revision kontrolü, reconnect/full resync ve DB outage fail-closed davranışı olmadan 3–5 s cache önerilmez. Asenkron replica'dan auth okumak da aynı riski taşır. İlk adaylar gecikmeye toleranslı raporlama/diagnostic sorguları olmalı; güvenlik kararları primary'de kalmalı.

### Omni ve web

ANN gerekiyorsa erişim filtresi aramadan önce veya aday seçimi sırasında uygulanmalı; yalnız top-K sonrası filtreleme hem recall kaybı hem görünürlük yan kanalı oluşturabilir. Index revision ve schema digest birlikte yenilenmeli. Önce mevcut `BenchmarkAuthorizedSearch` ile katalog boyutu, vektör boyutu, erişilebilir oran ve concurrent query matrisi çalıştırılmalı; bu incelemede ANN ihtiyacı kanıtlanmadı.

Web için audit, authorization, Omni ve invocation sayfaları lazy-load adaylarıdır. Amaç build uyarısını gizlemek değil, cold-load ağ/parse süresini azaltmaktır. Route fallback, oturum süresi dolması, yetki değişikliği ve chunk-load failure senaryoları korunmalı.

### Gözlemlenebilirlik

Önce karar aldıran sinyaller: pool acquire/lock wait, renew failure, breaker state/probe age, audit/export backlog ve oldest pending, DB WAL/disk, GC/live heap, dropped/denied iş sayısı. Loglarda trace/request/operation bağlantısı kurulmalı; session/subject gibi kişisel alanlar otomatik ve sınırsız eklenmemeli. Child span'lerde SQL parametreleri, argümanlar, tokenlar ve hassas URL'ler kaydedilmemeli. Collector kesintisi request yolunu bloke etmemeli.

Native Syslog/HEC, multi-IdP, mTLS, OPA ve tokenization yalnız somut müşteri kabul koşulu varsa ilk dalgaya girmeli. Bunların yokluğu bütün deployment'ları üretime uygunsuz yapmaz.

## 6. Performans ve güçlü yönlerin doğru okunması

[Issue 31 kabul belgesi](../maintainer/ISSUE31_ACCEPTANCE.md), 999,6 başarılı çağrı/s, %99,973 başarı, p95 168,8 ms ve p99 366,1 ms sonuçlarını gerçekten içeriyor. Ancak koşullar şunlar:

- 600 saniye, 300 sanal sunucu × 200 araç; 600 session, 1.024 worker.
- Tek data-plane replica, 8 CPU, 6 GiB pod limiti; PostgreSQL CPU/RAM limiti olmadan ayarlanmış.
- MCP tools/call trafiği **REST fixture upstream**'lerine gidiyor; strict **metadata** audit açık.
- 164 steady error ve 8 dropped demand var. Başarılı 600.078 çağrının audit eşleşmesi ve duplicate/incomplete olmaması bu koşulda doğrulanmış.
- Tam fault/soak, production ingress/IdP, payload capture, gerçek MCP uzun SSE, 2.000 peak veya güncel sürüm kapasitesi sertifikası değil.

“1.200 üstü CPU değil kesin DB kilidi yüzünden çöker” sonucu bu kanıttan çıkarılamaz. Eski gözlemlerde WAL write/sync beklemeleri var; blocked transaction/deadlock görülmediği de yazıyor. Batching iyileştirmesinin %37 sonucu bulk ile eski pipeline karşılaştırması; bütün disk I/O veya tekil event maliyetinde %37 genel kazanç değil.

Strict audit admission, transaction içi yönetim audit'i, PKCE/JWKS, AAD, netpolicy, PII checksum'ları, Redis bucket, readiness ve drain değerli mevcut yeteneklerdir. Bunları yeniden yazmak yerine sınırlarını korumak gerekir:

- `synchronous_commit=on` yerel dayanıklılık sağlar; failover'da sıfır kayıp garantisi replication/fsync/storage ayarlarına bağlıdır. [PostgreSQL WAL belgesi](https://www.postgresql.org/docs/current/runtime-config-wal.html) bu ayrımı açıklıyor.
- Redis local fallback, cluster çapında aynı kota garantisi değildir; strict mod ayrı test edilmelidir.
- 75 s drain her dış servis/DB arızasında bütün outcome'ların kesin kaydedildiği anlamına gelmez.
- “Stateless” podlar cache ve canlı upstream bağlantısı taşır; kalıcı durumun DB'de olması kesintisiz stream migration garantisi değildir.
- Cardinality limitleri yararlı fakat tüm gözlemlenebilirlik yüzeyleri için sınırsız DoS bağışıklığı iddiası değildir.
- #37 için v2.0.1 hedefi ve legacy global approval-hook kaldırılması korunmalıdır: `VMCP_APPROVAL_URL` kullanan deployment startup'ta başarısız olur; bu kırıcı değişiklik [v2.0.1 notlarında](../release-notes/v2.0.1.md) açıkça belgelenmiştir. Bu rapor sürüm veya release metadata değiştirmez.

[MCP 2025-06-18 araç belirtimi](https://modelcontextprotocol.io/specification/2025-06-18/server/tools), araç girdisi denetimini ve güvenilmeyen kaynak annotation'larına güvenilmemesini destekler. Şema kontrolü semantik prompt injection veya upstream'in gerçekten salt okunur davranacağı garantisi değildir.

## 7. Uygulanabilir iş paketleri

Tahminler deneyimli bir mühendis için **mühendis-gün aralığıdır**, teslim tarihi veya toplam proje taahhüdü değildir. Review, CI, altyapı erişimi ve canlı ortam kabulü ek süre gerektirebilir. Paketler ayrı issue/PR ile yürütülmeli; aşağıdaki tablo uygulama yetkisi veya otomatik merge talebi değildir.

| Paket | Kapsam ve çıktı | Öncelik | Yaklaşık efor | Bağımlılık / tamamlanma ölçütü |
|---|---|---|---|---|
| A | Probe completion/abort ve erken dönüşler | P1 | 2–4 gün | Deterministik ve race testleri; yeni probe eski sonuçtan etkilenmez. |
| B | Gateway schema validation + cache | P1 | 5–10 gün | Virtual/Omni/PII/onay sıralaması; invalid çağrıda sıfır dispatch. |
| C | Operatör kontrollü annotation/retry güveni | P1 | 5–10 gün | Backend policy, API/UI, migration/default ve replay regresyonları. |
| D | Nginx/chart security headers | P1 | 1–3 gün | OIDC/SPA browser ve gerçek response header kontrolleri. |
| E | Omni netpolicy/private CA/proxy sözleşmesi | P1 | 3–6 gün | Private endpoint uyumluluğu; metadata/DNS/redirect reddi. |
| F | Legacy ciphertext kapatma runbook'u | P1 | 2–5 gün + veri hacmi | Veri envanteri/rotate/restore; eski format reddi. |
| G | Worker failure/readiness ve panic stratejisi | P1 | 4–8 gün | Fault injection; sessiz worker kaybı ve kör devam yok. |
| H | Hedef ortam kapasite/HA/restore kabulü | P1 kapı | 5–10 gün + altyapı | Kaynak SHA, workload, RPO/RTO/SLO ve ham ölçümler kayıtlı. |
| I | Heartbeat/roots ve admission optimizasyonu | P1 ölçüm / P2 değişiklik | 5–12 gün | H sonuçları; fencing, cancel ve kota garantileri korunur. |
| J | Güvenli logging, exporter/pool/runtime metrikleri | P1/P2 | 4–8 gün | Secret leak testi; backlog/lease alarmı; bounded cardinality. |
| K | SQL parametreleme ve REST path uyumluluğu | P2 | 3–5 gün | Gerçek PG ve upstream delimiter regresyonları. |
| L | PII/transport profiling ve hedefli optimizasyon | P2 | 4–8 gün | Aynı workload'da alloc/live-heap/CPU ve redaction eşdeğerliği. |
| M | İlk KMS backend'i ve restore/rotation tasarımı | P2 / şartlı P1 | 10–20 gün | IAM, outage, key version, recovery ve maliyet kabulü. |
| N | OTel child spans ve web route splitting | P2/P3 | 4–8 gün | Trace bütünlüğü, payload güvenliği, cold-load/browser ölçümü. |

**Önerilen sıra:** Önce A/D/F ve B/C tasarım kararı; aynı dönemde H için hedef trafik ve SLO tanımı. Ardından B/C/E/G ve en gerekli J sinyalleri. I/L/ANN/cache değişiklikleri ancak H'nin darboğaz ölçümüyle seçilmeli. KMS, multi-IdP, mTLS ve SIEM native adapter'ları müşteri sözleşmesine göre planlanmalı. Kaynak rapordaki bütün P0'ların 1–2 haftada, dört KMS sağlayıcısının bir ayda tamamlanacağı varsayımı kullanılmamalı.

## 8. Üretime geçiş kabul kapıları

1. **Güven sınırı:** Onaylı upstream/araç listesi, annotation/retry kararı, argüman doğrulaması ve secret/egress ayarları açık. Kritik yazma çağrısında belirsiz sonuç otomatik tekrar edilmiyor.
2. **Yetki tutarlılığı:** Oturum/token/role revocation, claim expiry, node değişimi ve DB kesintisi testleri başarılı; cache eklendiyse izin verilen gecikme ölçülmüş.
3. **İş sürekliliği:** İki replica rolling restart, DB primary promotion/fencing, backup/PITR ve anahtarlarla restore tatbikatı tamamlanmış; hedef RPO/RTO müşteri/operatör tarafından belirlenip ölçülmüş.
4. **Kapasite:** Güncel commit ve gerçekçi MCP/REST karışımı; kısa/uzun iş, PII/payload capture ve tek-hot-server dahil. Hedef p95/p99, error budget, kaynak payı ve overload davranışı kabul edilmiş.
5. **Audit:** Admission/outcome reconciliation, eksik/belirsiz outcome alarmı, exporter kesinti/tekrar, legal hold/retention ve bağımsız sink doğrulanmış. Zero loss iddiası yalnız ölçülen arıza/replication kapsamına bağlanmış.
6. **Operasyon:** DB/WAL/disk, pool, renew/probe, audit lag ve secret lifecycle alarm/runbook sahipleri atanmış. Gerekli GitHub kontrolleri ilgili PR head üzerinde başarılı.

## 9. Bu incelemede çalıştırılan kontroller

Ortam: Darwin arm64, Apple M5 Max. Bunlar üretim kapasite ölçümü değildir.

| Kontrol | Sonuç |
|---|---|
| `go test -count=1 ./internal/resilience ./internal/pii ./internal/policy ./internal/mcp ./internal/secrets` (`core/`) | Beş paket başarılı. DB/Kubernetes entegrasyonu kapsam dışı. |
| `go test -run '^$' -bench BenchmarkAuditMasking -benchmem ./internal/audit` | Başarılı; aşağıdaki yeni ölçümler elde edildi. |
| Geçici kopyada fake-clock half-open deneyi | Başarılı: ignored probe sonrası 29. saniyede ret, 30. saniyede yeni probe. Bu mevcut kusuru doğrulayan gözlem; düzeltme testi değil. |
| `npm run build` (`web/`) | Başarılı; ana JS 1.016,56 kB / gzip 279,79 kB; >500 kB chunk uyarısı var. |

| Masking benchmark senaryosu | ns/op | B/op | allocs/op | Yaklaşık toplam tahsis / nominal yük |
|---|---:|---:|---:|---:|
| 1 KiB | 22.437 | 14.752 | 52 | 14,4x |
| 64 KiB | 1.871.500 | 348.937 | 62 | 5,3x |
| 1 MiB | 29.823.882 | 6.061.998 | 87 | 5,8x |

Benchmark nominal boyuttan biraz küçük JSON üretir (`size-60` string dolgusu); oranlar yaklaşık ve nominal boyuta göredir. 512 KiB/26 kural senaryosu çalıştırılmadı; bu nedenle kaynakta verilen 512 KiB→10,16 MB ölçümünü yanlış diye kesinleştirmek yerine **yeniden üretilemedi** olarak sınıflandırıyoruz.

Half-open deneyinin yeniden üretimi: `breaker.go` dosyasını geçici bir test paketine kopyala; `now` sahte saatini kullan; threshold=1, cooldown=30 s; `Failure()` ile aç; 30 s ilerletip `Allow()` ile probe al; Ignore yolundaki gibi Success/Failure çağırma; 29 s sonra `Allow()==false`, bir saniye sonra `Allow()==true` olduğunu doğrula. Üretim dosyasına veya mevcut testlere değişiklik yapılmadı.

Bu raporun sonucu: mevcut temeli koruyarak güven ve hata yaşam döngüsü boşluklarını kapatmak uygulanabilir ve değerlidir. Genel kapasite, sertifikasyon ve “tam üretim hazır” iddiaları ancak hedef ortamdaki kabul kanıtıyla verilebilir.
