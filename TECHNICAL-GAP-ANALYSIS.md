# vMCP Projesi Teknik Mimari Açık ve Eksiklik Analizi (Gap Analysis)

> **Kapsam:** Mevcut `vMCP` (Go 1.27 / PostgreSQL / Redis / React) kod tabanının, [`TECHNICAL-SYSTEM-ARCHITECTURE.md`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/TECHNICAL-SYSTEM-ARCHITECTURE.md) dosyasında tanımlanan **"Kurumsal MCP Ekosistemi: Gateway Dışı Kalan Altyapının Uçtan Uca Teknik Mimari Tasarımı"** hedefleri doğrultusunda incelenmesi; mevcut yeteneklerin tespit edilmesi ve eksik olan tüm bileşenlerin kod tabanına dayalı detaylı analizi.  
> **Referans Kod Tabanı:** `core/` (Data Plane, Control Plane, Store, Upstream, Virtual, Audit, PII, Resilience, RateLimit, Health), `deploy/` (Docker Compose, Helm), `api/` (OpenAPI).  
> **Temel İlke:** Mevcut projenin mimari standartları (Go idiomları, `pgx/v5`, `slog`, Goose migrations, `apierror`, tenant izolasyonu) korunmuş; halihazırda çalışan özellikler tekrar önerilmemiş veya çoklanmamıştır.

---

## 1. Yönetici Özeti ve Mimari Uyumluluk Matrisi

Aşağıdaki tablo, `TECHNICAL-SYSTEM-ARCHITECTURE.md` belgesindeki bileşenlerin mevcut `vMCP` kod tabanındaki durumunu özetlemektedir:

