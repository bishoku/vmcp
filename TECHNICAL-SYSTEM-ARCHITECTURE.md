# Kurumsal MCP Ekosistemi: Gateway Dışı Kalan Altyapının Uçtan Uca Teknik Mimari Tasarımı

> **Kapsam:** Go MCP Gateway'in var olduğu kabul edilerek; **Vektör Veritabanı (Qdrant)**, **Önbellek & Mesajlaşma (Redis)**, **Kayıt & Denetim (PostgreSQL)**, **300 Downstream MCP Sunucu Filosu** ve **Veri Senkronizasyon Hattının** en ince ayrıntısına kadar teknik tasarımı.  
> **Hedef:** 300 Sunucu, 10.000 Araç, 99.99% Yüksek Erişilebilirlik (HA), Sub-millisecond Gecikme, Kurumsal RBAC & Audit.

---

## 1. Mimari Katman Haritası (Gateway Odak Dışı)

Gateway istemcilerle konuşurken, arkasında çalışan 4 ana alt sistem bulunur:

```
                          ┌──────────────────────────┐
                          │      GO MCP GATEWAY      │ (Mevcut Varsayılan)
                          └─────────────┬────────────┘
                                        │
        ┌───────────────────────────────┼───────────────────────────────┐
        │ (gRPC - Vektör Arama)         │ (TCP - Cache & PubSub)        │ (SQL - Audit & Meta)
        ▼                               ▼                               ▼
┌─────────────────────────┐   ┌─────────────────────────┐   ┌─────────────────────────┐
│     ALT SİSTEM 1:       │   │     ALT SİSTEM 2:       │   │     ALT SİSTEM 3:       │
│    VEKTÖR ARAMA &       │   │     DAĞITIK CACHE &     │   │      KATALOG, RBAC      │
│   EMBEDDING MOTORU      │   │     OLAY VERİYOLU       │   │    & AUDIT VERİTABANI   │
│       (Qdrant)          │   │        (Redis)          │   │      (PostgreSQL)       │
└─────────────────────────┘   └─────────────────────────┘   └─────────────────────────┘
        ▲                                                                   ▲
        │                                                                   │
        └───────────────────────[ SYNC WORKER ]─────────────────────────────┘
                                  (CDC & Pipeline)
                                        │
                                        ▼
                      ┌───────────────────────────────────┐
                      │           ALT SİSTEM 4:           │
                      │     300 DOWNSTREAM MCP SERVER     │
                      │         HİZMET FİLOSU             │
                      │ (Stdio / SSE / HTTP / Unix Socket)│
                      └───────────────────────────────────┘
```

---

## 2. ALT SİSTEM 1: Vektör Arama & Embedding Altyapısı (Qdrant & Model Hattı)

### 2.1. Qdrant HNSW İndeksleme ve Bellek Mühendisliği
10.000 aracın aranmasında geleneksel k-NN (Linear scan) kullanılamaz ($O(N)$ maliyetlidir). Qdrant'ın **HNSW (Hierarchical Navigable Small World)** graf algoritması kullanılır:

* **Graf Parametreleri:**
  * `m: 16` : Her düğümün (vektörün) diğer düğümlerle kuracağı maksimum kenar (bağlantı) sayısı. (Düşük bellek, yüksek arama hızı).
  * `ef_construct: 100` : Dizin oluşturulurken değerlendirilecek aday sayısı (Yüksek arama doğruluğu).
  * `ef_search: 64` : Sorgu anında graf üzerinde taranacak komşu sayısı (Sub-millisecond latency sağlar).
* **Mesafe Metriği:** `Cosine` (Normalize edilmiş birim vektörler üzerinde açısal benzerlik).
* **Quantization (Kuantizasyon - Bellek Tasarrufu):**
  * 10.000 tool ölçeğinde **Scalar Quantization (int8)** aktif edilir.
  * 32-bit float değerler 8-bit tam sayılara sıkıştırılır. Bellek tüketimi **%75 oranında azalır**, arama hızı **4 katına çıkar**, doğruluk kaybı <%1'dir.