| Alt Sistem / Bileşen | Mimari Tasarımdaki Hedef | Mevcut Kod Tabanındaki Durum | Uyumluluk Durumu |
| :--- | :--- | :--- | :--- |
| **Alt Sistem 1: Vektör Arama & Embedding** | Qdrant (HNSW, Cosine, int8 SQ) + HF TEI (384-dim, bge-small) | Kod tabanında Qdrant, TEI, embedding veya vektör arama kodu **bulunmamaktadır**. | ❌ **Tamamen Eksik** |
| **Alt Sistem 2: L1 Semantik Cache** | Redis `mcp:cache:query:<sha256(query+role)>`, Gzip JSON, 15m TTL | Redis sadece opsiyonel IP/Key rate limiter için kullanılmaktadır; semantik cache **yoktur**. | ❌ **Tamamen Eksik** |
| **Alt Sistem 2: Olay Veriyolu (Event Bus)** | Redis Pub/Sub `mcp:catalog:events` (TOOL_UPDATED, pod cache eviction) | Sadece PostgreSQL `LISTEN/NOTIFY` (snapshot sync) vardır; Redis Pub/Sub **yoktur**. | ❌ **Tamamen Eksik** |
| **Alt Sistem 2: Dağıtık Rate Limiter** | Redis Lua Token Bucket (`mcp:ratelimit:<user_id>:<tool_id>`) | Redis Lua Token Bucket vardır (`ratelimit.go`), ancak anahtar yapısı virtual server/key düzeyindedir; tool bazlı granülerlik **eksiktir**. | ⚠️ **Kısmi / Revizyon Gerekli** |
| **Alt Sistem 3: Veritabanı Şeması** | `mcp_servers`, `mcp_tools`, `mcp_audit_logs` (Partitioned) | `upstreams`, `upstream_tools`, `audit_events` tabloları vardır. Ancak kolonlar ve partitioning yapısı **farklıdır/eksiktir**. | ⚠️ **Kısmi / Şema Güncellemesi Gerekli** |
| **Alt Sistem 3: Asenkron Audit Pipeline** | `chan AuditLog` -> `pgx.CopyFrom` batch insert (500ms / 100 log) | `audit.Writer` goroutine/channel buffer mevcuttur; ancak `pgx.CopyFrom` yerine `tx.Exec` tekil insert kullanılmaktadır ve payload kasıtlı tutulmamaktadır. | ⚠️ **Kısmi / Optimizasyon Gerekli** |
| **Alt Sistem 4: Hibrit Taşıma Katmanları** | Stdio (`os/exec` pool), Uzak (`streamable_http`, `sse`, `grpc`, `unix`) | Sadece `streamable_http` ve `sse` desteklenmektedir (`00026_http_only_upstreams.sql`). Stdio, gRPC ve Unix socket **yoktur**. | ❌ **Önemli Ölçüde Eksik** |
| **Alt Sistem 4: Devre Kesici (Circuit Breaker)** | 3 ardışık hata (>5s) -> Open, 30s fallback, Half-Open | Yerleşik Breaker vardır (`resilience/breaker.go`), ancak hata eşiği sınıflandırması ve standart fallback yanıtı **revizyon gerektirir**. | ⚠️ **Mevcut / Entegrasyon Gerekli** |
| **Alt Sistem 4: Otomatik Sağlık Taraması** | 15s periyodik JSON-RPC `ping`, offline -> Qdrant `is_active: false` | `health.Checker` mevcuttur; fakat `Initialize` yollar, Qdrant payload senkronizasyonu **yoktur**. | ⚠️ **Kısmi / Genişletme Gerekli** |
| **Bölüm 6: Veri Senkronizasyon Hattı** | CDC / Notify -> Sync Worker -> TEI Embedding -> Qdrant -> Redis Pub/Sub | Böyle bir worker veya senkronizasyon boru hattı **bulunmamaktadır**. | ❌ **Tamamen Eksik** |
| **Bölüm 7: Zero-Trust Güvenlik & Doğrulama** | JSON Schema validation (`inputSchema`), Tool bazlı `allowed_roles` | Parametreler şema doğrulamasına tabi tutulmadan iletilmektedir; tool düzeyinde rol kontrolü **yoktur**. | ❌ **Eksik** |
| **Bölüm 7: Hassas Veri Maskeleme (PII)** | RE2 Regex maskeleme (`***MASKED***`) | RE2 Regex motoru mevcuttur (`pii/pii.go`); ancak audit tablosuna maskelenmiş argüman yazımı **kapalıdır**. | ⚠️ **Mevcut / Audit'e Bağlanmalı** |
| **Bölüm 8: Altyapı & Port Matrisi** | Qdrant (6333/6334), Redis (6379), TEI (8081), Postgres (5432) | Docker Compose'da Postgres ve opsiyonel Redis vardır. TEI ve Qdrant **yoktur**. **Port 8081 Control-Plane ile çakışmaktadır.** | ⚠️ **Altyapı Eksik / Port Çakışması** |

---

## 2. Mevcut vMCP Altyapısında ZATEN BULUNAN (Yeniden Yazılmaması Gereken) Yetenekler

Projenin mevcut kod tabanında başarılı şekilde uygulanmış, sıfırdan geliştirilmeye **ihtiyaç duyulmayan** modüller şunlardır:

1. **Redis Lua Tabanlı Token Bucket Motoru (`core/internal/ratelimit/ratelimit.go`)**:
   - `tokenBucket` Lua script'i (`redis.NewScript`) Redis'in dahili saatini (`TIME`) kullanarak milisaniye hassasiyetinde token yenileme ve tüketme mantığını zaten hatasız yürütmektedir.
   - Yerel bellek fallback'i (`Local` in-memory limiter) mevcuttur.

2. **Temel Devre Kesici Durum Makinesi (`core/internal/resilience/breaker.go`)**:
   - `Closed`, `Open`, `HalfOpen` durumlarını atomik olarak yöneten, thread-safe, `Threshold` ve `Cooldown` parametrelerini destekleyen `Breaker` ve `Registry` yapıları hazırdır.

3. **PII Maskeleme Motoru (`core/internal/pii/pii.go`)**:
   - Kredi Kartı (Luhn algoritması doğrulamalı), IBAN (Mod-97 doğrulamalı), TCKN (Algoritmik hane doğrulamalı), US SSN, E-posta ve Telefon tespit eden yüksek performanslı RE2 regex motoru hazırdır.
   - Tool parametreleri ve sonuçları üzerinde JSON derinliğinde maskeleme (`MaskJSON`) fonksiyonları çalışır durumdadır.

4. **Arka Plan Asenkron Audit Buffer Altyapısı (`core/internal/audit/audit.go`)**:
   - `Writer` yapısı Go kanalı (`chan Event`, buffer 4096), periyodik ticker ve graceful shutdown mekanizması ile data-plane gecikmesini engellemek üzere tasarlanmıştır.

5. **PostgreSQL LISTEN/NOTIFY Tabanlı Snapshot Dağıtımı (`core/internal/snapshot/`)**:
   - `config_revision` tablosu ve PostgreSQL bildirimleri ile data-plane düğümlerinin anlık konfigürasyon değişikliklerini yakalama altyapısı mevcuttur.

6. **Agent & OAuth Kimlik Doğrulama (`core/internal/agentauth/`, `core/internal/oidc/`)**:
   - Asimetrik JWT doğrulama (JWKS), OAuth 2.0 Client Credentials desteği ve SHA-256 agent key hash kontrolü tamamlanmıştır.

---

## 3. Alt Sistem Bazında Detaylı Eksiklik (Gap) Analizi

---

### ALT SİSTEM 1: Vektör Arama & Embedding Altyapısı (Qdrant & HF TEI)

#### 1. Qdrant İstemcisi ve Koleksiyon Yönetimi Eksikliği
- **Mevcut Durum:** [`core/go.mod`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/go.mod) ve alt paketlerde Qdrant'a dair hiçbir gRPC/REST istemcisi veya veri modeli bulunmamaktadır.
- **Eksikler:**
  - `github.com/qdrant/go-client` (veya `google.golang.org/grpc` üzerinden protobuf istemcisi) bağımlılığı projede yoktur.
  - HNSW graf indeksini (`m: 16`, `ef_construct: 100`, `ef_search: 64`, Mesafe: `Cosine`) tanımlayan koleksiyon başlatma (`CreateCollection`) kodu eksiktir.
  - 10.000 araç için bellek tüketimini %75 azaltacak **Scalar Quantization (int8)** konfigürasyonu (`QuantizationConfig`) tanımlanmamıştır.

#### 2. Payload İndeksleme ve Filtreleme Şeması Eksikliği
- **Mevcut Durum:** Disk I/O yapmadan bellek üzerinden filtrelenmiş arama yapacak Qdrant Payload alanları ve payload indeksleri tanımlanmamıştır.
- **Eksikler:**
  - `allowed_roles` (Keyword index)
  - `domain` (Keyword index)
  - `server_id` (Keyword index)
  - `is_active` (Bool index)
  - Gateway'in arama yaparken kullanıcının rolüne (`allowed_roles MATCH user_role`) ve sunucu durumuna (`is_active == true`) göre Qdrant `Filter` sorguları oluşturacak arama katmanı eksiktir.

#### 3. HuggingFace TEI Embedding İstemcisi ve Metin Pipeline Eksikliği
- **Mevcut Durum:** Herhangi bir embedding çıkarım servisi bağlantısı veya vektör üretim kodu yoktur.
- **Eksikler:**
  - HuggingFace TEI REST servisine (`POST /embed`) HTTP Keep-Alive ile bağlanan bir Go istemcisi (`core/internal/embedding`) eksiktir.
  - Mimari belgedeki girdi formatlayıcı fonksiyon kodlanmamıştır:
    ```go
    // Eksik olan dönüştürücü:
    text := fmt.Sprintf("Name: %s | Domain: %s | Description: %s | Parameters: %s",
        tool.Name, tool.Domain, tool.Description, extractedParamKeys)
    ```
  - TEI container'ından dönen `[]float32` vektörünü normalize eden ve Qdrant point'i haline getiren hat eksiktir.