### 2.2. Payload (Metadata) İndeksleme Mimarisi
Qdrant'ta her vektör bir **Payload (JSON verisi)** taşır. Arama motorunun grafı gezerken disk okuması yapmaması için şu alanlar bellekte indekslenir (Payload Schema):

```json
{
  "field_name": "allowed_roles",
  "field_schema": "keyword"
},
{
  "field_name": "domain",
  "field_schema": "keyword"
},
{
  "field_name": "server_id",
  "field_schema": "keyword"
},
{
  "field_name": "is_active",
  "field_schema": "bool"
}
```

### 2.3. Embedding Pipeline (Yerel / On-Premises)
* **Model:** `BAAI/bge-small-en-v1.5` veya çok dilli `bge-m3`.
* **Vektör Boyutu:** 384 Boyut (Small) veya 1024 Boyut (M3).
* **Çıkarım Motoru:** Rust tabanlı **HuggingFace TEI (Text Embeddings Inference)** container'ı.
* **Girdi Formatı (Embedding Text):**
  ```
  Text = "Name: " + Tool.Name + " | Domain: " + Tool.Domain + " | Description: " + Tool.Description + " | Parameters: " + ExtractedParamKeys
  ```
  Sadece açıklamayı değil, parametre isimlerini de vektöre katarak fonksiyonun ne işe yaradığı tam yakalanır.

---

## 3. ALT SİSTEM 2: Dağıtık Cache, Olay Veriyolu & Rate-Limiter (Redis Cluster)

Gateway pod'ları durumsuzdur (stateless); tüm geçici durumlar Redis üzerinde yönetilir:

### 3.1. L1 Semantik Önbellek (Semantic Cache)
Aynı veya benzer sorular sorulduğunda Qdrant ve Embedding katmanını baypas eder:
* **Anahtar Yapısı:** `mcp:cache:query:<sha256(user_query + role)>`
* **Değer:** Top-K Tool ID'leri ve Şemaları (Gzip JSON).
* **TTL (Geçerlilik):** 15 Dakika.
* **Sonuç:** Cache hit durumunda gecikme **< 0.4 ms**.

### 3.2. Olay Veriyolu (Event Bus & Pub/Sub)
300 sunuculuk bir ortamda yeni bir tool eklendiğinde veya bir sunucu kapandığında tüm Gateway pod'larının hafızasını tazelemek için:
* **Kanal:** `mcp:catalog:events`
* **Mesaj Tipleri:**
  ```json
  {
    "event": "TOOL_UPDATED",
    "tool_id": "mcp-devops__k8s_list_pods",
    "timestamp": 1728751800
  }
  ```
* Bir pod bu mesajı aldığında, sadece o aracın yerel önbelleğini düşürür (`Cache Eviction`).

### 3.3. Dağıtık Rate-Limiting (Token Bucket)
Kullanıcıların veya ajanların downstream sistemleri çökertmesini engeller:
* **Anahtar:** `mcp:ratelimit:<user_id>:<tool_id>`
* **Algoritma:** Redis Lua Script üzerinde çalışan Token Bucket (örn: dakikada maks 60 çağrı).

---

## 4. ALT SİSTEM 3: Kalıcı Katalog, Yetkilendirme (RBAC) & Audit Trail (PostgreSQL 16)

PostgreSQL sistemin yasal uyumluluk, yedekleme ve değişmez kayıt defteridir (Source of Truth).

### 4.1. Veritabanı Tablo Mimarisi