#### 4. Gateway İçi Semantik Tool Arama (Semantic Tool Discovery) Eksikliği
- **Mevcut Durum:** [`core/internal/virtual/handler.go`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/internal/virtual/handler.go#L306-L311) satırlarında `mcp.MethodToolsList` çağrıldığında, `vs.Tools` içindeki tüm statik araçlar ham liste olarak istemciye iletilmektedir.
- **Eksikler:**
  - 10.000 aracın tamamı LLM istemcisine gönderildiğinde token penceresi taşacaktır. Kullanıcı sorgusunu (`user_prompt` veya arama filtresini) alıp Qdrant üzerinde vektörel arama yaparak yalnızca en alakalı **Top-K aracı** dönen dinamik semantik filtreleme mantığı eksiktir.

---

### ALT SİSTEM 2: Dağıtık Cache, Olay Veriyolu & Rate-Limiter (Redis Cluster)

#### 1. L1 Semantik Önbellek (Semantic Cache) Eksikliği
- **Mevcut Durum:** vMCP'de Redis bağlantısı yalnızca `core/cmd/data-plane/main.go` içerisinde `ratelimit.Redis` için açılmaktadır. Araç aramaları veya şemalar için önbellekleme yapılmamaktadır.
- **Eksikler:**
  - `mcp:cache:query:<sha256(user_query + role)>` anahtar formatını üreten hash motoru eksiktir.
  - Qdrant ve TEI'ye gitmeden, cache hit durumunda Top-K tool ID'lerini ve şemalarını Gzip JSON formatında `< 0.4 ms` süreyle dönen L1 cache yöneticisi (`core/internal/semanticcache`) eksiktir.
  - 15 dakikalık TTL ve bellek yönetimi (Redis Maxmemory-eviction) entegrasyonu yoktur.

#### 2. Redis Pub/Sub Olay Veriyolu (Event Bus) ve Pod Cache Eviction Eksikliği
- **Mevcut Durum:** vMCP sadece PostgreSQL `LISTEN/NOTIFY` mekanizmasını kullanarak [`core/internal/snapshot/listener.go`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/internal/snapshot/listener.go) üzerinden tüm snapshot'ı periyodik olarak yenilemektedir.
- **Eksikler:**
  - `mcp:catalog:events` kanalına bağlanan bir Redis Pub/Sub dinleyicisi yoktur.
  - Aşağıdaki JSON olayını dinleyip yalnızca ilgili aracın yerel önbelleğini düşüren (`Cache Eviction`) olay işleyicisi eksiktir:
    ```json
    {
      "event": "TOOL_UPDATED",
      "tool_id": "mcp-devops__k8s_list_pods",
      "timestamp": 1728751800
    }
    ```

#### 3. Rate Limiter Anahtar Yapısındaki Granülerlik Farkı
- **Mevcut Durum:** [`core/internal/virtual/handler.go`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/internal/virtual/handler.go#L443) satırında rate limit kontrolü yalnızca sanal sunucu ve agent key bazlı yapılmaktadır (`h.rateLimited(ctx, vs, snap, sess.keySlug)`).
- **Eksikler:**
  - Mimaride talep edilen `mcp:ratelimit:<user_id>:<tool_id>` formatında, hem kullanıcı kimliği hem de spesifik araç bazında bağımsız token bucket limiti uygulayan mantık eksiktir.

---

### ALT SİSTEM 3: Kalıcı Katalog, Yetkilendirme (RBAC) & Audit Trail (PostgreSQL 16)

#### 1. Veritabanı Şeması ve Migration Eksikleri

##### A. Sunucu Kataloğu Tablosu (`mcp_servers` vs `upstreams`)
- Mevcut tablo: `upstreams` ([`00001_init.sql`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/migrations/00001_init.sql#L15-L27)).
- **Eksik Alanlar:**
  - `domain VARCHAR(64)`: Araçların ve sunucuların iş alanına göre gruplanması (`devops`, `database`, `crm`) mevcut tabloda yoktur.
  - `health_status VARCHAR(16)`: Sunucunun merkezi sağlık durumu (`healthy`, `degraded`, `offline`) `upstreams` tablosunda tutulmamaktadır (mevcut sistem bunu node bazlı `upstream_health` tablosunda ayrı tutmaktadır).
  - `consecutive_failures INT`: Ardışık hata sayısı merkezi tabloda mevcut değildir.
  - `transport_type`: Mevcut `transport` kolonu sadece `'streamable_http'` ve `'sse'` kabul etmektedir; `'stdio'` ve `'grpc'` kısıt (`CHECK`) nedeniyle veritabanı seviyesinde reddedilmektedir.

##### B. 10.000 Araç Kataloğu Tablosu (`mcp_tools` vs `upstream_tools`)
- Mevcut tablo: `upstream_tools` ([`00001_init.sql`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/migrations/00001_init.sql#L30-L38)).
- **Eksik Alanlar:**
  - `allowed_roles TEXT[] NOT NULL DEFAULT '{"admin"}'`: Araç bazlı rol yetkilendirmesi tabloda yoktur.
  - `vector_id UUID`: Qdrant üzerindeki point ID karşılığı tabloda yoktur.
  - `is_active BOOLEAN DEFAULT TRUE`: Aracın aktif/pasif bayrağı yoktur.
  - `version INT DEFAULT 1`: Şema versiyon takibi yoktur.
  - `idx_tools_roles (USING GIN(allowed_roles))` ve `idx_tools_search` indeksleri eksiktir.

##### C. Audit Log Tablosu (`mcp_audit_logs` vs `audit_events`)
- Mevcut tablo: `audit_events` ([`00005_audit_events.sql`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/migrations/00005_audit_events.sql), [`00018_durable_audit.sql`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/migrations/00018_durable_audit.sql)).
- **Eksik Yapısal Özellikler:**
  - **Tablo Partitioning Yok:** Mimaride günlük bölümlenmiş `PARTITION BY RANGE (created_at)` zorunlu tutulmuştur; mevcut `audit_events` tablosu düz (monolitik) bir tablodur ve milyonlarca kayıt biriktiğinde sorgu/silme performansı düşecektir.
  - **Argüman ve Yanıt Kaydı Yok:** Mevcut [`core/internal/audit/audit.go`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/internal/audit/audit.go#L15) dosyasında:
    ```go
    // Event is one audited action. Arguments and results are intentionally absent.
    ```
    ifadesiyle argümanlar ve sonuçlar bilinçli olarak kaydedilmemektedir. Mimaride ise `arguments JSONB` (maskelenmiş haliyle) ve `response_summary TEXT` (özet yanıt) alanlarının saklanması şart koşulmuştur.
  - `user_role VARCHAR(64)` alanı eksiktir.

#### 2. Asenkron Audit Pipeline Performans Eksikliği (`pgx.CopyFrom`)
- **Mevcut Durum:** [`core/internal/audit/audit.go`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/internal/audit/audit.go#L246-L250) satırlarında `insert()` fonksiyonu, toplanan batch içindeki her bir event için tek tek `Insert(ctx, tx, e)` fonksiyonunu döngü içerisinde çağırmaktadır (`tx.Exec` ile tek tek SQL insert).
- **Eksikler:**
  - 10.000 araç ve yüksek trafik altında PostgreSQL üzerinde I/O yükünü en aza indirmek ve sıfır gecikme sağlamak için tasarlanan **`pgx.CopyFrom` binary streaming protokolü** entegrasyonu eksiktir.
  - Flush zamanlayıcısı mimaride **500 ms veya 100 log** iken, mevcut kodda 1000 ms (1 sn) ve 200 log olarak ayarlanmıştır.

---

### ALT SİSTEM 4: 300 Downstream MCP Sunucu Filosu Yönetimi

#### 1. Hibrit Taşıma Katmanları (Transport Types) Eksikliği
- **Mevcut Durum:** [`core/migrations/00026_http_only_upstreams.sql`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/migrations/00026_http_only_upstreams.sql) ile veritabanı kısıtı `CHECK (transport IN ('streamable_http', 'sse'))` şeklindedir. [`core/internal/upstream/client.go`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/internal/upstream/client.go) sadece HTTP/SSE bağlantılarını açabilmektedir.
- **Eksikler:**
  1. **Yerel Process Taşıyıcısı (`stdio`):**
     - Go'nun `os/exec.Command` mekanizması ile downstream process'leri başlatan,
     - `stdin` için `io.WriteCloser`, `stdout` için `bufio.Reader` üzerinden JSON-RPC framing yapan,
     - Kalıcı process havuzu (`process pool`), çökme durumunda otomatik yeniden başlatma (respawn) ve zombi process temizleme yöneticisi tamamen eksiktir.
  2. **gRPC Taşıyıcısı (`grpc`):**
     - gRPC tabanlı MCP sunucuları ile çift yönlü stream (bidirectional streaming) kuracak transport adaptörü eksiktir.
  3. **Unix Domain Socket Taşıyıcısı:**
     - Yerel sock dosyaları (`/var/run/mcp-*.sock`) üzerinden haberleşme katmanı eksiktir.

#### 2. Uzak Servis Bağlantı Havuzu (Connection Pool) Boyutlandırması
- **Mevcut Durum:** Standart `http.Client` ve `net/http.Transport` kullanılmaktadır.
- **Eksikler:**
  - 300 eşzamanlı upstream sunucu için `Keep-Alive: 90s`, `MaxIdleConns: 3000`, `MaxIdleConnsPerHost: 20`, `IdleConnTimeout: 90s` gibi TCP el sıkışma (handshake) gecikmesini sıfırlayacak özel bağlantı havuzu yapılandırması eksiktir.

#### 3. Circuit Breaker Entegrasyonu ve Standart Fallback
- **Mevcut Durum:** [`core/internal/resilience/breaker.go`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/internal/resilience/breaker.go) bulunmaktadır.
- **Eksikler:**
  - Circuit Breaker açıldığında (Open durumunda) mimaride istenen standart JSON-RPC fallback mesajı (`"Sunucu geçici olarak devre dışı"`, 30 saniye boyunca denemeden hızlı hata dönme) standartlaştırılmalıdır.
  - Zaman aşımı kriteri (timeout > 5 saniye) ardışık 3 hatada tetiklenecek şekilde hata sınıflandırıcısı (`classify.go`) ile tam senkronize edilmelidir.

#### 4. Otomatik Sağlık Taraması ve Qdrant Senkronizasyonu
- **Mevcut Durum:** [`core/internal/health/health.go`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/internal/health/health.go) her interval'de `Initialize` el sıkışması göndermektedir.
- **Eksikler:**
  - Mimaride sunucuları yormamak için her 15 saniyede bir hafif JSON-RPC `ping` atılması tasarlanmıştır.
  - Sağlık kontrolü başarısız olan veya zaman aşımına uğrayan bir sunucunun, PostgreSQL'de `offline` yapıldıktan sonra **Qdrant'taki tüm araçlarının `is_active: false` olarak payload güncellemesine tabi tutulması** katmanı tamamen eksiktir.

---

### VERİ SENKRONİZASYON HATTI (PostgreSQL -> Qdrant Pipeline)

- **Mevcut Durum:** Kod tabanında PostgreSQL değişikliklerini Qdrant ve Redis'e taşıyan hiçbir senkronizasyon servisi yoktur.
- **Eksik Bileşenler:**
  1. **Tetikleyici / CDC Katmanı:** PostgreSQL `mcp_tools` tablosundaki INSERT / UPDATE işlemlerini yakalayan `AFTER INSERT OR UPDATE` trigger'ı ve `pg_notify` yayını (veya Debezium/WAL tabanlı CDC).
  2. **Sync Worker Daemon (`core/cmd/sync-worker`):**
     - Bildirimi alan bağımsız arka plan servis executabledır.
     - Değişen aracı PostgreSQL'den çeker.
     - HuggingFace TEI'ye HTTP isteği atarak 384 boyutlu vektör oluşturur.
     - Qdrant `/collections/mcp_tools/points` endpoint'ine payload ile birlikte upsert eder.
     - Redis Pub/Sub `mcp:catalog:events` kanalına `TOOL_UPDATED` JSON mesajını yayınlar.

---

### GÜVENLİK, İZOLASYON & MASKELEME (Zero-Trust Tool Execution)

#### 1. JSON Schema (`inputSchema`) Validasyon Eksikliği (Schema Injection Engeli)
- **Mevcut Durum:** [`core/internal/virtual/handler.go`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/core/internal/virtual/handler.go#L412-L420) satırlarında, gelen parametreler yalnızca geçerli bir JSON mı diye kontrol edilmekte (`json.Unmarshal(msg.Params, &p)`), ardından şema doğrulaması yapılmadan doğrudan downstream upstream'e yönlendirilmektedir.
- **Eksikler:**
  - Tool'un `inputSchema` JSON Schema tanımına göre parametre tiplerini, zorunlu alanlarını (`required`) ve ek özelliklerini doğrulayan bir JSON Schema doğrulayıcısı (`github.com/santhosh-tekuri/jsonschema/v5`) entegre edilmemiştir. Bu durum mimaride belirtilen **Schema Injection** açığına yol açmaktadır.

#### 2. Araç Düzeyinde RBAC & Rol Eşleme (`allowed_roles`)
- **Mevcut Durum:** vMCP yetkilendirmesi sadece Virtual Server seviyesinde ve statik `read_only` bayrağı ile yapılmaktadır (`core/internal/policy/policy.go`).
- **Eksikler:**
  - Kullanıcının JWT token'ından çıkarılan rolün (örn: `roles: ["devops"]`), çağrılmak istenen aracın `mcp_tools.allowed_roles` listesinde olup olmadığını kontrol eden Zero-Trust Policy Engine kuralı eksiktir.

#### 3. Maskelenmiş Argümanların Audit Pipeline'ına İletilmesi
- **Mevcut Durum:** PII maskeleme argümanları maskelemektedir (`pii.MaskJSON`), ancak maskelenen bu parametreler `audit.Event` nesnesine eklenmemektedir.
- **Eksikler:**
  - Maskelenmiş argümanların (`arguments JSONB`) ve KVKK gereği özetlenmiş yanıtın (`response_summary TEXT`) audit loguna kaydedilmesi sağlanmalıdır.

---

### ALTYAPI BİLEŞENLERİ, KONTEYNERLER VE PORT MATRİSİ

#### 1. Kritik Port Çakışması Analizi
- **Sorun:** 
  - [`deploy/docker-compose.yml`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/deploy/docker-compose.yml#L42) satırında `control-plane` servisi `:8081` portunu kullanmaktadır (`VMCP_CP_ADDR: ":8081"`).
  - [`TECHNICAL-SYSTEM-ARCHITECTURE.md`](file:///Users/mustafaderyol/Documents/GitHub/vmcp/TECHNICAL-SYSTEM-ARCHITECTURE.md#L251) tablosunda `HuggingFace TEI` servisi için `8081 (HTTP)` portu tahsis edilmiştir.
- **Gerekli Revizyon:** Port çakışmasını engellemek için `mcp-tei` servisinin host/iç portu `8085` veya `8090` olarak belirlenmelidir.

#### 2. Eksik Servis Konteynerleri (`deploy/docker-compose.yml`)
- **Qdrant Servisi Eksik:**
  ```yaml
  # Eklenmesi gereken servis:
  qdrant:
    image: qdrant/qdrant:v1.12.0
    ports:
      - "6333:6333" # HTTP
      - "6334:6334" # gRPC
    volumes:
      - qdrant_storage:/qdrant/storage
  ```
- **HuggingFace TEI Servisi Eksik:**
  ```yaml
  # Eklenmesi gereken servis (port 8085 ile):
  tei:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.5
    environment:
      - MODEL_ID=BAAI/bge-small-en-v1.5
    ports:
      - "8085:80"
  ```
- **Redis Servisi Varsayılan Değil:** Docker Compose'da `profiles: [redis]` olarak tanımlıdır; mimarinin çalışabilmesi için varsayılan profilde başlatılmalıdır.

---

## 4. Uygulama ve Entegrasyon Yol Haritası (Adım Adım Plan)

Mevcut projenin standartlarını (`Go 1.27`, `pgx/v5`, `slog`, Goose migrations, `core/internal/` paket düzeni) koruyarak bu mimariyi hayata geçirmek için izlenmesi gereken adımlar:

### Faz 1: Veritabanı ve Şema Genişletmeleri (Migrations)
1. **Migration 00027_mcp_enterprise_schema.sql**:
   - `upstreams` tablosuna `domain`, `health_status`, `consecutive_failures` alanlarını eklemek; `transport` kısıtına `'stdio'` ve `'grpc'` tiplerini dahil etmek.
   - `upstream_tools` tablosuna `domain`, `allowed_roles TEXT[]`, `vector_id UUID`, `is_active BOOLEAN`, `version INT` eklemek; GIN ve arama indekslerini oluşturmak.
   - Günlük partition destekli `mcp_audit_logs` tablosunu ve `arguments JSONB`, `response_summary TEXT` alanlarını oluşturmak.

### Faz 2: Altyapı ve Konteyner Entegrasyonu
1. `deploy/docker-compose.yml` dosyasına `qdrant` (port 6333/6334), `tei` (port 8085) ve `redis` (varsayılan) servislerini eklemek.
2. `core/internal/config` içerisine Qdrant URL, TEI URL ve Redis Event Bus ortam değişkenlerini (`VMCP_QDRANT_URL`, `VMCP_TEI_URL`, `VMCP_REDIS_EVENTS_ENABLED`) tanımlamak.

### Faz 3: İstemci ve Katman Kütüphaneleri (`core/internal/`)
1. **`core/internal/qdrant`**: Qdrant gRPC/REST istemcisi, HNSW + int8 SQ koleksiyon başlatıcı ve payload filtreli Top-K arama fonksiyonlarını yazmak.
2. **`core/internal/embedding`**: HuggingFace TEI REST istemcisini ve tool embedding metin formatlayıcısını yazmak.
3. **`core/internal/semanticcache`**: Redis tabanlı L1 semantik cache (`mcp:cache:query:...`) ve Pub/Sub olay dinleyicisini geliştirmek.
4. **`core/internal/upstream/stdio`**: `os/exec` tabanlı kalıcı process yönetimi ve JSON-RPC `stdio` transportunu eklemek.

### Faz 4: Data Plane ve Güvenlik Entegrasyonu
1. `core/internal/virtual/handler.go`:
   - `mcp.MethodToolsList` çağrısında semantik arama (Top-K) ve L1 cache entegrasyonu yapmak.
   - `mcp.MethodToolsCall` çağrısında `inputSchema` JSON Schema doğrulaması ve tool `allowed_roles` kontrolünü eklemek.
   - `core/internal/audit/audit.go` içinde `pgx.CopyFrom` batch insert mekanizmasını ve maskelenmiş argüman kaydını entegre etmek.
2. `core/internal/health/health.go`:
   - Sağlık taramasını JSON-RPC `ping` ile yapmak ve sunucu düştüğünde Qdrant'ta `is_active: false` payload güncellemesi göndermek.

### Faz 5: Veri Senkronizasyon Daemon'ı (`core/cmd/sync-worker`)
1. PostgreSQL bildirimlerini dinleyen, TEI'den vektör alan, Qdrant'a yazan ve Redis'e `TOOL_UPDATED` yayınlayan bağımsız `sync-worker` servisini yazmak.

---

## 5. Sonuç

`TECHNICAL-SYSTEM-ARCHITECTURE.md` dosyasındaki tasarım, mevcut `vMCP` projesinin üzerine kurumsal ölçekte (10.000 araç, 300 sunucu filosu) **Vektör Arama (Qdrant)**, **Yerel Embedding (HF TEI)**, **L1 Semantik Önbellek (Redis)**, **Çoklu Protokol Taşıyıcıları (Stdio/gRPC)**, **JSON Schema Doğrulaması** ve **CDC Senkronizasyon Hattı** ekleyen ileri seviye bir eklentidir.

Yukarıda detaylandırılan analiz, mevcut çalışan hiçbir özelliği bozmadan ve kod tabanında mükerrerlik yaratmadan bu mimarinin eksiksiz ve güvenli bir şekilde hayata geçirilmesini sağlayacak teknik temeli oluşturmaktadır.