```sql
-- 1. MCP Sunucu Sicili
CREATE TABLE mcp_servers (
    id VARCHAR(64) PRIMARY KEY,                 -- 'mcp-k8s-prod'
    name VARCHAR(255) NOT NULL,
    transport_type VARCHAR(16) NOT NULL,        -- 'stdio', 'sse', 'http', 'grpc'
    endpoint TEXT NOT NULL,                     -- URL veya subprocess komut yolu
    domain VARCHAR(64) NOT NULL,                -- 'devops', 'database', 'crm'
    health_status VARCHAR(16) DEFAULT 'healthy',-- 'healthy', 'degraded', 'offline'
    consecutive_failures INT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 2. 10.000 Araç Kataloğu
CREATE TABLE mcp_tools (
    id VARCHAR(128) PRIMARY KEY,                -- 'server_id__tool_name'
    server_id VARCHAR(64) REFERENCES mcp_servers(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    description TEXT NOT NULL,
    input_schema JSONB NOT NULL,                -- JSON-RPC inputSchema tanımı
    allowed_roles TEXT[] NOT NULL DEFAULT '{"admin"}',
    is_active BOOLEAN DEFAULT TRUE,
    version INT DEFAULT 1,
    vector_id UUID,                             -- Qdrant Point UUID karşılığı
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- 3. Denetim ve Güvenlik İzi (Audit Log - Günlük Bölümlemeli / Partitioned)
CREATE TABLE mcp_audit_logs (
    id BIGSERIAL,
    session_id VARCHAR(128) NOT NULL,
    user_id VARCHAR(64) NOT NULL,
    user_role VARCHAR(64) NOT NULL,
    tool_id VARCHAR(128) NOT NULL,
    arguments JSONB,                            -- Maskelenmiş girdi argümanları
    response_summary TEXT,                      -- Özet yanıt (KVKK gereği hassas veri kırpılır)
    execution_time_ms INT NOT NULL,
    status VARCHAR(16) NOT NULL,                -- 'success', 'error', 'timeout'
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Performans İndeksleri
CREATE INDEX idx_tools_search ON mcp_tools(server_id, is_active);
CREATE INDEX idx_tools_roles ON mcp_tools USING GIN(allowed_roles);
CREATE INDEX idx_audit_user_tool ON mcp_audit_logs(user_id, tool_id);
```

### 4.2. Asenkron Audit Pipeline (Sıfır İstemci Beklemesi)
* Gateway, tool çağrısı bittiğinde doğrudan PostgreSQL'e `INSERT` atmaz (bu durum istemciyi yavaşlatır).
* Gateway log nesnesini bellek içi bir Go kanalına (`chan AuditLog`) atar.
* Arka plandaki **Batch Worker**, her 500 ms'de veya 100 log biriktiğinde `pgx.CopyFrom` kullanarak toplu (batch) insert yapar. İstemci ek yükü: **0.00 ms**.

---

## 5. ALT SİSTEM 4: 300 Downstream MCP Sunucu Filosu Yönetimi

300 farklı MCP sunucusunu kontrolsüz bırakmak bağlantı sızıntılarına ve çöküşlere neden olur.

### 5.1. Hibrit Taşıma Katmanları (Transport Types)
1. **Yerel Process'ler (Stdio):**
   * Dosya sistemine, yerel CLI'lara veya hafif script'lere erişen sunucular (örn: Git, yerel log okuyucular).
   * Go'nun `os/exec` havuzunda `bufio.Reader` ve `io.WriteCloser` ile tek bir kalıcı process olarak çalıştırılır.
2. **Uzak Servisler (HTTP / SSE / gRPC):**
   * Kubernetes ortamında pod olarak koşan veya şirket ağına dağılmış mikroservisler.
   * `Keep-Alive: 90s` ile HTTP bağlantı havuzunda sıcak tutulur (TCP handshake maliyeti elenir).

### 5.2. Hata Toleransı & Devre Kesici (Circuit Breaker)
Her MCP sunucusu için bir Circuit Breaker örneği (`sony/gobreaker`) tanımlanır:
* **State: Closed (Normal):** İstekler sunucuya iletilir.
* **State: Open (Kesik):** Bir sunucu 3 ardışık timeout (örn: > 5 saniye) veya hata verirse devre açılır. Sonraki 30 saniye boyunca gelen istekler sunucuya gönderilmeden anında `"Sunucu geçici olarak devre dışı"` fallback yanıtı döner.
* **State: Half-Open:** 30 saniye sonra tek bir deneme isteği gönderilir; başarılıysa sistem normale döner.

### 5.3. Otomatik Sağlık Taraması (Health Checker & Heartbeat)
* Her 15 saniyede bir, arka planda tüm aktif sunuculara hafif bir JSON-RPC `ping` atılır.
* Yanıt vermeyen sunucunun durumu PostgreSQL'de `offline` yapılır ve Qdrant'ta `is_active: false` payload güncellemesi atılır.
* **Sonuç:** Model, çalışmayan bir aracı asla arama sonuçlarında görmez!

---

## 6. Veri Senkronizasyon Hattı (PostgreSQL -> Qdrant Pipeline)

PostgreSQL'e yeni bir araç eklendiğinde veya bir şema güncellendiğinde Qdrant nasıl senkronize edilir?

```
[Yönetici / CI-CD]
       │
       ▼ (1. SQL INSERT/UPDATE)
[PostgreSQL (mcp_tools)]
       │
       ▼ (2. Notify / CDC Trigger)
[Sync Worker Daemon]
       │
       ├─► (3. HF TEI'den 384-dim Embedding Üret)
       │
       ├─► (4. Qdrant /points Upsert Yap)
       │
       └─► (5. Redis Pub/Sub: mcp:catalog:events 'TOOL_UPDATED' Yayınla)
```

Bu boru hattı sayesinde sistemde **sıfır kesinti (Zero Downtime)** ile dinamik araç kataloğu yönetilir.

---

## 7. Güvenlik, İzolasyon & Maskeleme (Security & Governance)

1. **Hassas Veri Maskeleme (PII Redaction):**
   * Finans veya CRM araçları çalıştırıldığında (`stripe_get_invoice`, `salesforce_get_customer`), kredi kartı veya TC Kimlik gibi hassas alanlar regex filtresinden geçer ve `mcp_audit_logs` tablosuna `***MASKED***` olarak yazılır.
2. **Zero-Trust Tool Execution:**
   * Bir araç çalıştırılmadan önce Gateway 2 ayrı kontrol yapar:
     1. Kullanıcının JWT token'ındaki rolü `mcp_tools.allowed_roles` dizisinde var mı?
     2. Gelen parametreler JSON Schema'ya (`inputSchema`) birebir uyuyor mu? (Schema Injection engeli).

---

## 8. Altyapı Bileşenleri ve Port Matrisi

| Servis | Konteyner / Süreç | Port / Protokol | Görevi |
| :--- | :--- | :--- | :--- |
| **Qdrant** | `mcp-qdrant` | `6333 (HTTP)`, `6334 (gRPC)` | Vektör indeksi, HNSW benzerlik araması, Payload filtreleme |
| **Redis** | `mcp-redis` | `6379 (TCP)` | Semantik önbellek, Pub/Sub olayları, Rate limiting |
| **PostgreSQL** | `mcp-postgres` | `5432 (TCP)` | Sunucu kataloğu, Tool şemaları, Günlük partition'lı audit |
| **HuggingFace TEI**| `mcp-tei` | `8081 (HTTP)` | On-premise 384-boyutlu yerel embedding çıkarımı |
| **Downstream MCP** | `300 Servis Filosu`| `stdio` / `HTTP 808x` | Gerçek iş mantığını yürüten iş parçacıkları |

Bu teknik tasarım; Gateway'inizin arkasında duran tüm veri, önbellek, arama ve sunucu katmanlarının kurumsal ölçekte kesintisiz, güvenli ve milisaniye hızında çalışmasını garanti eder.
