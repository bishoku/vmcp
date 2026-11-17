# PostgreSQL obje envanteri

İnceleme tarihi: **17 Kasım 2026**. Kaynak commit: `5fb8adbfce4e299208541db2da0ed24bb4fd3d40`. Dosya adı kullanıcı talebiyle `posgresql.md` olarak korunmuştur.

## Kapsam ve doğrulama

Bu belge uygulamanın repodaki son halini tanımlar: `core/migrations/00001`–`00046` dosyalarının yalnızca **Up** bölümleri sırasıyla PostgreSQL 17 üzerinde boş, geçici bir veritabanına uygulanmıştır. Kolon, varsayılan, constraint, indeks, kullanıcı trigger ve fonksiyon tanımları PostgreSQL sistem kataloglarından alınmıştır. Canlı/kurulu uygulama veritabanı incelenmemiştir; ortamda elle eklenmiş nesneler kapsam dışıdır. Mevcut uygulama verisine erişilmemiştir. Geçici veritabanı Goose CLI yerine migration SQL’iyle kurulduğu için Goose yönetim nesneleri ayrı belirtilmiştir.

Yalnızca halen mevcut nesneler ana envantere dahildir. Down bölümlerinde yeniden oluşturulan eski alanlar güncel şema değildir. Kullanım dosyaları, testler hariç nesne adını içeren Go kaynaklarından bulunmuştur; SQL dışında ek davranışlar ilgili paketlerin kodundadır.

## Sayısal özet

| Nesne | Sayı |
| --- | --- |
| Uygulama tablosu | 47 |
| Kolon | 349 |
| PK | 47 |
| FK | 43 |
| UNIQUE constraint | 14 |
| CHECK constraint | 54 |
| İndeks (PK/UNIQUE destek indeksleri dahil) | 95 |
| Kullanıcı trigger’ı | 31 |
| public fonksiyonu | 11 |
| Uygulama sequence’i | 2 |
| İzin kataloğu kaydı | 106 |
| Yerleşik uygulama rolü | 8 |


## Tablo ve kullanım özeti

| Tablo | Kullanım amacı |
| --- | --- |
| `admin_tokens` | Control-plane API erişim tokenları; tokenın kendisi yerine hash ve gösterim öneki saklanır. |
| `admission_settings` | Toplam, sanal sunucu ve principal başına eşzamanlı kabul limitlerinin sürümlü tekil ayarı. |
| `agent_key_servers` | Agent anahtarının erişebildiği sanal sunucuların çoktan çoğa bağlantısı. |
| `agent_keys` | Data-plane istemci anahtarları, IP kısıtları, süre, iptal ve hız limiti. |
| `audit_access_contexts` | Audit olayının değiştirilemez tenant ve provenance sınıflandırması. |
| `audit_access_domains` | Audit olayına erişim için tüketici/sağlayıcı/hedef kaynaklarının olay anındaki sahiplik bağlamı. |
| `audit_deliveries` | Her export tüketicisi ve olay için kalıcı teslim kuyruğu, deneme, lease ve şifreli zarf. |
| `audit_events` | İstek, araç çağrısı ve kontrol değişikliklerinin kalıcı metadata kaydı; admission/outcome yaşam döngüsü ve konuşma korelasyonu. |
| `audit_export_state` | Audit export tüketicilerinin checkpoint, aktiflik ve payload dahil etme ayarı. |
| `audit_legal_holds` | Belirli audit kayıtlarının retention ile silinmesini engelleyen hukuki muhafaza işareti. |
| `audit_payloads` | Audit istek/yanıt içeriklerinin metadata’dan ayrı, süreli ve şifreli saklanması. |
| `audit_resource_epochs` | Kaynak sahipliğinin revision aralıkları; kaynak silinse bile geçmiş erişim kanıtını korur. |
| `audit_settings` | Audit yakalama, redaksiyon, saklama ve export politikasının sürümlü JSON ayarları. |
| `authorization_assignments` | Rolü doğrudan principal’a veya claim eşlemesine belirli kaynak/capture kapsamıyla atar. |
| `authorization_mappings` | OIDC claim koşullarını yetkilendirme atamalarına bağlayan kurallar. |
| `authorization_permissions` | Uygulama izin kataloğu; geçerli scope türleri ve payload capture kısıt bilgisi. |
| `authorization_role_permissions` | Uygulama rolleri ile izin kataloğu arasındaki çoktan çoğa ilişki. |
| `authorization_roles` | Yerleşik ve özel uygulama rolleri. PostgreSQL LOGIN/NOLOGIN rollerinden ayrıdır. |
| `authorization_state` | Yetkilendirme politikasının tekil revision sayacı; snapshot tazeliğini denetler. |
| `browser_sessions` | OIDC tarayıcı oturumları; issuer/subject, iptal, süre ve şifreli claim oturum modeli. |
| `config_revision` | Global yapılandırma revision sayacı; snapshot yenilenmesini ve bildirimleri tetikler. |
| `identity_binding_servers` | OIDC kimlik eşlemesinin erişebildiği sanal sunucular. |
| `identity_bindings` | OIDC claim koşullarına göre runtime kimlik erişimi, IP ve hız kısıtları. |
| `invocation_dispatches` | Operation ID tüketimini benzersizleştirerek replikalar arasında ikinci dispatch’i engeller. |
| `invocation_security` | Invocation güvenlik yapılandırmasının tekil, şifreli ve sürümlü kaydı. |
| `mcp_client_notifications` | MCP istemcisinden gelen roots değişikliği revision takibi. |
| `mcp_discovery_requests` | Upstream discovery yenileme işlerinin istek ve lease koordinasyonu. |
| `mcp_events` | Şifreli MCP stream olayları; sıra numarasıyla yeniden oynatma ve terminal sonuç teslimi. |
| `mcp_operations` | MCP isteklerinin kalıcı çalışma durumu, principal eşzamanlılığı, iptal, fencing ve lease kaydı. |
| `mcp_pending_requests` | Sunucudan istemciye giden MCP isteklerinin bekleyen yanıt ve süre kaydı. |
| `mcp_resource_subscriptions` | MCP kaynak aboneliklerinin upstream URI, owner, lease ve güncelleme takibi. |
| `mcp_streams` | MCP SSE/yanıt stream sahipliği, işlem bağlantısı, lease ve expiry. |
| `projects` | Sanal sunucuları gruplayan ve isteğe bağlı bir takıma ait olan projeler. |
| `revoked_jwts` | Hash üzerinden runtime JWT iptali; süresi dolan iptal kayıtları temizlenebilir. |
| `sessions` | MCP protokol oturumları, initialize durumu, yetenekler ve kimlik sahipliği. |
| `teams` | OIDC issuer/group çiftini sahiplik grubuna bağlayan takımlar. |
| `tenant_runtime_identity` | Bu veritabanının runtime tenant kimliği; audit kayıtlarına text kimlik sağlar. |
| `tenants` | Tenant UUID ve slug kataloğu; temel yapılandırma kayıtları buna bağlanır. |
| `tool_grants` | Sanal sunucunun belirli upstream aracını kullanma talebi ve onay/ret/iptal durumu. |
| `upstream_catalog` | Upstream kaynak/resource-template/prompt gibi katalog tanımlarının önbelleği. |
| `upstream_health` | Upstream için düğüm bazında sağlık, circuit breaker ve gecikme görünümü. |
| `upstream_status` | Upstream discovery yenilemesinin son zamanı ve hata bilgisi. |
| `upstream_tools` | Upstream araç adları, JSON input şeması, annotation ve tam tanım önbelleği. |
| `upstreams` | Fiziksel MCP/REST bağlantıları; transport, TLS/OAuth, şifreli sırlar, kimlik aktarımı ve sahiplik. |
| `virtual_catalog` | Sanal sunucuda yayımlanan upstream katalog öğeleri; alias, açıklama ve sıralama. |
| `virtual_server_tools` | Sanal sunucuda yayımlanan araçların allowlist’i, alias ve açıklama geçersiz kılmaları. |
| `virtual_servers` | İstemciye sunulan sanal MCP sunucuları; güvenlik, PII, araç politikası, istemci özellikleri ve proje. |


## Tasarım ve ilişki notları

- `tenants.id` UUID’dir; `tenant_runtime_identity.tenant_id` ve audit tenant alanları text kimliktir. Aralarında otomatik UUID FK ilişkisi yoktur; bu iki alanı eşdeğer varsaymayın.
- `config_revision` yapılandırma snapshot’ını, `authorization_state.revision` izin snapshot’ını izler. Revision trigger’ları statement düzeyindedir; satır sayısı başına artış varsayılmamalıdır.
- `virtual_server_tools` güncel şemada araç adıyla `upstream_tools` tablosuna composite FK içermez; upstream silinince temizlenir, discovery sırasında kaybolan araç kaydı yayın bağını otomatik düşürmez.
- Audit geçmişindeki kaynak/proje/takım kimlikleri çoğunlukla canlı kaynağa FK değildir. Kaynağın silinmesi veya taşınması geçmiş kanıtı silmesin diye epoch ve immutable context kullanılır.
- FK `ON DELETE` davranışları aşağıda PostgreSQL’in döndürdüğü biçimde verilmiştir; yazılmamış davranış `NO ACTION` varsayılanıdır. FK kolonlarına ayrıca indeks otomatik eklenmez; gerçek indeksler ayrı listelenmiştir.
- `updated_at`/`version` alanlarının bulunması otomatik timestamp/sürüm trigger’ı olduğu anlamına gelmez; uygulama SQL’i bunları günceller.
- `CHECK` yoksa enum benzeri text alanın değerlerinin PostgreSQL tarafından sınırlandığı varsayılmamalıdır. JSONB iç yapıları da yalnızca açık constraint varsa veritabanı düzeyinde denetlenir.

## Tablo ayrıntıları

Kolon tablosunda “Zorunlu” `NOT NULL` bilgisidir. `audit_events.id` özel olarak `GENERATED ALWAYS AS IDENTITY` alanıdır; klasik DEFAULT ifadesi olmadan `audit_events_id_seq` üzerinden üretilir. `—` varsayılan olmadığını gösterir. PK/FK/UNIQUE/CHECK tanımları her tablonun altında tam olarak listelenir.

### `admin_tokens`

Control-plane API erişim tokenları; tokenın kendisi yerine hash ve gösterim öneki saklanır.

**Uygulama kullanımı:** `core/internal/store/authorization_assignments.go`, `core/internal/store/store.go`, `core/internal/store/authorization_scopes.go`, `core/internal/store/admintokens.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/configuration_check.go`

**Migration kaynakları:** `core/migrations/00007_admin_tokens.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `tenant_id` | `uuid` | Evet | — | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |
| `slug` | `text` | Evet | — | İnsan tarafından okunabilir kısa kimlik |
| `description` | `text` | Evet | `''::text` | Açıklama metni |
| `scope` | `text` | Evet | — | Token erişim kapsamı veya audit yakalama kapsamı |
| `token_hash` | `bytea` | Evet | — | Token/JWT hash’i; açık token değildir |
| `token_prefix` | `text` | Evet | — | Gösterim için token öneki |
| `expires_at` | `timestamp with time zone` | Hayır | — | Geçerlilik/saklama süresinin sonu |
| `revoked_at` | `timestamp with time zone` | Hayır | — | İptal zamanı |
| `last_used_at` | `timestamp with time zone` | Hayır | — | Son kullanım zamanı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |
| `updated_at` | `timestamp with time zone` | Evet | `now()` | Son güncelleme zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `admin_tokens_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `admin_tokens_scope_check` | CHECK | `CHECK (scope = ANY (ARRAY['admin'::text, 'read'::text]))` |
| `admin_tokens_tenant_id_fkey` | FOREIGN KEY | `FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE` |
| `admin_tokens_tenant_id_slug_key` | UNIQUE | `UNIQUE (tenant_id, slug)` |
| `admin_tokens_token_hash_key` | UNIQUE | `UNIQUE (token_hash)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `admin_tokens_pkey` | `CREATE UNIQUE INDEX admin_tokens_pkey ON public.admin_tokens USING btree (id)` |
| `admin_tokens_tenant_id_slug_key` | `CREATE UNIQUE INDEX admin_tokens_tenant_id_slug_key ON public.admin_tokens USING btree (tenant_id, slug)` |
| `admin_tokens_token_hash_key` | `CREATE UNIQUE INDEX admin_tokens_token_hash_key ON public.admin_tokens USING btree (token_hash)` |


**Trigger’lar:** Yok.

### `admission_settings`

Toplam, sanal sunucu ve principal başına eşzamanlı kabul limitlerinin sürümlü tekil ayarı.

**Uygulama kullanımı:** `core/internal/store/admission_settings.go`

**Migration kaynakları:** `core/migrations/00046_admission_settings.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `singleton` | `boolean` | Evet | `true` | Yalnızca true olabilen tekil ayar anahtarı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |
| `settings` | `jsonb` | Evet | — | İlgili özelliğin sürümlü JSON ayarları |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `admission_settings_pkey` | PRIMARY KEY | `PRIMARY KEY (singleton)` |
| `admission_settings_singleton_check` | CHECK | `CHECK (singleton)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `admission_settings_pkey` | `CREATE UNIQUE INDEX admission_settings_pkey ON public.admission_settings USING btree (singleton)` |


**Trigger’lar:** `admission_settings_revision`

### `agent_key_servers`

Agent anahtarının erişebildiği sanal sunucuların çoktan çoğa bağlantısı.

**Uygulama kullanımı:** `core/internal/store/ownership.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/configuration_check.go`, `core/internal/store/agentkeys.go`, `core/internal/store/snapshot.go`

**Migration kaynakları:** `core/migrations/00004_agent_keys.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `agent_key_id` | `uuid` | Evet | — | Agent anahtar kimliği |
| `virtual_server_id` | `uuid` | Evet | — | İlgili sanal sunucu kimliği |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `agent_key_servers_agent_key_id_fkey` | FOREIGN KEY | `FOREIGN KEY (agent_key_id) REFERENCES agent_keys(id) ON DELETE CASCADE` |
| `agent_key_servers_pkey` | PRIMARY KEY | `PRIMARY KEY (agent_key_id, virtual_server_id)` |
| `agent_key_servers_virtual_server_id_fkey` | FOREIGN KEY | `FOREIGN KEY (virtual_server_id) REFERENCES virtual_servers(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `agent_key_servers_pkey` | `CREATE UNIQUE INDEX agent_key_servers_pkey ON public.agent_key_servers USING btree (agent_key_id, virtual_server_id)` |


**Trigger’lar:** `agent_key_servers_bump`

### `agent_keys`

Data-plane istemci anahtarları, IP kısıtları, süre, iptal ve hız limiti.

**Uygulama kullanımı:** `core/internal/controlplane/configuration_authorization.go`, `core/internal/authz/catalog.go`, `core/internal/store/ownership.go`, `core/internal/store/store.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/configuration_check.go`, `core/internal/store/agentkeys.go`, `core/internal/store/snapshot.go`

**Migration kaynakları:** `core/migrations/00004_agent_keys.sql`, `core/migrations/00011_rate_limits.sql`, `core/migrations/00035_audit_conversations.sql`, `core/migrations/00036_control_plane_authorization.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `tenant_id` | `uuid` | Evet | — | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |
| `slug` | `text` | Evet | — | İnsan tarafından okunabilir kısa kimlik |
| `description` | `text` | Evet | `''::text` | Açıklama metni |
| `key_hash` | `bytea` | Evet | — | Agent anahtarının doğrulama hash’i |
| `key_prefix` | `text` | Evet | — | Arayüzde gösterilen anahtar öneki |
| `ip_allowlist` | `text[]` | Evet | `'{}'::text[]` | İzin verilen istemci IP/CIDR listesi |
| `expires_at` | `timestamp with time zone` | Hayır | — | Geçerlilik/saklama süresinin sonu |
| `revoked_at` | `timestamp with time zone` | Hayır | — | İptal zamanı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |
| `updated_at` | `timestamp with time zone` | Evet | `now()` | Son güncelleme zamanı |
| `rate_limit_per_minute` | `integer` | Hayır | — | Dakikalık istek sınırı; NULL sınırsızdır, dolu değer CHECK gereği pozitiftir |
| `audit_correlation` | `jsonb` | Evet | `'{}'::jsonb` | Anahtar için audit konuşma korelasyon ayarı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `agent_keys_key_hash_key` | UNIQUE | `UNIQUE (key_hash)` |
| `agent_keys_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `agent_keys_rate_limit_per_minute_check` | CHECK | `CHECK (rate_limit_per_minute > 0)` |
| `agent_keys_tenant_id_fkey` | FOREIGN KEY | `FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE` |
| `agent_keys_tenant_id_slug_key` | UNIQUE | `UNIQUE (tenant_id, slug)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `agent_keys_key_hash_key` | `CREATE UNIQUE INDEX agent_keys_key_hash_key ON public.agent_keys USING btree (key_hash)` |
| `agent_keys_pkey` | `CREATE UNIQUE INDEX agent_keys_pkey ON public.agent_keys USING btree (id)` |
| `agent_keys_tenant_id_slug_key` | `CREATE UNIQUE INDEX agent_keys_tenant_id_slug_key ON public.agent_keys USING btree (tenant_id, slug)` |


**Trigger’lar:** `agent_keys_bump`

### `audit_access_contexts`

Audit olayının değiştirilemez tenant ve provenance sınıflandırması.

**Uygulama kullanımı:** `core/internal/audit/durable.go`, `core/internal/audit/access.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/audit_access.go`, `core/internal/store/audit_policy.go`

**Migration kaynakları:** `core/migrations/00036_control_plane_authorization.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `event_id` | `uuid` | Evet | — | Audit olay UUID’si; delivery/hold tablolarında bigint audit satır kimliği olabilir |
| `tenant_id` | `text` | Evet | `vmcp_tenant_id()` | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |
| `context_version` | `integer` | Evet | `1` | Audit erişim bağlamının format sürümü |
| `classification` | `text` | Evet | — | Audit erişim sınıfı: resource/tenant/legacy |
| `origin` | `text` | Evet | — | Audit olayının kaynak bileşeni |
| `operation_config_revision` | `bigint` | Evet | — | İşlemin çalıştığı config revision |
| `provenance_state` | `text` | Evet | — | Kaynak/sahiplik kanıtının verified/legacy durumu |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `audit_access_contexts_check` | CHECK | `CHECK ((classification = 'legacy'::text) = (provenance_state = 'legacy'::text))` |
| `audit_access_contexts_classification_check` | CHECK | `CHECK (classification = ANY (ARRAY['resource'::text, 'tenant'::text, 'legacy'::text]))` |
| `audit_access_contexts_context_version_check` | CHECK | `CHECK (context_version = 1)` |
| `audit_access_contexts_event_id_fkey` | FOREIGN KEY | `FOREIGN KEY (event_id) REFERENCES audit_events(event_id) ON DELETE CASCADE` |
| `audit_access_contexts_operation_config_revision_check` | CHECK | `CHECK (operation_config_revision >= 0)` |
| `audit_access_contexts_pkey` | PRIMARY KEY | `PRIMARY KEY (event_id)` |
| `audit_access_contexts_provenance_state_check` | CHECK | `CHECK (provenance_state = ANY (ARRAY['verified'::text, 'legacy'::text]))` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `audit_access_contexts_pkey` | `CREATE UNIQUE INDEX audit_access_contexts_pkey ON public.audit_access_contexts USING btree (event_id)` |


**Trigger’lar:** `audit_access_contexts_immutable`, `audit_access_contexts_validate`

### `audit_access_domains`

Audit olayına erişim için tüketici/sağlayıcı/hedef kaynaklarının olay anındaki sahiplik bağlamı.

**Uygulama kullanımı:** `core/internal/audit/durable.go`, `core/internal/audit/access.go`, `core/internal/store/audit_access.go`, `core/internal/store/audit_policy.go`

**Migration kaynakları:** `core/migrations/00036_control_plane_authorization.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `event_id` | `uuid` | Evet | — | Audit olay UUID’si; delivery/hold tablolarında bigint audit satır kimliği olabilir |
| `domain_kind` | `text` | Evet | — | Erişim alanının consumer/provider/target rolü |
| `resource_kind` | `text` | Evet | — | İlgili kaynağın türü |
| `resource_id` | `uuid` | Evet | — | Kaynağın kalıcı UUID’si |
| `project_id` | `uuid` | Hayır | — | Proje kimliği veya geçmiş proje bağlamı |
| `owner_team_id` | `uuid` | Hayır | — | Kaynak sahibi takım kimliği veya geçmiş sahiplik bağlamı |
| `epoch_id` | `uuid` | Evet | — | Geçmiş sahiplik döneminin UUID’si |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `audit_access_domains_domain_kind_check` | CHECK | `CHECK (domain_kind = ANY (ARRAY['consumer'::text, 'provider'::text, 'target'::text]))` |
| `audit_access_domains_epoch_id_fkey` | FOREIGN KEY | `FOREIGN KEY (epoch_id) REFERENCES audit_resource_epochs(epoch_id) ON DELETE RESTRICT` |
| `audit_access_domains_event_id_fkey` | FOREIGN KEY | `FOREIGN KEY (event_id) REFERENCES audit_access_contexts(event_id) ON DELETE CASCADE` |
| `audit_access_domains_pkey` | PRIMARY KEY | `PRIMARY KEY (event_id, domain_kind, resource_kind, resource_id)` |
| `audit_access_domains_resource_kind_check` | CHECK | `CHECK (resource_kind = ANY (ARRAY['upstream'::text, 'virtual_server'::text]))` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `audit_access_domains_pkey` | `CREATE UNIQUE INDEX audit_access_domains_pkey ON public.audit_access_domains USING btree (event_id, domain_kind, resource_kind, resource_id)` |


**Trigger’lar:** `audit_access_domains_immutable`, `audit_access_domains_validate`

### `audit_deliveries`

Her export tüketicisi ve olay için kalıcı teslim kuyruğu, deneme, lease ve şifreli zarf.

**Uygulama kullanımı:** `core/internal/auditexport/metrics.go`, `core/internal/audit/audit.go`, `core/internal/audit/export.go`, `core/cmd/rotate-secrets/main.go`

**Migration kaynakları:** `core/migrations/00023_audit_export.sql`, `core/migrations/00029_audit_payloads.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `consumer` | `text` | Evet | — | Export tüketicisinin benzersiz adı |
| `event_id` | `uuid` | Evet | — | Audit olay UUID’si; delivery/hold tablolarında bigint audit satır kimliği olabilir |
| `state` | `text` | Evet | `'pending'::text` | İşlem, teslim veya sağlık durum değeri |
| `attempts` | `integer` | Evet | `0` | Teslim denemesi sayısı |
| `next_attempt` | `timestamp with time zone` | Evet | `now()` | Bir sonraki deneme zamanı |
| `lease_until` | `timestamp with time zone` | Hayır | — | İş sahibi lease’inin sona erme zamanı |
| `lease_token` | `uuid` | Hayır | — | Teslim lease fencing tokenı |
| `delivered_at` | `timestamp with time zone` | Hayır | — | Başarılı teslim zamanı |
| `envelope` | `bytea` | Hayır | — | Yeniden denemeler için değişmeden korunan şifreli export zarfı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `audit_deliveries_consumer_fkey` | FOREIGN KEY | `FOREIGN KEY (consumer) REFERENCES audit_export_state(consumer)` |
| `audit_deliveries_event_id_fkey` | FOREIGN KEY | `FOREIGN KEY (event_id) REFERENCES audit_events(event_id) ON DELETE CASCADE` |
| `audit_deliveries_pkey` | PRIMARY KEY | `PRIMARY KEY (consumer, event_id)` |
| `audit_deliveries_state_check` | CHECK | `CHECK (state = ANY (ARRAY['pending'::text, 'leased'::text, 'delivered'::text, 'dead'::text]))` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `audit_deliveries_due` | `CREATE INDEX audit_deliveries_due ON public.audit_deliveries USING btree (consumer, next_attempt) WHERE (state = ANY (ARRAY['pending'::text, 'leased'::text]))` |
| `audit_deliveries_pkey` | `CREATE UNIQUE INDEX audit_deliveries_pkey ON public.audit_deliveries USING btree (consumer, event_id)` |


**Trigger’lar:** Yok.

### `audit_events`

İstek, araç çağrısı ve kontrol değişikliklerinin kalıcı metadata kaydı; admission/outcome yaşam döngüsü ve konuşma korelasyonu.

**Uygulama kullanımı:** `core/internal/auditexport/metrics.go`, `core/internal/audit/audit.go`, `core/internal/audit/durable.go`, `core/internal/audit/export.go`, `core/internal/controlplane/audit_settings.go`, `core/internal/delivery/store.go`, `core/internal/store/audit.go`, `core/internal/store/store.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/audit_access.go`, `core/internal/store/audit_policy.go`

**Migration kaynakları:** `core/migrations/00005_audit_events.sql`, `core/migrations/00014_identity_bindings.sql`, `core/migrations/00018_durable_audit.sql`, `core/migrations/00023_audit_export.sql`, `core/migrations/00025_tenant_identity.sql`, `core/migrations/00029_audit_payloads.sql`, `core/migrations/00035_audit_conversations.sql`, `core/migrations/00036_control_plane_authorization.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `bigint` | Evet | — | Kaydın veya ilgili protokol nesnesinin kimliği |
| `ts` | `timestamp with time zone` | Evet | `now()` | Audit olay zamanı |
| `virtual_server` | `text` | Evet | — | Sanal sunucunun metinsel kimliği/slug bilgisi |
| `agent_key` | `text` | Hayır | — | Audit olayındaki agent anahtar etiketi |
| `client_ip` | `text` | Hayır | — | İstemci IP bilgisi |
| `method` | `text` | Evet | — | MCP/HTTP işlem metodu |
| `tool` | `text` | Hayır | — | Araç adı |
| `upstream` | `text` | Hayır | — | Audit olayındaki upstream etiketi |
| `status` | `text` | Evet | — | Sonuç veya grant durumu |
| `reason` | `text` | Hayır | — | Karar, hata veya hold gerekçesi |
| `duration_ms` | `integer` | Evet | `0` | İşlem süresi (ms) |
| `redactions` | `integer` | Evet | `0` | Audit metadata redaksiyon sayısı |
| `subject` | `text` | Hayır | — | OIDC subject bilgisi |
| `event_id` | `uuid` | Evet | `gen_random_uuid()` | Audit olay UUID’si; delivery/hold tablolarında bigint audit satır kimliği olabilir |
| `operation_id` | `uuid` | Hayır | — | İlgili işlem kimliği |
| `request_id` | `uuid` | Hayır | — | İstek UUID’si |
| `trace_id` | `text` | Evet | `''::text` | Dağıtık izleme kimliği |
| `schema_version` | `integer` | Evet | `1` | Audit olay şema sürümü |
| `actor_type` | `text` | Evet | `''::text` | İşlemi yapan kimliğin türü |
| `actor_id` | `text` | Evet | `''::text` | İşlemi yapan kimlik |
| `issuer` | `text` | Evet | `''::text` | OIDC kimlik sağlayıcısının issuer değeri |
| `action` | `text` | Evet | `''::text` | Audit işlem/eylem kodu |
| `target` | `text` | Evet | `''::text` | İşlemin hedefi |
| `config_revision` | `bigint` | Evet | `0` | Olay sırasında kullanılan global config revision |
| `stage` | `text` | Evet | `'event'::text` | Audit yaşam döngüsü aşaması |
| `tenant_id` | `text` | Evet | `vmcp_tenant_id()` | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |
| `audit_mode` | `text` | Evet | `'legacy'::text` | Olayın kaydedildiği audit modu |
| `policy_revision` | `bigint` | Evet | `0` | Uygulanan audit politika revision’ı |
| `scope` | `text` | Evet | `''::text` | Token erişim kapsamı veya audit yakalama kapsamı |
| `boundary` | `text` | Evet | `''::text` | Yakalamanın gerçekleştiği sistem sınırı |
| `direction` | `text` | Evet | `''::text` | Yakalanan verinin yönü |
| `parent_operation_id` | `uuid` | Hayır | — | Üst işlemin UUID’si |
| `payload_state` | `text` | Evet | `'disabled'::text` | Payload yakalama/saklama durumu |
| `payload_original_bytes` | `integer` | Evet | `0` | Redaksiyon öncesi payload boyutu |
| `payload_stored_bytes` | `integer` | Evet | `0` | Saklanan payload boyutu |
| `payload_redactions` | `integer` | Evet | `0` | Payload üzerinde yapılan redaksiyon sayısı |
| `conversation_id` | `text` | Evet | `''::text` | Konuşma kimliği |
| `root_conversation_id` | `text` | Evet | `''::text` | Kök konuşma kimliği |
| `turn_id` | `text` | Evet | `''::text` | Konuşma turu kimliği |
| `conversation_group_id` | `text` | Evet | `''::text` | Konuşmaları bağlayan grup kimliği |
| `correlation_status` | `text` | Evet | `''::text` | Konuşma korelasyonunun durumu |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `audit_events_lifecycle_check` | CHECK | `CHECK (((stage <> ALL (ARRAY['admission'::text, 'outcome'::text])) OR operation_id IS NOT NULL AND request_id IS NOT NULL) AND (stage <> 'admission'::text OR status = 'started'::text) AND (stage <> 'outcome'::text OR status <> 'started'::text) AND schema_version > 0)` |
| `audit_events_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `audit_events_stage_check` | CHECK | `CHECK (stage = ANY (ARRAY['event'::text, 'admission'::text, 'outcome'::text, 'mutation'::text]))` |
| `audit_events_status_check` | CHECK | `CHECK (status = ANY (ARRAY['started'::text, 'ok'::text, 'tool_error'::text, 'error'::text, 'denied'::text]))` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `audit_conversation_group_idx` | `CREATE INDEX audit_conversation_group_idx ON public.audit_events USING btree (tenant_id, conversation_group_id, id DESC) WHERE (conversation_group_id <> ''::text)` |
| `audit_conversation_id_idx` | `CREATE INDEX audit_conversation_id_idx ON public.audit_events USING btree (tenant_id, conversation_id, id DESC) WHERE (conversation_id <> ''::text)` |
| `audit_events_event_id_idx` | `CREATE UNIQUE INDEX audit_events_event_id_idx ON public.audit_events USING btree (event_id)` |
| `audit_events_key_idx` | `CREATE INDEX audit_events_key_idx ON public.audit_events USING btree (agent_key, id DESC)` |
| `audit_events_operation_stage_idx` | `CREATE UNIQUE INDEX audit_events_operation_stage_idx ON public.audit_events USING btree (operation_id, stage) WHERE (stage = ANY (ARRAY['admission'::text, 'outcome'::text]))` |
| `audit_events_parent_operation_idx` | `CREATE INDEX audit_events_parent_operation_idx ON public.audit_events USING btree (parent_operation_id) WHERE (parent_operation_id IS NOT NULL)` |
| `audit_events_pending_idx` | `CREATE INDEX audit_events_pending_idx ON public.audit_events USING btree (ts, operation_id) WHERE (stage = 'admission'::text)` |
| `audit_events_pkey` | `CREATE UNIQUE INDEX audit_events_pkey ON public.audit_events USING btree (id)` |
| `audit_events_request_id_idx` | `CREATE INDEX audit_events_request_id_idx ON public.audit_events USING btree (request_id)` |
| `audit_events_retention` | `CREATE INDEX audit_events_retention ON public.audit_events USING btree (ts, id)` |
| `audit_events_server_idx` | `CREATE INDEX audit_events_server_idx ON public.audit_events USING btree (virtual_server, id DESC)` |
| `audit_events_ts_idx` | `CREATE INDEX audit_events_ts_idx ON public.audit_events USING btree (ts DESC)` |
| `audit_root_conversation_id_idx` | `CREATE INDEX audit_root_conversation_id_idx ON public.audit_events USING btree (tenant_id, root_conversation_id, id DESC) WHERE (root_conversation_id <> ''::text)` |


**Trigger’lar:** Yok.

### `audit_export_state`

Audit export tüketicilerinin checkpoint, aktiflik ve payload dahil etme ayarı.

**Uygulama kullanımı:** `core/internal/audit/audit.go`, `core/internal/audit/export.go`, `core/internal/store/audit_settings.go`

**Migration kaynakları:** `core/migrations/00018_durable_audit.sql`, `core/migrations/00023_audit_export.sql`, `core/migrations/00029_audit_payloads.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `consumer` | `text` | Evet | — | Export tüketicisinin benzersiz adı |
| `last_event_id` | `bigint` | Evet | `0` | Tüketicinin işlediği son bigint audit satır kimliği |
| `updated_at` | `timestamp with time zone` | Evet | `now()` | Son güncelleme zamanı |
| `active` | `boolean` | Evet | `true` | Export tüketicisinin etkinliği |
| `include_payload` | `boolean` | Evet | `false` | Export zarfına payload dahil edilmesi |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `audit_export_state_pkey` | PRIMARY KEY | `PRIMARY KEY (consumer)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `audit_export_state_pkey` | `CREATE UNIQUE INDEX audit_export_state_pkey ON public.audit_export_state USING btree (consumer)` |


**Trigger’lar:** Yok.

### `audit_legal_holds`

Belirli audit kayıtlarının retention ile silinmesini engelleyen hukuki muhafaza işareti.

**Uygulama kullanımı:** `core/internal/audit/audit.go`

**Migration kaynakları:** `core/migrations/00023_audit_export.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `event_id` | `uuid` | Evet | — | Audit olay UUID’si; delivery/hold tablolarında bigint audit satır kimliği olabilir |
| `reason` | `text` | Evet | — | Karar, hata veya hold gerekçesi |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `audit_legal_holds_event_id_fkey` | FOREIGN KEY | `FOREIGN KEY (event_id) REFERENCES audit_events(event_id)` |
| `audit_legal_holds_pkey` | PRIMARY KEY | `PRIMARY KEY (event_id)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `audit_legal_holds_pkey` | `CREATE UNIQUE INDEX audit_legal_holds_pkey ON public.audit_legal_holds USING btree (event_id)` |


**Trigger’lar:** Yok.

### `audit_payloads`

Audit istek/yanıt içeriklerinin metadata’dan ayrı, süreli ve şifreli saklanması.

**Uygulama kullanımı:** `core/internal/audit/audit.go`, `core/internal/audit/durable.go`, `core/internal/audit/export.go`, `core/internal/controlplane/audit_settings.go`, `core/cmd/rotate-secrets/main.go`

**Migration kaynakları:** `core/migrations/00029_audit_payloads.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `event_id` | `uuid` | Evet | — | Audit olay UUID’si; delivery/hold tablolarında bigint audit satır kimliği olabilir |
| `tenant_id` | `text` | Evet | `vmcp_tenant_id()` | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |
| `ciphertext` | `bytea` | Evet | — | Şifreli içerik; açık veri değildir |
| `expires_at` | `timestamp with time zone` | Evet | — | Geçerlilik/saklama süresinin sonu |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `audit_payloads_event_id_fkey` | FOREIGN KEY | `FOREIGN KEY (event_id) REFERENCES audit_events(event_id) ON DELETE CASCADE` |
| `audit_payloads_pkey` | PRIMARY KEY | `PRIMARY KEY (event_id)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `audit_payloads_expiry_idx` | `CREATE INDEX audit_payloads_expiry_idx ON public.audit_payloads USING btree (expires_at)` |
| `audit_payloads_pkey` | `CREATE UNIQUE INDEX audit_payloads_pkey ON public.audit_payloads USING btree (event_id)` |


**Trigger’lar:** Yok.

### `audit_resource_epochs`

Kaynak sahipliğinin revision aralıkları; kaynak silinse bile geçmiş erişim kanıtını korur.

**Uygulama kullanımı:** `core/internal/store/authorization_assignments.go`, `core/internal/store/authorization_scopes.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/control_audit.go`, `core/internal/store/snapshot_audit.go`

**Migration kaynakları:** `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00038_snapshot_audit_provenance.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `epoch_id` | `uuid` | Evet | `gen_random_uuid()` | Geçmiş sahiplik döneminin UUID’si |
| `tenant_id` | `text` | Evet | `vmcp_tenant_id()` | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |
| `resource_kind` | `text` | Evet | — | İlgili kaynağın türü |
| `resource_id` | `uuid` | Evet | — | Kaynağın kalıcı UUID’si |
| `project_id` | `uuid` | Hayır | — | Proje kimliği veya geçmiş proje bağlamı |
| `owner_team_id` | `uuid` | Hayır | — | Kaynak sahibi takım kimliği veya geçmiş sahiplik bağlamı |
| `start_revision` | `bigint` | Evet | — | Sahiplik döneminin başlangıç revision’ı |
| `end_revision` | `bigint` | Hayır | — | Dönemin bitiş revision’ı; NULL açık dönemdir |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `audit_resource_epochs_check` | CHECK | `CHECK (end_revision IS NULL OR end_revision >= start_revision)` |
| `audit_resource_epochs_pkey` | PRIMARY KEY | `PRIMARY KEY (epoch_id)` |
| `audit_resource_epochs_resource_kind_check` | CHECK | `CHECK (resource_kind = ANY (ARRAY['upstream'::text, 'virtual_server'::text]))` |
| `audit_resource_epochs_start_revision_check` | CHECK | `CHECK (start_revision > 0)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `audit_resource_epochs_open` | `CREATE UNIQUE INDEX audit_resource_epochs_open ON public.audit_resource_epochs USING btree (tenant_id, resource_kind, resource_id) WHERE (end_revision IS NULL)` |
| `audit_resource_epochs_pkey` | `CREATE UNIQUE INDEX audit_resource_epochs_pkey ON public.audit_resource_epochs USING btree (epoch_id)` |


**Trigger’lar:** `audit_resource_epochs_protect`

### `audit_settings`

Audit yakalama, redaksiyon, saklama ve export politikasının sürümlü JSON ayarları.

**Uygulama kullanımı:** `core/internal/audit/export.go`, `core/internal/store/audit_settings.go`

**Migration kaynakları:** `core/migrations/00029_audit_payloads.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `singleton` | `boolean` | Evet | `true` | Yalnızca true olabilen tekil ayar anahtarı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |
| `settings` | `jsonb` | Evet | `'{}'::jsonb` | İlgili özelliğin sürümlü JSON ayarları |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `audit_settings_pkey` | PRIMARY KEY | `PRIMARY KEY (singleton)` |
| `audit_settings_singleton_check` | CHECK | `CHECK (singleton)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `audit_settings_pkey` | `CREATE UNIQUE INDEX audit_settings_pkey ON public.audit_settings USING btree (singleton)` |


**Trigger’lar:** `audit_settings_bump`

### `authorization_assignments`

Rolü doğrudan principal’a veya claim eşlemesine belirli kaynak/capture kapsamıyla atar.

**Uygulama kullanımı:** `core/internal/store/authorization_roles.go`, `core/internal/store/authorization_assignments.go`, `core/internal/store/authorization_mappings.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/configuration_check.go`, `core/internal/store/authorization.go`

**Migration kaynakları:** `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00038_snapshot_audit_provenance.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `role_id` | `uuid` | Evet | — | Uygulama rolü kimliği |
| `principal_kind` | `text` | Hayır | — | Atanan kimliğin OIDC/control token türü |
| `issuer` | `text` | Evet | `''::text` | OIDC kimlik sağlayıcısının issuer değeri |
| `principal_id` | `text` | Hayır | — | Doğrudan rol atanan principal kimliği |
| `mapping_id` | `uuid` | Hayır | — | Claim eşlemesi üzerinden atama bağlantısı |
| `scope_kind` | `text` | Evet | — | Atamanın tenant/team/project/kaynak kapsam türü |
| `scope_id` | `uuid` | Hayır | — | Kapsamın kaynak kimliği |
| `epoch_id` | `uuid` | Hayır | — | Geçmiş sahiplik döneminin UUID’si |
| `owner_team_id` | `uuid` | Hayır | — | Kaynak sahibi takım kimliği veya geçmiş sahiplik bağlamı |
| `capture_scopes` | `text[]` | Evet | `'{}'::text[]` | Payload erişimine izin verilen capture kapsamları |
| `enabled` | `boolean` | Evet | `true` | Kaydın kullanımda olup olmadığı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `authorization_assignments_capture_scopes_check` | CHECK | `CHECK (capture_scopes <@ ARRAY['virtual'::text, 'upstream'::text, 'control'::text, 'reads'::text, 'system'::text, 'omni'::text, 'omni_diagnostics'::text, 'omni_embedding'::text, 'omni_rerank'::text])` |
| `authorization_assignments_check` | CHECK | `CHECK (mapping_id IS NOT NULL AND principal_kind IS NULL AND principal_id IS NULL AND issuer = ''::text OR mapping_id IS NULL AND principal_kind IS NOT NULL AND principal_id IS NOT NULL AND principal_id <> ''::text AND (principal_kind = 'oidc'::text AND issuer <> ''::text OR principal_kind = 'control_token'::text AND issuer = ''::text))` |
| `authorization_assignments_check1` | CHECK | `CHECK ((scope_kind = ANY (ARRAY['tenant'::text, 'unmanaged'::text])) AND scope_id IS NULL AND epoch_id IS NULL AND owner_team_id IS NULL OR (scope_kind <> ALL (ARRAY['tenant'::text, 'unmanaged'::text])) AND scope_id IS NOT NULL)` |
| `authorization_assignments_mapping_id_fkey` | FOREIGN KEY | `FOREIGN KEY (mapping_id) REFERENCES authorization_mappings(id) ON DELETE RESTRICT` |
| `authorization_assignments_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `authorization_assignments_principal_kind_check` | CHECK | `CHECK (principal_kind = ANY (ARRAY['oidc'::text, 'control_token'::text]))` |
| `authorization_assignments_role_id_fkey` | FOREIGN KEY | `FOREIGN KEY (role_id) REFERENCES authorization_roles(id) ON DELETE RESTRICT` |
| `authorization_assignments_scope_kind_check` | CHECK | `CHECK (scope_kind = ANY (ARRAY['tenant'::text, 'unmanaged'::text, 'team'::text, 'project'::text, 'upstream'::text, 'virtual_server'::text]))` |
| `authorization_assignments_version_check` | CHECK | `CHECK (version > 0)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `authorization_assignments_mapping` | `CREATE INDEX authorization_assignments_mapping ON public.authorization_assignments USING btree (mapping_id) WHERE enabled` |
| `authorization_assignments_pkey` | `CREATE UNIQUE INDEX authorization_assignments_pkey ON public.authorization_assignments USING btree (id)` |
| `authorization_assignments_principal` | `CREATE INDEX authorization_assignments_principal ON public.authorization_assignments USING btree (principal_kind, issuer, principal_id) WHERE enabled` |


**Trigger’lar:** `authorization_assignments_bump`

### `authorization_mappings`

OIDC claim koşullarını yetkilendirme atamalarına bağlayan kurallar.

**Uygulama kullanımı:** `core/internal/store/oidc_claims.go`, `core/internal/store/authorization_assignments.go`, `core/internal/store/authorization_mappings.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/authorization.go`

**Migration kaynakları:** `core/migrations/00036_control_plane_authorization.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `issuer` | `text` | Evet | — | OIDC kimlik sağlayıcısının issuer değeri |
| `principal_kind` | `text` | Evet | — | Atanan kimliğin OIDC/control token türü |
| `conditions` | `jsonb` | Evet | — | OIDC claim eşleme koşulları |
| `enabled` | `boolean` | Evet | `true` | Kaydın kullanımda olup olmadığı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `authorization_mappings_conditions_check` | CHECK | `CHECK (jsonb_typeof(conditions) = 'array'::text AND jsonb_array_length(conditions) > 0)` |
| `authorization_mappings_issuer_check` | CHECK | `CHECK (issuer <> ''::text)` |
| `authorization_mappings_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `authorization_mappings_principal_kind_check` | CHECK | `CHECK (principal_kind = 'oidc'::text)` |
| `authorization_mappings_version_check` | CHECK | `CHECK (version > 0)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `authorization_mappings_pkey` | `CREATE UNIQUE INDEX authorization_mappings_pkey ON public.authorization_mappings USING btree (id)` |


**Trigger’lar:** `authorization_mappings_bump`

### `authorization_permissions`

Uygulama izin kataloğu; geçerli scope türleri ve payload capture kısıt bilgisi.

**Uygulama kullanımı:** Doğrudan nesne adıyla Go referansı bulunmadı; FK/trigger veya deployment üzerinden kullanılır.

**Migration kaynakları:** `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00044_invocation_permissions.sql`, `core/migrations/00045_upstream_invocation_security.sql`, `core/migrations/00046_admission_settings.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `text` | Evet | — | Kaydın veya ilgili protokol nesnesinin kimliği |
| `scopes` | `text[]` | Evet | — | İzin kodunun desteklediği scope türleri |
| `capture_restricted` | `boolean` | Evet | `false` | İznin payload capture kapsamıyla ayrıca kısıtlanması |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `authorization_permissions_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `authorization_permissions_scopes_check` | CHECK | `CHECK (cardinality(scopes) > 0)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `authorization_permissions_pkey` | `CREATE UNIQUE INDEX authorization_permissions_pkey ON public.authorization_permissions USING btree (id)` |


**Trigger’lar:** `authorization_permissions_bump`

### `authorization_role_permissions`

Uygulama rolleri ile izin kataloğu arasındaki çoktan çoğa ilişki.

**Uygulama kullanımı:** `core/internal/store/authorization_roles.go`, `core/internal/store/authorization_assignments.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/authorization.go`

**Migration kaynakları:** `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00041_builtin_workflow_dependencies.sql`, `core/migrations/00042_platform_operator_contract.sql`, `core/migrations/00044_invocation_permissions.sql`, `core/migrations/00045_upstream_invocation_security.sql`, `core/migrations/00046_admission_settings.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `role_id` | `uuid` | Evet | — | Uygulama rolü kimliği |
| `permission_id` | `text` | Evet | — | Uygulama izin kodu |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `authorization_role_permissions_permission_id_fkey` | FOREIGN KEY | `FOREIGN KEY (permission_id) REFERENCES authorization_permissions(id) ON DELETE RESTRICT` |
| `authorization_role_permissions_pkey` | PRIMARY KEY | `PRIMARY KEY (role_id, permission_id)` |
| `authorization_role_permissions_role_id_fkey` | FOREIGN KEY | `FOREIGN KEY (role_id) REFERENCES authorization_roles(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `authorization_role_permissions_pkey` | `CREATE UNIQUE INDEX authorization_role_permissions_pkey ON public.authorization_role_permissions USING btree (role_id, permission_id)` |


**Trigger’lar:** `authorization_role_permissions_bump`

### `authorization_roles`

Yerleşik ve özel uygulama rolleri. PostgreSQL LOGIN/NOLOGIN rollerinden ayrıdır.

**Uygulama kullanımı:** `core/internal/store/authorization_roles.go`, `core/internal/store/authorization_assignments.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/authorization.go`

**Migration kaynakları:** `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00041_builtin_workflow_dependencies.sql`, `core/migrations/00042_platform_operator_contract.sql`, `core/migrations/00044_invocation_permissions.sql`, `core/migrations/00045_upstream_invocation_security.sql`, `core/migrations/00046_admission_settings.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `slug` | `text` | Evet | — | İnsan tarafından okunabilir kısa kimlik |
| `name` | `text` | Evet | — | Görünen ad veya katalog öğesinin adı |
| `description` | `text` | Evet | `''::text` | Açıklama metni |
| `builtin` | `boolean` | Evet | `false` | Yerleşik rol olup olmadığı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `authorization_roles_name_check` | CHECK | `CHECK (name <> ''::text)` |
| `authorization_roles_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `authorization_roles_slug_check` | CHECK | `CHECK (slug <> ''::text)` |
| `authorization_roles_slug_key` | UNIQUE | `UNIQUE (slug)` |
| `authorization_roles_version_check` | CHECK | `CHECK (version > 0)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `authorization_roles_pkey` | `CREATE UNIQUE INDEX authorization_roles_pkey ON public.authorization_roles USING btree (id)` |
| `authorization_roles_slug_key` | `CREATE UNIQUE INDEX authorization_roles_slug_key ON public.authorization_roles USING btree (slug)` |


**Trigger’lar:** `authorization_roles_bump`

### `authorization_state`

Yetkilendirme politikasının tekil revision sayacı; snapshot tazeliğini denetler.

**Uygulama kullanımı:** `core/internal/store/authorization_roles.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/authorization.go`

**Migration kaynakları:** `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00037_audit_payload_authorization.sql`, `core/migrations/00040_enforced_authorization.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `singleton` | `boolean` | Evet | `true` | Yalnızca true olabilen tekil ayar anahtarı |
| `revision` | `bigint` | Evet | `1` | Değişiklik/snapshot revision sayacı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `authorization_state_pkey` | PRIMARY KEY | `PRIMARY KEY (singleton)` |
| `authorization_state_revision_check` | CHECK | `CHECK (revision > 0)` |
| `authorization_state_singleton_check` | CHECK | `CHECK (singleton)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `authorization_state_pkey` | `CREATE UNIQUE INDEX authorization_state_pkey ON public.authorization_state USING btree (singleton)` |


**Trigger’lar:** Yok.

### `browser_sessions`

OIDC tarayıcı oturumları; issuer/subject, iptal, süre ve şifreli claim oturum modeli.

**Uygulama kullanımı:** `core/internal/oidc/sessions.go`, `core/internal/store/store.go`, `core/internal/store/authorization_inspection.go`

**Migration kaynakları:** `core/migrations/00022_identity_revocation.sql`, `core/migrations/00039_oidc_claim_sessions.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `text` | Evet | — | Kaydın veya ilgili protokol nesnesinin kimliği |
| `subject` | `text` | Evet | — | OIDC subject bilgisi |
| `expires_at` | `timestamp with time zone` | Evet | — | Geçerlilik/saklama süresinin sonu |
| `revoked_at` | `timestamp with time zone` | Hayır | — | İptal zamanı |
| `issuer` | `text` | Evet | `''::text` | OIDC kimlik sağlayıcısının issuer değeri |
| `model_version` | `integer` | Evet | `0` | Tarayıcı oturum şifreli claim model sürümü |
| `session_ciphertext` | `bytea` | Hayır | — | Şifreli OIDC oturum/claim içeriği |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `browser_session_model` | CHECK | `CHECK (model_version = 0 AND session_ciphertext IS NULL OR model_version = 1 AND issuer <> ''::text AND session_ciphertext IS NOT NULL)` |
| `browser_sessions_model_version_check` | CHECK | `CHECK (model_version = ANY (ARRAY[0, 1]))` |
| `browser_sessions_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `browser_sessions_expiry` | `CREATE INDEX browser_sessions_expiry ON public.browser_sessions USING btree (expires_at)` |
| `browser_sessions_issuer_subject` | `CREATE INDEX browser_sessions_issuer_subject ON public.browser_sessions USING btree (issuer, subject)` |
| `browser_sessions_pkey` | `CREATE UNIQUE INDEX browser_sessions_pkey ON public.browser_sessions USING btree (id)` |
| `browser_sessions_subject` | `CREATE INDEX browser_sessions_subject ON public.browser_sessions USING btree (subject)` |


**Trigger’lar:** Yok.

### `config_revision`

Global yapılandırma revision sayacı; snapshot yenilenmesini ve bildirimleri tetikler.

**Uygulama kullanımı:** `core/internal/audit/durable.go`, `core/internal/controlplane/mock_upstreams.go`, `core/internal/delivery/store.go`, `core/internal/store/audit.go`, `core/internal/store/control_audit.go`, `core/internal/store/snapshot.go`, `core/internal/store/tx.go`

**Migration kaynakları:** `core/migrations/00001_init.sql`, `core/migrations/00002_revision_triggers.sql`, `core/migrations/00018_durable_audit.sql`, `core/migrations/00038_snapshot_audit_provenance.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `boolean` | Evet | `true` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `revision` | `bigint` | Evet | `1` | Değişiklik/snapshot revision sayacı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `config_revision_id_check` | CHECK | `CHECK (id)` |
| `config_revision_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `config_revision_pkey` | `CREATE UNIQUE INDEX config_revision_pkey ON public.config_revision USING btree (id)` |


**Trigger’lar:** Yok.

### `identity_binding_servers`

OIDC kimlik eşlemesinin erişebildiği sanal sunucular.

**Uygulama kullanımı:** `core/internal/store/ownership.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/bindings.go`, `core/internal/store/configuration_check.go`, `core/internal/store/snapshot.go`

**Migration kaynakları:** `core/migrations/00014_identity_bindings.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `binding_id` | `uuid` | Evet | — | Identity binding kimliği |
| `virtual_server_id` | `uuid` | Evet | — | İlgili sanal sunucu kimliği |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `identity_binding_servers_binding_id_fkey` | FOREIGN KEY | `FOREIGN KEY (binding_id) REFERENCES identity_bindings(id) ON DELETE CASCADE` |
| `identity_binding_servers_pkey` | PRIMARY KEY | `PRIMARY KEY (binding_id, virtual_server_id)` |
| `identity_binding_servers_virtual_server_id_fkey` | FOREIGN KEY | `FOREIGN KEY (virtual_server_id) REFERENCES virtual_servers(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `identity_binding_servers_pkey` | `CREATE UNIQUE INDEX identity_binding_servers_pkey ON public.identity_binding_servers USING btree (binding_id, virtual_server_id)` |


**Trigger’lar:** `identity_binding_servers_bump`

### `identity_bindings`

OIDC claim koşullarına göre runtime kimlik erişimi, IP ve hız kısıtları.

**Uygulama kullanımı:** `core/internal/controlplane/configuration_authorization.go`, `core/internal/authz/catalog.go`, `core/internal/store/ownership.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/bindings.go`, `core/internal/store/configuration_check.go`, `core/internal/store/snapshot.go`

**Migration kaynakları:** `core/migrations/00014_identity_bindings.sql`, `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00041_builtin_workflow_dependencies.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `tenant_id` | `uuid` | Evet | — | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |
| `slug` | `text` | Evet | — | İnsan tarafından okunabilir kısa kimlik |
| `description` | `text` | Evet | `''::text` | Açıklama metni |
| `conditions` | `jsonb` | Evet | — | OIDC claim eşleme koşulları |
| `ip_allowlist` | `text[]` | Evet | `'{}'::text[]` | İzin verilen istemci IP/CIDR listesi |
| `rate_limit_per_minute` | `integer` | Hayır | — | Dakikalık istek sınırı; NULL sınırsızdır, dolu değer CHECK gereği pozitiftir |
| `enabled` | `boolean` | Evet | `true` | Kaydın kullanımda olup olmadığı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |
| `updated_at` | `timestamp with time zone` | Evet | `now()` | Son güncelleme zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `identity_bindings_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `identity_bindings_rate_limit_per_minute_check` | CHECK | `CHECK (rate_limit_per_minute > 0)` |
| `identity_bindings_tenant_id_fkey` | FOREIGN KEY | `FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE` |
| `identity_bindings_tenant_id_slug_key` | UNIQUE | `UNIQUE (tenant_id, slug)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `identity_bindings_pkey` | `CREATE UNIQUE INDEX identity_bindings_pkey ON public.identity_bindings USING btree (id)` |
| `identity_bindings_tenant_id_slug_key` | `CREATE UNIQUE INDEX identity_bindings_tenant_id_slug_key ON public.identity_bindings USING btree (tenant_id, slug)` |


**Trigger’lar:** `identity_bindings_bump`

### `invocation_dispatches`

Operation ID tüketimini benzersizleştirerek replikalar arasında ikinci dispatch’i engeller.

**Uygulama kullanımı:** `core/cmd/data-plane/main.go`

**Migration kaynakları:** `core/migrations/00043_invocation_security.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `operation_id` | `text` | Evet | — | İlgili işlem kimliği |
| `binding_hash` | `text` | Evet | — | Operation ID’nin bağlandığı çağrı bağlamının hash’i |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `invocation_dispatches_pkey` | PRIMARY KEY | `PRIMARY KEY (operation_id)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `invocation_dispatches_pkey` | `CREATE UNIQUE INDEX invocation_dispatches_pkey ON public.invocation_dispatches USING btree (operation_id)` |


**Trigger’lar:** Yok.

### `invocation_security`

Invocation güvenlik yapılandırmasının tekil, şifreli ve sürümlü kaydı.

**Uygulama kullanımı:** `core/internal/controlplane/configuration_authorization.go`, `core/internal/authz/catalog.go`, `core/internal/store/invocation_security.go`, `core/internal/store/configuration_check.go`, `core/cmd/rotate-secrets/main.go`

**Migration kaynakları:** `core/migrations/00043_invocation_security.sql`, `core/migrations/00044_invocation_permissions.sql`, `core/migrations/00045_upstream_invocation_security.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `singleton` | `boolean` | Evet | `true` | Yalnızca true olabilen tekil ayar anahtarı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |
| `ciphertext` | `bytea` | Hayır | — | Şifreli içerik; açık veri değildir |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `invocation_security_pkey` | PRIMARY KEY | `PRIMARY KEY (singleton)` |
| `invocation_security_singleton_check` | CHECK | `CHECK (singleton)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `invocation_security_pkey` | `CREATE UNIQUE INDEX invocation_security_pkey ON public.invocation_security USING btree (singleton)` |


**Trigger’lar:** `invocation_security_revision`

### `mcp_client_notifications`

MCP istemcisinden gelen roots değişikliği revision takibi.

**Uygulama kullanımı:** `core/internal/virtual/streams.go`, `core/internal/virtual/roots.go`

**Migration kaynakları:** `core/migrations/00021_client_features.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `session_id` | `text` | Evet | — | MCP protokol oturum kimliği |
| `roots_revision` | `bigint` | Evet | `0` | İstemcinin roots değişiklik revision’ı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `mcp_client_notifications_pkey` | PRIMARY KEY | `PRIMARY KEY (session_id)` |
| `mcp_client_notifications_session_id_fkey` | FOREIGN KEY | `FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `mcp_client_notifications_pkey` | `CREATE UNIQUE INDEX mcp_client_notifications_pkey ON public.mcp_client_notifications USING btree (session_id)` |


**Trigger’lar:** Yok.

### `mcp_discovery_requests`

Upstream discovery yenileme işlerinin istek ve lease koordinasyonu.

**Uygulama kullanımı:** `core/internal/controlplane/discovery.go`, `core/internal/virtual/streams.go`

**Migration kaynakları:** `core/migrations/00019_mcp_delivery.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `upstream_id` | `uuid` | Evet | — | İlgili fiziksel upstream kimliği |
| `requested_at` | `timestamp with time zone` | Evet | `now()` | Discovery yenileme isteği zamanı |
| `lease_owner` | `uuid` | Hayır | — | Discovery işinin sahibi düğüm |
| `lease_until` | `timestamp with time zone` | Hayır | — | İş sahibi lease’inin sona erme zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `mcp_discovery_requests_pkey` | PRIMARY KEY | `PRIMARY KEY (upstream_id)` |
| `mcp_discovery_requests_upstream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `mcp_discovery_requests_pkey` | `CREATE UNIQUE INDEX mcp_discovery_requests_pkey ON public.mcp_discovery_requests USING btree (upstream_id)` |


**Trigger’lar:** Yok.

### `mcp_events`

Şifreli MCP stream olayları; sıra numarasıyla yeniden oynatma ve terminal sonuç teslimi.

**Uygulama kullanımı:** `core/internal/delivery/store.go`, `core/internal/delivery/resources.go`, `core/cmd/rotate-secrets/main.go`

**Migration kaynakları:** `core/migrations/00019_mcp_delivery.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `seq` | `bigint` | Evet | `nextval('mcp_events_seq_seq'::regclass)` | MCP replay sıralama sayacı |
| `id` | `uuid` | Evet | — | Kaydın veya ilgili protokol nesnesinin kimliği |
| `stream_id` | `uuid` | Evet | — | MCP stream UUID’si |
| `payload` | `bytea` | Evet | — | Şifreli MCP event verisi |
| `terminal` | `boolean` | Evet | `false` | İşlemi sonlandıran event olup olmadığı |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `mcp_events_id_key` | UNIQUE | `UNIQUE (id)` |
| `mcp_events_payload_check` | CHECK | `CHECK (octet_length(payload) <= 10486000)` |
| `mcp_events_pkey` | PRIMARY KEY | `PRIMARY KEY (seq)` |
| `mcp_events_stream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (stream_id) REFERENCES mcp_streams(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `mcp_event_replay` | `CREATE INDEX mcp_event_replay ON public.mcp_events USING btree (stream_id, seq)` |
| `mcp_events_id_key` | `CREATE UNIQUE INDEX mcp_events_id_key ON public.mcp_events USING btree (id)` |
| `mcp_events_pkey` | `CREATE UNIQUE INDEX mcp_events_pkey ON public.mcp_events USING btree (seq)` |


**Trigger’lar:** Yok.

### `mcp_operations`

MCP isteklerinin kalıcı çalışma durumu, principal eşzamanlılığı, iptal, fencing ve lease kaydı.

**Uygulama kullanımı:** `core/internal/delivery/store.go`

**Migration kaynakları:** `core/migrations/00019_mcp_delivery.sql`, `core/migrations/00032_delivery_cleanup_indexes.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | — | Kaydın veya ilgili protokol nesnesinin kimliği |
| `session_id` | `text` | Evet | — | MCP protokol oturum kimliği |
| `client_request_id` | `text` | Evet | — | İstemcinin MCP JSON-RPC istek kimliği |
| `principal_hash` | `bytea` | Evet | — | Çağıran principal’ın hash’i |
| `virtual_server` | `text` | Evet | — | Sanal sunucunun metinsel kimliği/slug bilgisi |
| `fence` | `uuid` | Evet | — | İşlem sahipliğini koruyan fencing UUID’si |
| `lease_until` | `timestamp with time zone` | Evet | — | İş sahibi lease’inin sona erme zamanı |
| `cancelled` | `boolean` | Evet | `false` | İptal isteği durumu |
| `state` | `text` | Evet | `'running'::text` | İşlem, teslim veya sağlık durum değeri |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `mcp_operations_client_request_id_check` | CHECK | `CHECK (octet_length(client_request_id) <= 1024)` |
| `mcp_operations_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `mcp_operations_session_id_fkey` | FOREIGN KEY | `FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE` |
| `mcp_operations_state_check` | CHECK | `CHECK (state = ANY (ARRAY['running'::text, 'done'::text, 'abandoned'::text]))` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `mcp_operation_cleanup` | `CREATE INDEX mcp_operation_cleanup ON public.mcp_operations USING btree (created_at) WHERE (state <> 'running'::text)` |
| `mcp_operation_expiry` | `CREATE INDEX mcp_operation_expiry ON public.mcp_operations USING btree (lease_until) WHERE (state = 'running'::text)` |
| `mcp_operation_principal` | `CREATE INDEX mcp_operation_principal ON public.mcp_operations USING btree (principal_hash) WHERE (state = 'running'::text)` |
| `mcp_operation_request` | `CREATE UNIQUE INDEX mcp_operation_request ON public.mcp_operations USING btree (session_id, client_request_id) WHERE (state = 'running'::text)` |
| `mcp_operations_pkey` | `CREATE UNIQUE INDEX mcp_operations_pkey ON public.mcp_operations USING btree (id)` |


**Trigger’lar:** Yok.

### `mcp_pending_requests`

Sunucudan istemciye giden MCP isteklerinin bekleyen yanıt ve süre kaydı.

**Uygulama kullanımı:** `core/internal/delivery/store.go`, `core/cmd/rotate-secrets/main.go`

**Migration kaynakları:** `core/migrations/00019_mcp_delivery.sql`, `core/migrations/00032_delivery_cleanup_indexes.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | — | Kaydın veya ilgili protokol nesnesinin kimliği |
| `operation_id` | `uuid` | Evet | — | İlgili işlem kimliği |
| `session_id` | `text` | Evet | — | MCP protokol oturum kimliği |
| `response` | `bytea` | Hayır | — | Şifreli istemci yanıtı |
| `expires_at` | `timestamp with time zone` | Evet | — | Geçerlilik/saklama süresinin sonu |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `mcp_pending_requests_operation_id_fkey` | FOREIGN KEY | `FOREIGN KEY (operation_id) REFERENCES mcp_operations(id) ON DELETE CASCADE` |
| `mcp_pending_requests_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `mcp_pending_requests_session_id_fkey` | FOREIGN KEY | `FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `mcp_pending_operation` | `CREATE INDEX mcp_pending_operation ON public.mcp_pending_requests USING btree (operation_id)` |
| `mcp_pending_requests_pkey` | `CREATE UNIQUE INDEX mcp_pending_requests_pkey ON public.mcp_pending_requests USING btree (id)` |


**Trigger’lar:** Yok.

### `mcp_resource_subscriptions`

MCP kaynak aboneliklerinin upstream URI, owner, lease ve güncelleme takibi.

**Uygulama kullanımı:** `core/internal/virtual/subscriptions.go`, `core/internal/delivery/resources.go`

**Migration kaynakları:** `core/migrations/00020_catalogs.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `session_id` | `text` | Evet | — | MCP protokol oturum kimliği |
| `uri` | `text` | Evet | — | İstemciye yayımlanan kaynak URI’si |
| `upstream_id` | `uuid` | Evet | — | İlgili fiziksel upstream kimliği |
| `original_uri` | `text` | Evet | — | Upstream kaynak URI’si |
| `revision` | `bigint` | Evet | — | Değişiklik/snapshot revision sayacı |
| `owner` | `uuid` | Hayır | — | Stream/abonelik sahibi düğüm |
| `lease_until` | `timestamp with time zone` | Hayır | — | İş sahibi lease’inin sona erme zamanı |
| `ready` | `boolean` | Evet | `false` | Aboneliğin hazır olma durumu |
| `failed` | `boolean` | Evet | `false` | Abonelik kurulumu hata durumu |
| `expires_at` | `timestamp with time zone` | Evet | `(now() + '00:15:00'::interval)` | Geçerlilik/saklama süresinin sonu |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `mcp_resource_subscriptions_pkey` | PRIMARY KEY | `PRIMARY KEY (session_id, uri)` |
| `mcp_resource_subscriptions_session_id_fkey` | FOREIGN KEY | `FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE` |
| `mcp_resource_subscriptions_upstream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `mcp_resource_subscriptions_pkey` | `CREATE UNIQUE INDEX mcp_resource_subscriptions_pkey ON public.mcp_resource_subscriptions USING btree (session_id, uri)` |


**Trigger’lar:** Yok.

### `mcp_streams`

MCP SSE/yanıt stream sahipliği, işlem bağlantısı, lease ve expiry.

**Uygulama kullanımı:** `core/internal/delivery/store.go`, `core/internal/delivery/resources.go`

**Migration kaynakları:** `core/migrations/00019_mcp_delivery.sql`, `core/migrations/00032_delivery_cleanup_indexes.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | — | Kaydın veya ilgili protokol nesnesinin kimliği |
| `session_id` | `text` | Evet | — | MCP protokol oturum kimliği |
| `operation_id` | `uuid` | Hayır | — | İlgili işlem kimliği |
| `owner` | `uuid` | Hayır | — | Stream/abonelik sahibi düğüm |
| `lease_until` | `timestamp with time zone` | Hayır | — | İş sahibi lease’inin sona erme zamanı |
| `closed` | `boolean` | Evet | `false` | Stream’in kapanma durumu |
| `last_revision` | `bigint` | Evet | `0` | Stream’in gördüğü son değişiklik revision’ı |
| `expires_at` | `timestamp with time zone` | Evet | `(now() + '00:15:00'::interval)` | Geçerlilik/saklama süresinin sonu |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `mcp_streams_operation_id_fkey` | FOREIGN KEY | `FOREIGN KEY (operation_id) REFERENCES mcp_operations(id) ON DELETE CASCADE` |
| `mcp_streams_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `mcp_streams_session_id_fkey` | FOREIGN KEY | `FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `mcp_get_stream` | `CREATE UNIQUE INDEX mcp_get_stream ON public.mcp_streams USING btree (session_id) WHERE ((operation_id IS NULL) AND (NOT closed))` |
| `mcp_stream_expiry` | `CREATE INDEX mcp_stream_expiry ON public.mcp_streams USING btree (expires_at)` |
| `mcp_stream_operation` | `CREATE INDEX mcp_stream_operation ON public.mcp_streams USING btree (operation_id) WHERE (operation_id IS NOT NULL)` |
| `mcp_streams_pkey` | `CREATE UNIQUE INDEX mcp_streams_pkey ON public.mcp_streams USING btree (id)` |


**Trigger’lar:** Yok.

### `projects`

Sanal sunucuları gruplayan ve isteğe bağlı bir takıma ait olan projeler.

**Uygulama kullanımı:** `core/internal/controlplane/server.go`, `core/internal/controlplane/ownership.go`, `core/internal/controlplane/configuration_authorization.go`, `core/internal/controlplane/declarative.go`, `core/internal/authz/catalog.go`, `core/internal/store/ownership_snapshot.go`, `core/internal/store/ownership.go`, `core/internal/store/authorization_assignments.go`, `core/internal/store/authorization_scopes.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/configuration_check.go`, `core/internal/store/snapshot.go`, `core/internal/store/virtualservers.go`, `core/internal/store/tool_grants.go`

**Migration kaynakları:** `core/migrations/00033_team_projects.sql`, `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00038_snapshot_audit_provenance.sql`, `core/migrations/00041_builtin_workflow_dependencies.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `slug` | `text` | Evet | — | İnsan tarafından okunabilir kısa kimlik |
| `name` | `text` | Evet | — | Görünen ad veya katalog öğesinin adı |
| `owner_team_id` | `uuid` | Hayır | — | Kaynak sahibi takım kimliği veya geçmiş sahiplik bağlamı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `project_audit_epochs` | CONSTRAINT TRIGGER | `TRIGGER DEFERRABLE INITIALLY DEFERRED` |
| `projects_owner_team_id_fkey` | FOREIGN KEY | `FOREIGN KEY (owner_team_id) REFERENCES teams(id) ON DELETE RESTRICT` |
| `projects_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `projects_slug_key` | UNIQUE | `UNIQUE (slug)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `projects_pkey` | `CREATE UNIQUE INDEX projects_pkey ON public.projects USING btree (id)` |
| `projects_slug_key` | `CREATE UNIQUE INDEX projects_slug_key ON public.projects USING btree (slug)` |


**Trigger’lar:** `project_audit_epochs`, `projects_bump`

### `revoked_jwts`

Hash üzerinden runtime JWT iptali; süresi dolan iptal kayıtları temizlenebilir.

**Uygulama kullanımı:** `core/internal/agentauth/revocations.go`

**Migration kaynakları:** `core/migrations/00022_identity_revocation.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `token_hash` | `bytea` | Evet | — | Token/JWT hash’i; açık token değildir |
| `expires_at` | `timestamp with time zone` | Evet | — | Geçerlilik/saklama süresinin sonu |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `revoked_jwts_pkey` | PRIMARY KEY | `PRIMARY KEY (token_hash)` |
| `revoked_jwts_token_hash_check` | CHECK | `CHECK (octet_length(token_hash) = 32)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `revoked_jwts_expiry` | `CREATE INDEX revoked_jwts_expiry ON public.revoked_jwts USING btree (expires_at)` |
| `revoked_jwts_pkey` | `CREATE UNIQUE INDEX revoked_jwts_pkey ON public.revoked_jwts USING btree (token_hash)` |


**Trigger’lar:** Yok.

### `sessions`

MCP protokol oturumları, initialize durumu, yetenekler ve kimlik sahipliği.

**Uygulama kullanımı:** `core/internal/fixture/legacy.go`, `core/internal/fixture/fixture.go`, `core/internal/config/snapshot.go`, `core/internal/oidc/oidc.go`, `core/internal/securitydemo/server.go`, `core/internal/controlplane/server.go`, `core/internal/controlplane/configuration_authorization.go`, `core/internal/virtual/handler.go`, `core/internal/virtual/omni.go`, `core/internal/virtual/sessionstore.go`, `core/internal/virtual/session.go`, `core/internal/delivery/store.go`, `core/internal/delivery/resources.go`, `core/internal/authz/catalog.go`, `core/internal/store/store.go`, `core/cmd/data-plane/main.go`

**Migration kaynakları:** `core/migrations/00009_sessions.sql`, `core/migrations/00016_session_lifecycle.sql`, `core/migrations/00017_session_ownership.sql`, `core/migrations/00019_mcp_delivery.sql`, `core/migrations/00020_catalogs.sql`, `core/migrations/00021_client_features.sql`, `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00039_oidc_claim_sessions.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `text` | Evet | — | Kaydın veya ilgili protokol nesnesinin kimliği |
| `virtual_server` | `text` | Evet | — | Sanal sunucunun metinsel kimliği/slug bilgisi |
| `key_slug` | `text` | Evet | `''::text` | Oturum açılırken kullanılan agent anahtar slug’ı |
| `protocol_version` | `text` | Evet | — | MCP protokol sürümü |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |
| `last_seen_at` | `timestamp with time zone` | Evet | `now()` | Son oturum etkinliği |
| `initialized` | `boolean` | Evet | `false` | MCP initialize tamamlanma durumu |
| `client_capabilities` | `jsonb` | Evet | `'{}'::jsonb` | İstemci MCP yetenekleri |
| `owner_mode` | `text` | Evet | `'legacy'::text` | Oturum sahibinin kimlik modeli |
| `credential_id` | `text` | Evet | `''::text` | Oturumu açan kimlik doğrulama nesnesi |
| `owner_issuer` | `text` | Evet | `''::text` | Oturum sahibi OIDC issuer |
| `owner_subject` | `text` | Evet | `''::text` | Oturum sahibi OIDC subject |
| `terminated_at` | `timestamp with time zone` | Hayır | — | MCP oturum sonlandırma zamanı |
| `logging_level` | `text` | Evet | `'warning'::text` | Oturumun MCP log seviyesi |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `sessions_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `sessions_last_seen_idx` | `CREATE INDEX sessions_last_seen_idx ON public.sessions USING btree (last_seen_at)` |
| `sessions_pkey` | `CREATE UNIQUE INDEX sessions_pkey ON public.sessions USING btree (id)` |
| `sessions_terminated_at_idx` | `CREATE INDEX sessions_terminated_at_idx ON public.sessions USING btree (terminated_at) WHERE (terminated_at IS NOT NULL)` |


**Trigger’lar:** Yok.

### `teams`

OIDC issuer/group çiftini sahiplik grubuna bağlayan takımlar.

**Uygulama kullanımı:** `core/internal/controlplane/server.go`, `core/internal/controlplane/ownership.go`, `core/internal/controlplane/configuration_authorization.go`, `core/internal/controlplane/declarative.go`, `core/internal/authz/catalog.go`, `core/internal/store/ownership.go`, `core/internal/store/authorization_assignments.go`, `core/internal/store/upstreams.go`, `core/internal/store/authorization_scopes.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/authorization_inspection.go`, `core/internal/store/configuration_check.go`, `core/internal/store/virtualservers.go`, `core/internal/store/tool_grants.go`

**Migration kaynakları:** `core/migrations/00033_team_projects.sql`, `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00041_builtin_workflow_dependencies.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `slug` | `text` | Evet | — | İnsan tarafından okunabilir kısa kimlik |
| `name` | `text` | Evet | — | Görünen ad veya katalog öğesinin adı |
| `issuer` | `text` | Evet | — | OIDC kimlik sağlayıcısının issuer değeri |
| `group_value` | `text` | Evet | — | Takıma karşılık gelen OIDC group claim değeri |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `teams_issuer_group_value_key` | UNIQUE | `UNIQUE (issuer, group_value)` |
| `teams_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `teams_slug_key` | UNIQUE | `UNIQUE (slug)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `teams_issuer_group_value_key` | `CREATE UNIQUE INDEX teams_issuer_group_value_key ON public.teams USING btree (issuer, group_value)` |
| `teams_pkey` | `CREATE UNIQUE INDEX teams_pkey ON public.teams USING btree (id)` |
| `teams_slug_key` | `CREATE UNIQUE INDEX teams_slug_key ON public.teams USING btree (slug)` |


**Trigger’lar:** `teams_bump`

### `tenant_runtime_identity`

Bu veritabanının runtime tenant kimliği; audit kayıtlarına text kimlik sağlar.

**Uygulama kullanımı:** `core/internal/store/store.go`

**Migration kaynakları:** `core/migrations/00025_tenant_identity.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `singleton` | `boolean` | Evet | `true` | Yalnızca true olabilen tekil ayar anahtarı |
| `tenant_id` | `text` | Evet | — | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `tenant_runtime_identity_pkey` | PRIMARY KEY | `PRIMARY KEY (singleton)` |
| `tenant_runtime_identity_singleton_check` | CHECK | `CHECK (singleton)` |
| `tenant_runtime_identity_tenant_id_key` | UNIQUE | `UNIQUE (tenant_id)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `tenant_runtime_identity_pkey` | `CREATE UNIQUE INDEX tenant_runtime_identity_pkey ON public.tenant_runtime_identity USING btree (singleton)` |
| `tenant_runtime_identity_tenant_id_key` | `CREATE UNIQUE INDEX tenant_runtime_identity_tenant_id_key ON public.tenant_runtime_identity USING btree (tenant_id)` |


**Trigger’lar:** Yok.

### `tenants`

Tenant UUID ve slug kataloğu; temel yapılandırma kayıtları buna bağlanır.

**Uygulama kullanımı:** `core/internal/tenancy/router.go`, `core/internal/store/store.go`, `core/internal/store/mock.go`, `core/internal/store/snapshot.go`, `core/internal/store/tx.go`

**Migration kaynakları:** `core/migrations/00001_init.sql`, `core/migrations/00002_revision_triggers.sql`, `core/migrations/00004_agent_keys.sql`, `core/migrations/00007_admin_tokens.sql`, `core/migrations/00014_identity_bindings.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `slug` | `text` | Evet | — | İnsan tarafından okunabilir kısa kimlik |
| `name` | `text` | Evet | — | Görünen ad veya katalog öğesinin adı |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `tenants_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `tenants_slug_key` | UNIQUE | `UNIQUE (slug)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `tenants_pkey` | `CREATE UNIQUE INDEX tenants_pkey ON public.tenants USING btree (id)` |
| `tenants_slug_key` | `CREATE UNIQUE INDEX tenants_slug_key ON public.tenants USING btree (slug)` |


**Trigger’lar:** `tenants_bump`

### `tool_grants`

Sanal sunucunun belirli upstream aracını kullanma talebi ve onay/ret/iptal durumu.

**Uygulama kullanımı:** `core/internal/controlplane/configuration_access.go`, `core/internal/controlplane/configuration_authorization.go`, `core/internal/authz/catalog.go`, `core/internal/store/ownership_snapshot.go`, `core/internal/store/ownership.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/configuration_check.go`, `core/internal/store/virtualservers.go`, `core/internal/store/tool_grants.go`

**Migration kaynakları:** `core/migrations/00033_team_projects.sql`, `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00041_builtin_workflow_dependencies.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `virtual_server_id` | `uuid` | Evet | — | İlgili sanal sunucu kimliği |
| `upstream_id` | `uuid` | Evet | — | İlgili fiziksel upstream kimliği |
| `tool_name` | `text` | Evet | — | Upstream araç adı |
| `status` | `text` | Evet | — | Sonuç veya grant durumu |
| `reason` | `text` | Evet | `''::text` | Karar, hata veya hold gerekçesi |
| `actor` | `text` | Evet | `''::text` | Grant kararını veren aktör |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |
| `updated_at` | `timestamp with time zone` | Evet | `now()` | Son güncelleme zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `tool_grants_pkey` | PRIMARY KEY | `PRIMARY KEY (virtual_server_id, upstream_id, tool_name)` |
| `tool_grants_status_check` | CHECK | `CHECK (status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'revoked'::text]))` |
| `tool_grants_upstream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE` |
| `tool_grants_virtual_server_id_fkey` | FOREIGN KEY | `FOREIGN KEY (virtual_server_id) REFERENCES virtual_servers(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `tool_grants_pkey` | `CREATE UNIQUE INDEX tool_grants_pkey ON public.tool_grants USING btree (virtual_server_id, upstream_id, tool_name)` |


**Trigger’lar:** `tool_grants_bump`

### `upstream_catalog`

Upstream kaynak/resource-template/prompt gibi katalog tanımlarının önbelleği.

**Uygulama kullanımı:** `core/internal/store/catalog.go`, `core/internal/store/catalog_snapshot.go`

**Migration kaynakları:** `core/migrations/00020_catalogs.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `upstream_id` | `uuid` | Evet | — | İlgili fiziksel upstream kimliği |
| `kind` | `text` | Evet | — | Upstream veya katalog öğesi türü |
| `name` | `text` | Evet | — | Görünen ad veya katalog öğesinin adı |
| `definition` | `jsonb` | Evet | — | Tam MCP katalog/araç JSON tanımı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `upstream_catalog_definition_check` | CHECK | `CHECK (jsonb_typeof(definition) = 'object'::text)` |
| `upstream_catalog_kind_check` | CHECK | `CHECK (kind = ANY (ARRAY['resource'::text, 'template'::text, 'prompt'::text]))` |
| `upstream_catalog_name_check` | CHECK | `CHECK (octet_length(name) <= 4096)` |
| `upstream_catalog_pkey` | PRIMARY KEY | `PRIMARY KEY (upstream_id, kind, name)` |
| `upstream_catalog_upstream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `upstream_catalog_pkey` | `CREATE UNIQUE INDEX upstream_catalog_pkey ON public.upstream_catalog USING btree (upstream_id, kind, name)` |


**Trigger’lar:** `upstream_catalog_revision`

### `upstream_health`

Upstream için düğüm bazında sağlık, circuit breaker ve gecikme görünümü.

**Uygulama kullanımı:** `core/internal/health/health.go`, `core/internal/store/upstreams.go`

**Migration kaynakları:** `core/migrations/00010_resilience.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `upstream_id` | `uuid` | Evet | — | İlgili fiziksel upstream kimliği |
| `node` | `text` | Evet | — | Sağlık kaydını üreten düğüm |
| `state` | `text` | Evet | — | İşlem, teslim veya sağlık durum değeri |
| `breaker` | `text` | Evet | `'closed'::text` | Circuit breaker durumu |
| `url` | `text` | Evet | `''::text` | Upstream erişim URL’si |
| `latency_ms` | `integer` | Evet | `0` | Ölçülen gecikme (ms) |
| `error` | `text` | Hayır | — | Son hata metni |
| `checked_at` | `timestamp with time zone` | Evet | `now()` | Sağlık kontrol zamanı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `upstream_health_pkey` | PRIMARY KEY | `PRIMARY KEY (upstream_id, node)` |
| `upstream_health_state_check` | CHECK | `CHECK (state = ANY (ARRAY['healthy'::text, 'unhealthy'::text]))` |
| `upstream_health_upstream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `upstream_health_pkey` | `CREATE UNIQUE INDEX upstream_health_pkey ON public.upstream_health USING btree (upstream_id, node)` |


**Trigger’lar:** Yok.

### `upstream_status`

Upstream discovery yenilemesinin son zamanı ve hata bilgisi.

**Uygulama kullanımı:** `core/internal/controlplane/discovery.go`, `core/internal/store/upstreams.go`

**Migration kaynakları:** `core/migrations/00003_api_support.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `upstream_id` | `uuid` | Evet | — | İlgili fiziksel upstream kimliği |
| `last_refresh_at` | `timestamp with time zone` | Hayır | — | Son discovery yenileme zamanı |
| `last_refresh_error` | `text` | Hayır | — | Son discovery hatası |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `upstream_status_pkey` | PRIMARY KEY | `PRIMARY KEY (upstream_id)` |
| `upstream_status_upstream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `upstream_status_pkey` | `CREATE UNIQUE INDEX upstream_status_pkey ON public.upstream_status USING btree (upstream_id)` |


**Trigger’lar:** Yok.

### `upstream_tools`

Upstream araç adları, JSON input şeması, annotation ve tam tanım önbelleği.

**Uygulama kullanımı:** `core/internal/store/upstreams.go`, `core/internal/store/upstream_invocation_security.go`, `core/internal/store/snapshot.go`, `core/internal/store/virtualservers.go`, `core/internal/store/tool_grants.go`

**Migration kaynakları:** `core/migrations/00001_init.sql`, `core/migrations/00002_revision_triggers.sql`, `core/migrations/00021_client_features.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `upstream_id` | `uuid` | Evet | — | İlgili fiziksel upstream kimliği |
| `name` | `text` | Evet | — | Görünen ad veya katalog öğesinin adı |
| `description` | `text` | Evet | `''::text` | Açıklama metni |
| `input_schema` | `jsonb` | Evet | `'{}'::jsonb` | Aracın JSON girdi şeması |
| `annotations` | `jsonb` | Evet | `'{}'::jsonb` | MCP araç annotation bilgileri |
| `fetched_at` | `timestamp with time zone` | Evet | `now()` | Discovery ile alınma zamanı |
| `definition` | `jsonb` | Evet | `'{}'::jsonb` | Tam MCP katalog/araç JSON tanımı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `upstream_tools_pkey` | PRIMARY KEY | `PRIMARY KEY (upstream_id, name)` |
| `upstream_tools_upstream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `upstream_tools_pkey` | `CREATE UNIQUE INDEX upstream_tools_pkey ON public.upstream_tools USING btree (upstream_id, name)` |


**Trigger’lar:** `upstream_tools_bump`

### `upstreams`

Fiziksel MCP/REST bağlantıları; transport, TLS/OAuth, şifreli sırlar, kimlik aktarımı ve sahiplik.

**Uygulama kullanımı:** `core/internal/invocation/policy.go`, `core/internal/snapshot/syncer.go`, `core/internal/config/guidance.go`, `core/internal/health/health.go`, `core/internal/audit/policy.go`, `core/internal/controlplane/discovery.go`, `core/internal/controlplane/server.go`, `core/internal/controlplane/audit_admission.go`, `core/internal/controlplane/upstreams.go`, `core/internal/controlplane/configuration_authorization.go`, `core/internal/controlplane/control_audit.go`, `core/internal/controlplane/declarative.go`, `core/internal/resilience/breaker.go`, `core/internal/virtual/handler.go`, `core/internal/virtual/session.go`, `core/internal/authz/catalog.go`, `core/internal/callercontext/context.go`, `core/internal/store/ownership_snapshot.go`, `core/internal/store/ownership.go`, `core/internal/store/store.go`, `core/internal/store/catalog.go`, `core/internal/store/upstreams.go`, `core/internal/store/authorization_scopes.go`, `core/internal/store/rest.go`, `core/internal/store/invocation_security.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/upstream_invocation_security.go`, `core/internal/store/configuration_check.go`, `core/internal/store/control_audit.go`, `core/internal/store/snapshot.go`, `core/internal/store/virtualservers.go`, `core/internal/store/guidance.go`, `core/internal/store/catalog_snapshot.go`, `core/internal/store/tool_grants.go`, `core/cmd/rotate-secrets/main.go`

**Migration kaynakları:** `core/migrations/00001_init.sql`, `core/migrations/00002_revision_triggers.sql`, `core/migrations/00003_api_support.sql`, `core/migrations/00010_resilience.sql`, `core/migrations/00012_upstream_tls.sql`, `core/migrations/00015_upstream_oauth.sql`, `core/migrations/00019_mcp_delivery.sql`, `core/migrations/00020_catalogs.sql`, `core/migrations/00026_http_only_upstreams.sql`, `core/migrations/00027_guidance.sql`, `core/migrations/00028_rest_upstreams.sql`, `core/migrations/00030_caller_context.sql`, `core/migrations/00031_request_header_mappings.sql`, `core/migrations/00033_team_projects.sql`, `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00038_snapshot_audit_provenance.sql`, `core/migrations/00041_builtin_workflow_dependencies.sql`, `core/migrations/00045_upstream_invocation_security.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `tenant_id` | `uuid` | Evet | — | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |
| `slug` | `text` | Evet | — | İnsan tarafından okunabilir kısa kimlik |
| `name` | `text` | Evet | — | Görünen ad veya katalog öğesinin adı |
| `url` | `text` | Evet | — | Upstream erişim URL’si |
| `transport` | `text` | Evet | — | MCP HTTP transport türü |
| `encrypted_headers` | `bytea` | Hayır | — | AES-GCM ile şifreli upstream HTTP başlıkları |
| `enabled` | `boolean` | Evet | `true` | Kaydın kullanımda olup olmadığı |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |
| `updated_at` | `timestamp with time zone` | Evet | `now()` | Son güncelleme zamanı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |
| `description` | `text` | Evet | `''::text` | Açıklama metni |
| `fallback_urls` | `text[]` | Evet | `'{}'::text[]` | Alternatif upstream erişim URL’leri |
| `ca_pem` | `text` | Evet | `''::text` | Upstream TLS doğrulamasında kullanılan CA PEM |
| `tls_server_name` | `text` | Evet | `''::text` | TLS sunucu adı doğrulama/SNI geçersiz kılması |
| `client_cert_pem` | `text` | Evet | `''::text` | mTLS istemci sertifikası PEM |
| `encrypted_client_key` | `bytea` | Hayır | — | Şifreli mTLS özel anahtarı |
| `oauth_token_url` | `text` | Evet | `''::text` | OAuth token endpoint URL’si |
| `oauth_client_id` | `text` | Evet | `''::text` | OAuth istemci kimliği |
| `oauth_scopes` | `text[]` | Evet | `'{}'::text[]` | Talep edilen OAuth scope listesi |
| `oauth_audience` | `text` | Evet | `''::text` | OAuth hedef audience |
| `encrypted_oauth_secret` | `bytea` | Hayır | — | Şifreli OAuth istemci sırrı |
| `mcp_capabilities` | `jsonb` | Evet | `'{}'::jsonb` | Discovery ile öğrenilen upstream MCP yetenekleri |
| `discovered_instructions` | `text` | Evet | `''::text` | Upstream tarafından bildirilen instructions |
| `kind` | `text` | Evet | `'mcp'::text` | Upstream veya katalog öğesi türü |
| `encrypted_rest_config` | `bytea` | Hayır | — | Kimlik bilgileri dahil şifreli REST bağlantı tanımı |
| `caller_context` | `jsonb` | Evet | `'{}'::jsonb` | Çağıran kimliğin upstream’e aktarım politikası |
| `request_header_mappings` | `jsonb` | Evet | `'[]'::jsonb` | Gelen HTTP başlıklarının upstream’e eşlenme kuralları |
| `owner_team_id` | `uuid` | Hayır | — | Kaynak sahibi takım kimliği veya geçmiş sahiplik bağlamı |
| `invocation_security_revision` | `bigint` | Evet | `0` | Upstream’i etkileyen invocation güvenlik revision’ı |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `upstream_audit_epoch` | CONSTRAINT TRIGGER | `TRIGGER DEFERRABLE INITIALLY DEFERRED` |
| `upstreams_kind_check` | CHECK | `CHECK (kind = ANY (ARRAY['mcp'::text, 'rest'::text]))` |
| `upstreams_owner_team_id_fkey` | FOREIGN KEY | `FOREIGN KEY (owner_team_id) REFERENCES teams(id) ON DELETE RESTRICT` |
| `upstreams_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `upstreams_tenant_id_fkey` | FOREIGN KEY | `FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE` |
| `upstreams_tenant_id_slug_key` | UNIQUE | `UNIQUE (tenant_id, slug)` |
| `upstreams_transport_check` | CHECK | `CHECK (transport = ANY (ARRAY['streamable_http'::text, 'sse'::text]))` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `upstream_owner_idx` | `CREATE INDEX upstream_owner_idx ON public.upstreams USING btree (owner_team_id)` |
| `upstreams_pkey` | `CREATE UNIQUE INDEX upstreams_pkey ON public.upstreams USING btree (id)` |
| `upstreams_tenant_id_slug_key` | `CREATE UNIQUE INDEX upstreams_tenant_id_slug_key ON public.upstreams USING btree (tenant_id, slug)` |


**Trigger’lar:** `upstream_audit_epoch`, `upstreams_bump`

### `virtual_catalog`

Sanal sunucuda yayımlanan upstream katalog öğeleri; alias, açıklama ve sıralama.

**Uygulama kullanımı:** `core/internal/virtual/streams.go`, `core/internal/delivery/store.go`, `core/internal/store/catalog.go`, `core/internal/store/upstreams.go`, `core/internal/store/catalog_snapshot.go`

**Migration kaynakları:** `core/migrations/00020_catalogs.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `virtual_server_id` | `uuid` | Evet | — | İlgili sanal sunucu kimliği |
| `upstream_id` | `uuid` | Evet | — | İlgili fiziksel upstream kimliği |
| `kind` | `text` | Evet | — | Upstream veya katalog öğesi türü |
| `name` | `text` | Evet | — | Görünen ad veya katalog öğesinin adı |
| `alias` | `text` | Evet | `''::text` | Yayımlanırken kullanılan isteğe bağlı alternatif ad |
| `description` | `text` | Evet | `''::text` | Açıklama metni |
| `description_basis` | `text` | Evet | `''::text` | Açıklama override’ının dayandığı kaynak açıklama |
| `position` | `integer` | Evet | — | Yayımlama sırası |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `virtual_catalog_kind_check` | CHECK | `CHECK (kind = ANY (ARRAY['resource'::text, 'template'::text, 'prompt'::text]))` |
| `virtual_catalog_name_check` | CHECK | `CHECK (octet_length(name) <= 4096)` |
| `virtual_catalog_pkey` | PRIMARY KEY | `PRIMARY KEY (virtual_server_id, upstream_id, kind, name)` |
| `virtual_catalog_upstream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE` |
| `virtual_catalog_virtual_server_id_fkey` | FOREIGN KEY | `FOREIGN KEY (virtual_server_id) REFERENCES virtual_servers(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `virtual_catalog_pkey` | `CREATE UNIQUE INDEX virtual_catalog_pkey ON public.virtual_catalog USING btree (virtual_server_id, upstream_id, kind, name)` |


**Trigger’lar:** `virtual_catalog_revision`

### `virtual_server_tools`

Sanal sunucuda yayımlanan araçların allowlist’i, alias ve açıklama geçersiz kılmaları.

**Uygulama kullanımı:** `core/internal/virtual/streams.go`, `core/internal/store/upstreams.go`, `core/internal/store/snapshot.go`, `core/internal/store/virtualservers.go`

**Migration kaynakları:** `core/migrations/00001_init.sql`, `core/migrations/00002_revision_triggers.sql`, `core/migrations/00003_api_support.sql`, `core/migrations/00013_tool_description_override.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `virtual_server_id` | `uuid` | Evet | — | İlgili sanal sunucu kimliği |
| `upstream_id` | `uuid` | Evet | — | İlgili fiziksel upstream kimliği |
| `tool_name` | `text` | Evet | — | Upstream araç adı |
| `alias` | `text` | Hayır | — | Yayımlanırken kullanılan isteğe bağlı alternatif ad |
| `position` | `integer` | Evet | `0` | Yayımlama sırası |
| `description` | `text` | Hayır | — | Açıklama metni |
| `description_basis` | `text` | Hayır | — | Açıklama override’ının dayandığı kaynak açıklama |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `virtual_server_tools_pkey` | PRIMARY KEY | `PRIMARY KEY (virtual_server_id, upstream_id, tool_name)` |
| `virtual_server_tools_upstream_id_fkey` | FOREIGN KEY | `FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE` |
| `virtual_server_tools_virtual_server_id_fkey` | FOREIGN KEY | `FOREIGN KEY (virtual_server_id) REFERENCES virtual_servers(id) ON DELETE CASCADE` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `virtual_server_tools_pkey` | `CREATE UNIQUE INDEX virtual_server_tools_pkey ON public.virtual_server_tools USING btree (virtual_server_id, upstream_id, tool_name)` |


**Trigger’lar:** `virtual_server_tools_bump`

### `virtual_servers`

İstemciye sunulan sanal MCP sunucuları; güvenlik, PII, araç politikası, istemci özellikleri ve proje.

**Uygulama kullanımı:** `core/internal/snapshot/syncer.go`, `core/internal/controlplane/configuration_authorization.go`, `core/internal/virtual/streams.go`, `core/internal/delivery/store.go`, `core/internal/authz/catalog.go`, `core/internal/store/audit.go`, `core/internal/store/ownership_snapshot.go`, `core/internal/store/ownership.go`, `core/internal/store/store.go`, `core/internal/store/upstreams.go`, `core/internal/store/authorization_scopes.go`, `core/internal/store/configuration_policy.go`, `core/internal/store/bindings.go`, `core/internal/store/configuration_check.go`, `core/internal/store/control_audit.go`, `core/internal/store/agentkeys.go`, `core/internal/store/snapshot_audit.go`, `core/internal/store/snapshot.go`, `core/internal/store/virtualservers.go`, `core/internal/store/tool_grants.go`

**Migration kaynakları:** `core/migrations/00001_init.sql`, `core/migrations/00002_revision_triggers.sql`, `core/migrations/00003_api_support.sql`, `core/migrations/00004_agent_keys.sql`, `core/migrations/00006_pii_config.sql`, `core/migrations/00008_tool_policy.sql`, `core/migrations/00011_rate_limits.sql`, `core/migrations/00014_identity_bindings.sql`, `core/migrations/00020_catalogs.sql`, `core/migrations/00021_client_features.sql`, `core/migrations/00027_guidance.sql`, `core/migrations/00033_team_projects.sql`, `core/migrations/00034_omni.sql`, `core/migrations/00036_control_plane_authorization.sql`, `core/migrations/00038_snapshot_audit_provenance.sql`, `core/migrations/00041_builtin_workflow_dependencies.sql`

| Kolon | PostgreSQL tipi | Zorunlu | Varsayılan | Amaç |
| --- | --- | --- | --- | --- |
| `id` | `uuid` | Evet | `gen_random_uuid()` | Kaydın veya ilgili protokol nesnesinin kimliği |
| `tenant_id` | `uuid` | Evet | — | Tenant kimliği; türü ve FK bulunup bulunmadığı aşağıdaki constraint listesinde belirtilir |
| `slug` | `text` | Evet | — | İnsan tarafından okunabilir kısa kimlik |
| `name` | `text` | Evet | — | Görünen ad veya katalog öğesinin adı |
| `description` | `text` | Evet | `''::text` | Açıklama metni |
| `enabled` | `boolean` | Evet | `true` | Kaydın kullanımda olup olmadığı |
| `created_at` | `timestamp with time zone` | Evet | `now()` | Oluşturulma zamanı |
| `updated_at` | `timestamp with time zone` | Evet | `now()` | Son güncelleme zamanı |
| `version` | `bigint` | Evet | `1` | İyimser eşzamanlılık kontrolü için kayıt sürümü |
| `auth_required` | `boolean` | Evet | `true` | Sanal sunucunun kimlik doğrulama gereksinimi |
| `pii_config` | `jsonb` | Evet | `'{}'::jsonb` | Kişisel veri tespit/redaksiyon JSON ayarı |
| `tool_policy` | `jsonb` | Evet | `'{}'::jsonb` | Araç kullanım politikası JSON ayarı |
| `rate_limit_per_minute` | `integer` | Hayır | — | Dakikalık istek sınırı; NULL sınırsızdır, dolu değer CHECK gereği pozitiftir |
| `client_features` | `jsonb` | Evet | `'{}'::jsonb` | İzin verilen MCP istemci özellikleri |
| `guidance` | `jsonb` | Evet | `'{}'::jsonb` | Sanal sunucu instructions/guidance JSON ayarı |
| `project_id` | `uuid` | Evet | — | Proje kimliği veya geçmiş proje bağlamı |
| `omni_enabled` | `boolean` | Evet | `false` | Sanal sunucuda Omni özelliklerinin etkinliği |


**Anahtar ve kısıtlar:**

| Ad | Tür | Tanım |
| --- | --- | --- |
| `virtual_audit_epoch` | CONSTRAINT TRIGGER | `TRIGGER DEFERRABLE INITIALLY DEFERRED` |
| `virtual_servers_pkey` | PRIMARY KEY | `PRIMARY KEY (id)` |
| `virtual_servers_project_id_fkey` | FOREIGN KEY | `FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE RESTRICT` |
| `virtual_servers_rate_limit_per_minute_check` | CHECK | `CHECK (rate_limit_per_minute > 0)` |
| `virtual_servers_tenant_id_fkey` | FOREIGN KEY | `FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE` |
| `virtual_servers_tenant_id_slug_key` | UNIQUE | `UNIQUE (tenant_id, slug)` |


**İndeksler:**

| Ad | Tam tanım |
| --- | --- |
| `virtual_project_idx` | `CREATE INDEX virtual_project_idx ON public.virtual_servers USING btree (project_id)` |
| `virtual_servers_pkey` | `CREATE UNIQUE INDEX virtual_servers_pkey ON public.virtual_servers USING btree (id)` |
| `virtual_servers_tenant_id_slug_key` | `CREATE UNIQUE INDEX virtual_servers_tenant_id_slug_key ON public.virtual_servers USING btree (tenant_id, slug)` |


**Trigger’lar:** `virtual_audit_epoch`, `virtual_default_project`, `virtual_servers_bump`

## İndekslerin kullanım amaçları

- Her `_pkey` indeksi primary key benzersizliğini sağlar ve kimlikle erişimi hızlandırır. UNIQUE constraint destek indeksleri ilgili tekil slug/kimlik/ilişki kurallarını uygular.
- `audit_events_*`, `audit_conversation_*`, `audit_root_conversation_*` indeksleri zaman, sunucu, anahtar, request/operation ve konuşma bazında audit arama/retention işlerini hızlandırır. Operation-stage unique indeksi tek admission/outcome kaydını garanti eder.
- `audit_deliveries_due` teslim zamanı gelmiş pending/leased işleri; payload expiry ve retention indeksleri süresi dolan kayıtları bulur.
- `authorization_assignments_principal` ve `_mapping` yalnızca etkin atamaları ilgili kimlik/eşlemeyle bulur. `audit_resource_epochs_open` aynı tenant/kaynak için tek açık epoch sağlar.
- `mcp_operation_*` aktif işlem tekilliği, expiry ve principal sorguları; `mcp_get_stream` açık GET stream tekilliği; stream/event/subscription indeksleri replay, sahiplik ve temizlik sorguları içindir.
- `browser_sessions_*`, `revoked_jwts_*` ve session indeksleri kimlik/expiry ile oturum ve iptal aramasını hızlandırır.
- `virtual_project_idx` ve `upstream_owner_idx` proje/takım bazında kaynak seçimini hızlandırır.

Her indeksin kesin kolon sırası, UNIQUE özelliği, metodu ve varsa partial WHERE koşulu ilgili tablo altında tam SQL olarak verilmiştir. Aynı amaca benzeyen indeksler katalogda mevcut oldukları için ayrıca korunmuştur.

## Trigger envanteri

FK’lerin PostgreSQL tarafından oluşturulan iç RI trigger’ları ayrıca kullanıcı trigger’ı olarak sayılmamıştır; işlevleri FK tanımlarında yer alır. Aşağıdaki tanımlar zamanlama, INSERT/UPDATE/DELETE olayları, satır/statement düzeyi ve deferred davranışını içerir.

| Tablo | Trigger | Tam tanım |
| --- | --- | --- |
| `admission_settings` | `admission_settings_revision` | `CREATE TRIGGER admission_settings_revision AFTER UPDATE ON admission_settings FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `agent_key_servers` | `agent_key_servers_bump` | `CREATE TRIGGER agent_key_servers_bump AFTER INSERT OR DELETE OR UPDATE ON agent_key_servers FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `agent_keys` | `agent_keys_bump` | `CREATE TRIGGER agent_keys_bump AFTER INSERT OR DELETE OR UPDATE ON agent_keys FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `audit_access_contexts` | `audit_access_contexts_immutable` | `CREATE TRIGGER audit_access_contexts_immutable BEFORE UPDATE ON audit_access_contexts FOR EACH ROW EXECUTE FUNCTION reject_audit_context_update()` |
| `audit_access_contexts` | `audit_access_contexts_validate` | `CREATE TRIGGER audit_access_contexts_validate BEFORE INSERT ON audit_access_contexts FOR EACH ROW EXECUTE FUNCTION validate_audit_context()` |
| `audit_access_domains` | `audit_access_domains_immutable` | `CREATE TRIGGER audit_access_domains_immutable BEFORE UPDATE ON audit_access_domains FOR EACH ROW EXECUTE FUNCTION reject_audit_context_update()` |
| `audit_access_domains` | `audit_access_domains_validate` | `CREATE TRIGGER audit_access_domains_validate BEFORE INSERT ON audit_access_domains FOR EACH ROW EXECUTE FUNCTION validate_audit_domain()` |
| `audit_resource_epochs` | `audit_resource_epochs_protect` | `CREATE TRIGGER audit_resource_epochs_protect BEFORE UPDATE ON audit_resource_epochs FOR EACH ROW EXECUTE FUNCTION protect_audit_epoch()` |
| `audit_settings` | `audit_settings_bump` | `CREATE TRIGGER audit_settings_bump AFTER INSERT OR DELETE OR UPDATE ON audit_settings FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `authorization_assignments` | `authorization_assignments_bump` | `CREATE TRIGGER authorization_assignments_bump AFTER INSERT OR DELETE OR UPDATE ON authorization_assignments FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision()` |
| `authorization_mappings` | `authorization_mappings_bump` | `CREATE TRIGGER authorization_mappings_bump AFTER INSERT OR DELETE OR UPDATE ON authorization_mappings FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision()` |
| `authorization_permissions` | `authorization_permissions_bump` | `CREATE TRIGGER authorization_permissions_bump AFTER INSERT OR DELETE OR UPDATE ON authorization_permissions FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision()` |
| `authorization_role_permissions` | `authorization_role_permissions_bump` | `CREATE TRIGGER authorization_role_permissions_bump AFTER INSERT OR DELETE OR UPDATE ON authorization_role_permissions FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision()` |
| `authorization_roles` | `authorization_roles_bump` | `CREATE TRIGGER authorization_roles_bump AFTER INSERT OR DELETE OR UPDATE ON authorization_roles FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision()` |
| `identity_binding_servers` | `identity_binding_servers_bump` | `CREATE TRIGGER identity_binding_servers_bump AFTER INSERT OR DELETE OR UPDATE ON identity_binding_servers FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `identity_bindings` | `identity_bindings_bump` | `CREATE TRIGGER identity_bindings_bump AFTER INSERT OR DELETE OR UPDATE ON identity_bindings FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `invocation_security` | `invocation_security_revision` | `CREATE TRIGGER invocation_security_revision AFTER UPDATE ON invocation_security FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `projects` | `project_audit_epochs` | `CREATE CONSTRAINT TRIGGER project_audit_epochs AFTER UPDATE ON projects DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION maintain_project_audit_epochs()` |
| `projects` | `projects_bump` | `CREATE TRIGGER projects_bump AFTER INSERT OR DELETE OR UPDATE ON projects FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `teams` | `teams_bump` | `CREATE TRIGGER teams_bump AFTER INSERT OR DELETE OR UPDATE ON teams FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `tenants` | `tenants_bump` | `CREATE TRIGGER tenants_bump AFTER INSERT OR DELETE OR UPDATE ON tenants FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `tool_grants` | `tool_grants_bump` | `CREATE TRIGGER tool_grants_bump AFTER INSERT OR DELETE OR UPDATE ON tool_grants FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `upstream_catalog` | `upstream_catalog_revision` | `CREATE TRIGGER upstream_catalog_revision AFTER INSERT OR DELETE OR UPDATE ON upstream_catalog FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `upstream_tools` | `upstream_tools_bump` | `CREATE TRIGGER upstream_tools_bump AFTER INSERT OR DELETE OR UPDATE ON upstream_tools FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `upstreams` | `upstream_audit_epoch` | `CREATE CONSTRAINT TRIGGER upstream_audit_epoch AFTER INSERT OR DELETE OR UPDATE ON upstreams DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION maintain_audit_resource_epoch()` |
| `upstreams` | `upstreams_bump` | `CREATE TRIGGER upstreams_bump AFTER INSERT OR DELETE OR UPDATE ON upstreams FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `virtual_catalog` | `virtual_catalog_revision` | `CREATE TRIGGER virtual_catalog_revision AFTER INSERT OR DELETE OR UPDATE ON virtual_catalog FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `virtual_server_tools` | `virtual_server_tools_bump` | `CREATE TRIGGER virtual_server_tools_bump AFTER INSERT OR DELETE OR UPDATE ON virtual_server_tools FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |
| `virtual_servers` | `virtual_audit_epoch` | `CREATE CONSTRAINT TRIGGER virtual_audit_epoch AFTER INSERT OR DELETE OR UPDATE ON virtual_servers DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION maintain_audit_resource_epoch()` |
| `virtual_servers` | `virtual_default_project` | `CREATE TRIGGER virtual_default_project BEFORE INSERT ON virtual_servers FOR EACH ROW EXECUTE FUNCTION default_virtual_project()` |
| `virtual_servers` | `virtual_servers_bump` | `CREATE TRIGGER virtual_servers_bump AFTER INSERT OR DELETE OR UPDATE ON virtual_servers FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision()` |


## Fonksiyonlar

### `bump_authorization_revision`

Yetkilendirme kataloğu/rol/atama değiştiğinde authorization_state.revision değerini artırır.

```sql
CREATE OR REPLACE FUNCTION public.bump_authorization_revision()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
 UPDATE authorization_state SET revision=revision+1 WHERE singleton;
 RETURN NULL;
END $function$
```

### `bump_config_revision`

Global revision’ı artırır ve vmcp_config kanalına yeni değeri pg_notify ile yollar; data-plane snapshot syncer LISTEN ile izler.

```sql
CREATE OR REPLACE FUNCTION public.bump_config_revision()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
DECLARE
    rev bigint;
BEGIN
    UPDATE config_revision SET revision = revision + 1 RETURNING revision INTO rev;
    PERFORM pg_notify('vmcp_config', rev::text);
    RETURN NULL;
END;
$function$
```

### `default_virtual_project`

Yeni sanal sunucuda project_id verilmemişse default projenin UUID’sini yerleştirir.

```sql
CREATE OR REPLACE FUNCTION public.default_virtual_project()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
 IF NEW.project_id IS NULL THEN SELECT id INTO NEW.project_id FROM projects WHERE slug='default'; END IF;
 RETURN NEW;
END $function$
```

### `maintain_audit_resource_epoch`

Deferred trigger çalışırken upstream/sanal sunucunun transaction sonundaki halini okuyup epoch senkronizasyonunu çağırır.

```sql
CREATE OR REPLACE FUNCTION public.maintain_audit_resource_epoch()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
DECLARE rid uuid; pid uuid; tid uuid;
BEGIN
 rid=CASE WHEN TG_OP='DELETE' THEN OLD.id ELSE NEW.id END;
 IF TG_TABLE_NAME='upstreams' THEN
  -- At deferred execution the resource might have been changed/deleted again.
  SELECT owner_team_id INTO tid FROM upstreams WHERE id=rid;
  PERFORM sync_audit_resource_epoch('upstream',rid,NULL,tid,NOT FOUND);
 ELSE
  SELECT v.project_id,p.owner_team_id INTO pid,tid FROM virtual_servers v JOIN projects p ON p.id=v.project_id WHERE v.id=rid;
  PERFORM sync_audit_resource_epoch('virtual_server',rid,pid,tid,NOT FOUND);
 END IF;
 RETURN NULL;
END $function$
```

### `maintain_project_audit_epochs`

Proje sahibi takım değişince projedeki sanal sunucuların epoch kayıtlarını yeniler.

```sql
CREATE OR REPLACE FUNCTION public.maintain_project_audit_epochs()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
DECLARE v record;
BEGIN
 IF NEW.owner_team_id IS NOT DISTINCT FROM OLD.owner_team_id THEN RETURN NULL; END IF;
 FOR v IN SELECT vs.id,vs.project_id,p.owner_team_id FROM virtual_servers vs JOIN projects p ON p.id=vs.project_id WHERE vs.project_id=NEW.id LOOP
  PERFORM sync_audit_resource_epoch('virtual_server',v.id,v.project_id,v.owner_team_id,false);
 END LOOP;
 RETURN NULL;
END $function$
```

### `protect_audit_epoch`

Epoch kimlik ve sahiplik alanlarının değişmesini engeller; açık dönem yalnızca bir kez kapatılabilir.

```sql
CREATE OR REPLACE FUNCTION public.protect_audit_epoch()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
 IF ROW(NEW.epoch_id,NEW.tenant_id,NEW.resource_kind,NEW.resource_id,NEW.project_id,NEW.owner_team_id,NEW.start_revision)
    IS DISTINCT FROM ROW(OLD.epoch_id,OLD.tenant_id,OLD.resource_kind,OLD.resource_id,OLD.project_id,OLD.owner_team_id,OLD.start_revision)
    OR OLD.end_revision IS NOT NULL OR NEW.end_revision IS NULL THEN
  RAISE EXCEPTION 'audit epoch may only be closed once';
 END IF;
 RETURN NEW;
END $function$
```

### `reject_audit_context_update`

Audit context/domain güncellemelerini exception ile engeller.

```sql
CREATE OR REPLACE FUNCTION public.reject_audit_context_update()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN RAISE EXCEPTION 'audit access provenance is immutable'; END $function$
```

### `sync_audit_resource_epoch`

Config revision kilidi altında sahiplik dönemini kapatır/açar; sahiplik değişiminde epoch’a sabitlenmemiş canlı kaynak atamalarını devre dışı bırakır.

```sql
CREATE OR REPLACE FUNCTION public.sync_audit_resource_epoch(k text, rid uuid, pid uuid, tid uuid, removed boolean)
 RETURNS void
 LANGUAGE plpgsql
AS $function$
DECLARE current_epoch audit_resource_epochs; rev bigint; tenant text;
BEGIN
 SELECT revision INTO rev FROM config_revision FOR UPDATE;
 tenant=vmcp_tenant_id();
 SELECT * INTO current_epoch FROM audit_resource_epochs WHERE tenant_id=tenant AND resource_kind=k AND resource_id=rid AND end_revision IS NULL FOR UPDATE;
 IF current_epoch.epoch_id IS NOT NULL THEN
  IF NOT removed AND current_epoch.project_id IS NOT DISTINCT FROM pid AND current_epoch.owner_team_id IS NOT DISTINCT FROM tid THEN RETURN; END IF;
  UPDATE audit_resource_epochs SET end_revision=rev WHERE epoch_id=current_epoch.epoch_id;
  -- Historical epoch-pinned audit assignments survive; live resource grants do not.
  UPDATE authorization_assignments SET enabled=false,version=version+1
   WHERE scope_kind=k AND scope_id=rid AND epoch_id IS NULL AND enabled;
 END IF;
 IF NOT removed THEN
  INSERT INTO audit_resource_epochs(tenant_id,resource_kind,resource_id,project_id,owner_team_id,start_revision) VALUES(tenant,k,rid,pid,tid,rev);
 END IF;
END $function$
```

### `validate_audit_context`

Context tenant bilgisinin bağlı audit olayıyla aynı olmasını zorunlu kılar.

```sql
CREATE OR REPLACE FUNCTION public.validate_audit_context()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
DECLARE recorded_tenant text;
BEGIN
 SELECT tenant_id INTO recorded_tenant FROM audit_events WHERE event_id=NEW.event_id;
 IF recorded_tenant IS NULL OR recorded_tenant<>NEW.tenant_id THEN
  RAISE EXCEPTION 'audit context tenant does not match event';
 END IF;
 RETURN NEW;
END $function$
```

### `validate_audit_domain`

Domain kaynağı, sahiplik ve tenant bilgisini kayıtlı epoch ve resource context ile karşılaştırır.

```sql
CREATE OR REPLACE FUNCTION public.validate_audit_domain()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
DECLARE e audit_resource_epochs; c audit_access_contexts;
BEGIN
 SELECT * INTO e FROM audit_resource_epochs WHERE epoch_id=NEW.epoch_id;
 SELECT * INTO c FROM audit_access_contexts WHERE event_id=NEW.event_id;
 IF e.epoch_id IS NULL OR c.event_id IS NULL OR c.classification<>'resource' OR
    e.tenant_id<>c.tenant_id OR e.resource_kind<>NEW.resource_kind OR e.resource_id<>NEW.resource_id OR
    e.project_id IS DISTINCT FROM NEW.project_id OR e.owner_team_id IS DISTINCT FROM NEW.owner_team_id THEN
  RAISE EXCEPTION 'audit domain does not match its recorded epoch and tenant';
 END IF;
 RETURN NEW;
END $function$
```

### `vmcp_tenant_id`

Tekil runtime kimliğinden text tenant bilgisini döndürür; audit tenant varsayılanıdır. STABLE SQL fonksiyonudur.

```sql
CREATE OR REPLACE FUNCTION public.vmcp_tenant_id()
 RETURNS text
 LANGUAGE sql
 STABLE
AS $function$ SELECT tenant_id FROM tenant_runtime_identity WHERE singleton $function$
```

## Sequence’ler

| Ad | Veri tipi | Başlangıç | Artış | Min | Max | Cache | Cycle |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `audit_events_id_seq` | bigint | 1 | 1 | 1 | 9223372036854775807 | 1 | False |
| `mcp_events_seq_seq` | bigint | 1 | 1 | 1 | 9223372036854775807 | 1 | False |


`audit_events_id_seq` → `audit_events.id`; `mcp_events_seq_seq` → `mcp_events.seq`. Identity/BIGSERIAL sayaçları transaction rollback sonrasında boşluk bırakabilir; iş kimliği/eksiksiz sıra garantisi olarak kullanılmaz. UUID varsayılanları PostgreSQL’in yerleşik `gen_random_uuid()` fonksiyonudur ve ayrı sequence gerektirmez.

## Migration yönetimi ve diğer nesne türleri

`core/internal/store/store.go` migration uygularken Goose (`github.com/pressly/goose/v3 v3.28.0`) kullanır. Goose’un oluşturduğu `goose_db_version` altyapı tablosu migration SQL dosyalarında tanımlı değildir. Kullanılan v3.28.0 bağımlılığının `internal/dialects/postgres.go` kaynağından doğrulanan tanımı:

```sql
CREATE TABLE goose_db_version (
    id integer PRIMARY KEY GENERATED BY DEFAULT AS IDENTITY,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp NOT NULL DEFAULT now()
);
```

| Kolon | Amaç |
| --- | --- |
| `id` | Migration geçmişi satır kimliği; integer, NOT NULL, BY DEFAULT identity. |
| `version_id` | Migration numarası; bigint, NOT NULL. |
| `is_applied` | Uygulanma durumu; boolean, NOT NULL. |
| `tstamp` | Geçmiş kaydı zamanı; timestamp without time zone, NOT NULL, varsayılan now(). |

PK constraint ve destek indeksi `goose_db_version_pkey`, identity sequence’i `goose_db_version_id_seq` olur. FK, CHECK veya kullanıcı trigger’ı tanımlanmaz. Uygulama `MAX(version_id) WHERE is_applied` ile şema uyumunu denetler. Bu tablo ve ona ait indeks/sequence yukarıdaki uygulama nesne sayılarına dahil edilmemiştir; Goose altyapısıyla toplam **48 tablo, 353 kolon, 48 PK, 96 indeks ve 3 sequence** bulunur.

Migration kilidi `pg_advisory_lock(784321901)` ile alınır ve işlem sonunda bırakılır. Delivery admission, abonelik ve mock eşitleme kodlarında da advisory lock kullanılır; bunlar kalıcı şema objesi değildir. `vmcp_config` LISTEN/NOTIFY kanalı da tablo veya kalıcı kuyruk değildir.

Migration’lar özel schema, view, materialized view, enum/domain/custom type, stored procedure, extension, partition, RLS policy veya scheduled SQL job tanımlamaz. Uygulama tabloları `public` altında oluşur. `plpgsql`, `jsonb`, UUID, timestamp, array, CIDR/inet benzeri kullanılan tipler PostgreSQL altyapısıdır; kullanıcı tanımlı tip değildir. PostgreSQL’in dahili TOAST nesneleri bu işlevsel envantere dahil değildir.

## Başlangıç verileri ve uygulama izin kataloğu

- `tenants`: default / Default tenant kaydı.
- `projects`: default / Default proje kaydı; eski sanal sunucular buraya bağlanır.
- `tenant_runtime_identity`: başlangıç text tenant değeri `default`.
- `config_revision`, `authorization_state`, `audit_settings`, `invocation_security`, `admission_settings`: tekil başlangıç kayıtları. Revision değerleri migration trigger’larının çalışmasıyla artabilir.
- `admission_settings.settings`: `{"total":1024,"perServer":128,"perPrincipal":16}`.
- Migration 38 mevcut kaynaklar için sahiplik epoch başlangıcı oluşturur; geçmiş olayları güncel sahiplikle yeniden yazmaz.
- Migration 39 eski browser oturumlarını model_version=0 bırakır; yeni claim modelinde yeniden giriş gerekir.
- Migration 40 legacy/shadow yetkilendirme modlarını kaldırır; `authorization_state.mode`, `audit_payload_mode` ve `authorization_mode_revision()` artık yoktur.
- Migration 41 yerleşik iş akışı rollerinin bağımlı izinlerini tamamlar. Migration 42 `platform-admin` rolünü kimliğini koruyarak `platform-operator` yapar ve `access.*` izinlerini bu rolden çıkarır. Geniş operasyon rolü platform yöneticisi kimliği sağlamaz.
- Migration 43–46 invocation/admission tablolarını ve izinlerini ekler. Eski approval-hook mekanizması güncel invocation güvenlik modelinin parçası değildir; belgede eski mekanizma için aktif DB objesi varsayılmamıştır.

### Yerleşik uygulama rolleri ve nihai izinleri

| Rol slug | Ad | İzinler |
| --- | --- | --- |
| `access-approver` | Access Approver | `tool_grants.approve`, `tool_grants.read`, `tool_grants.reject`, `tool_grants.revoke` |
| `auditor` | Auditor | `audit.events.read` |
| `automation-deployer` | Automation Deployer | `audit.settings.read`, `config.apply`, `config.export`, `config.validate`, `identity_bindings.read`, `projects.read`, `teams.read`, `tool_grants.read`, `upstreams.activation.update`, `upstreams.catalog.read`, `upstreams.catalog.refresh`, `upstreams.connection.update`, `upstreams.create`, `upstreams.credentials.update`, `upstreams.identity_forwarding.update`, `upstreams.read`, `upstreams.rest.manage`, `upstreams.tls.update`, `upstreams.update`, `virtual_servers.client_features.update`, `virtual_servers.create`, `virtual_servers.policy.update`, `virtual_servers.publication.update`, `virtual_servers.read`, `virtual_servers.update` |
| `credential-manager` | Credential Manager | `agent_keys.access.update`, `agent_keys.constraints.update`, `agent_keys.create`, `agent_keys.read`, `agent_keys.revoke`, `agent_keys.rotate`, `agent_keys.update`, `identity_bindings.access.update`, `identity_bindings.constraints.update`, `identity_bindings.create`, `identity_bindings.read`, `identity_bindings.rules.update`, `identity_bindings.update`, `virtual_servers.read` |
| `observer` | Observer | `platform.info.read`, `projects.read`, `teams.read`, `upstreams.catalog.read`, `upstreams.read`, `virtual_servers.read` |
| `platform-operator` | Platform Operator | `admission.read`, `admission.update`, `agent_keys.access.update`, `agent_keys.audit_correlation.update`, `agent_keys.constraints.update`, `agent_keys.create`, `agent_keys.delete`, `agent_keys.read`, `agent_keys.revoke`, `agent_keys.rotate`, `agent_keys.update`, `audit.capture.update`, `audit.details.read`, `audit.diagnostics.denied.read`, `audit.diagnostics.update`, `audit.durability.update`, `audit.events.read`, `audit.export_policy.update`, `audit.identities.read`, `audit.legacy.events.read`, `audit.legacy.payloads.read`, `audit.legacy.payloads.unclassified.read`, `audit.payloads.request.read`, `audit.payloads.response.read`, `audit.redaction.global.update`, `audit.redaction.local.update`, `audit.redaction.preview`, `audit.retention.update`, `audit.settings.read`, `audit.status.read`, `config.apply`, `config.export`, `config.prune`, `config.validate`, `connections.test_unsaved`, `control_tokens.access.update`, `control_tokens.constraints.update`, `control_tokens.create`, `control_tokens.delete`, `control_tokens.read`, `control_tokens.revoke`, `control_tokens.rotate`, `control_tokens.update`, `identity_bindings.access.update`, `identity_bindings.constraints.update`, `identity_bindings.create`, `identity_bindings.delete`, `identity_bindings.evaluate`, `identity_bindings.read`, `identity_bindings.rules.update`, `identity_bindings.update`, `invocation_security.manage`, `invocation_security.read`, `mock.provider.read`, `omni.preview`, `omni.reindex`, `omni.status.details.read`, `omni.status.read`, `ownership.transfer`, `platform.info.read`, `projects.manage`, `projects.read`, `rest.import`, `runtime_jwts.revoke`, `sessions.revoke`, `teams.manage`, `teams.read`, `tool_catalog.read`, `tool_grants.approve`, `tool_grants.read`, `tool_grants.reject`, `tool_grants.request`, `tool_grants.revoke`, `upstreams.activation.update`, `upstreams.catalog.read`, `upstreams.catalog.refresh`, `upstreams.connection.update`, `upstreams.create`, `upstreams.credentials.update`, `upstreams.delete`, `upstreams.identity_forwarding.update`, `upstreams.invocation_security.manage`, `upstreams.invocation_security.read`, `upstreams.mock.read`, `upstreams.mock.sync`, `upstreams.read`, `upstreams.rest.execute`, `upstreams.rest.manage`, `upstreams.rest.preview`, `upstreams.test`, `upstreams.tls.update`, `upstreams.update`, `virtual_servers.client_features.update`, `virtual_servers.create`, `virtual_servers.delete`, `virtual_servers.policy.update`, `virtual_servers.publication.update`, `virtual_servers.read`, `virtual_servers.update` |
| `upstream-maintainer` | Upstream Maintainer | `upstreams.catalog.read`, `upstreams.catalog.refresh`, `upstreams.connection.update`, `upstreams.read`, `upstreams.test`, `upstreams.update` |
| `virtual-server-editor` | Virtual Server Editor | `projects.read`, `tool_catalog.read`, `tool_grants.read`, `tool_grants.request`, `virtual_servers.create`, `virtual_servers.read`, `virtual_servers.update` |


### Tüm izinler

| İzin kodu | Desteklenen kapsamlar | Capture kısıtı |
| --- | --- | --- |
| `access.assignments.manage` | tenant | Hayır |
| `access.assignments.read` | tenant | Hayır |
| `access.evaluate` | tenant | Hayır |
| `access.mappings.manage` | tenant | Hayır |
| `access.mappings.read` | tenant | Hayır |
| `access.roles.manage` | tenant | Hayır |
| `access.roles.read` | tenant | Hayır |
| `admission.read` | tenant | Hayır |
| `admission.update` | tenant | Hayır |
| `agent_keys.access.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `agent_keys.audit_correlation.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `agent_keys.constraints.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `agent_keys.create` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `agent_keys.delete` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `agent_keys.read` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `agent_keys.revoke` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `agent_keys.rotate` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `agent_keys.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `audit.capture.update` | tenant | Hayır |
| `audit.details.read` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `audit.diagnostics.denied.read` | tenant | Hayır |
| `audit.diagnostics.update` | tenant | Hayır |
| `audit.durability.update` | tenant | Hayır |
| `audit.events.read` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `audit.export_policy.update` | tenant | Hayır |
| `audit.identities.read` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `audit.legacy.events.read` | tenant | Hayır |
| `audit.legacy.payloads.read` | tenant | Hayır |
| `audit.legacy.payloads.unclassified.read` | tenant | Hayır |
| `audit.payloads.request.read` | tenant, team, project, upstream, virtual_server, unmanaged | Evet |
| `audit.payloads.response.read` | tenant, team, project, upstream, virtual_server, unmanaged | Evet |
| `audit.redaction.global.update` | tenant | Hayır |
| `audit.redaction.local.update` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `audit.redaction.preview` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `audit.retention.update` | tenant | Hayır |
| `audit.settings.read` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `audit.status.read` | tenant | Hayır |
| `config.apply` | tenant | Hayır |
| `config.export` | tenant | Hayır |
| `config.prune` | tenant | Hayır |
| `config.validate` | tenant | Hayır |
| `connections.test_unsaved` | tenant | Hayır |
| `control_tokens.access.update` | tenant | Hayır |
| `control_tokens.constraints.update` | tenant | Hayır |
| `control_tokens.create` | tenant | Hayır |
| `control_tokens.delete` | tenant | Hayır |
| `control_tokens.read` | tenant | Hayır |
| `control_tokens.revoke` | tenant | Hayır |
| `control_tokens.rotate` | tenant | Hayır |
| `control_tokens.update` | tenant | Hayır |
| `identity_bindings.access.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `identity_bindings.constraints.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `identity_bindings.create` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `identity_bindings.delete` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `identity_bindings.evaluate` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `identity_bindings.read` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `identity_bindings.rules.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `identity_bindings.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `invocation_security.manage` | tenant | Hayır |
| `invocation_security.read` | tenant | Hayır |
| `mock.provider.read` | tenant | Hayır |
| `omni.preview` | tenant | Hayır |
| `omni.reindex` | tenant | Hayır |
| `omni.status.details.read` | tenant | Hayır |
| `omni.status.read` | tenant | Hayır |
| `ownership.transfer` | tenant, team, upstream, unmanaged, project, virtual_server | Hayır |
| `platform.info.read` | tenant | Hayır |
| `projects.manage` | tenant, team, project, unmanaged | Hayır |
| `projects.read` | tenant, team, project, unmanaged | Hayır |
| `rest.import` | tenant | Hayır |
| `runtime_jwts.revoke` | tenant | Hayır |
| `sessions.revoke` | tenant | Hayır |
| `teams.manage` | tenant, team | Hayır |
| `teams.read` | tenant, team | Hayır |
| `tool_catalog.read` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `tool_grants.approve` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `tool_grants.read` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `tool_grants.reject` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `tool_grants.request` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `tool_grants.revoke` | tenant, team, project, upstream, virtual_server, unmanaged | Hayır |
| `upstreams.activation.update` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.catalog.read` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.catalog.refresh` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.connection.update` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.create` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.credentials.update` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.delete` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.identity_forwarding.update` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.invocation_security.manage` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.invocation_security.read` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.mock.read` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.mock.sync` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.read` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.rest.execute` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.rest.manage` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.rest.preview` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.test` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.tls.update` | tenant, team, upstream, unmanaged | Hayır |
| `upstreams.update` | tenant, team, upstream, unmanaged | Hayır |
| `virtual_servers.client_features.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `virtual_servers.create` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `virtual_servers.delete` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `virtual_servers.policy.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `virtual_servers.publication.update` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `virtual_servers.read` | tenant, team, project, virtual_server, unmanaged | Hayır |
| `virtual_servers.update` | tenant, team, project, virtual_server, unmanaged | Hayır |


## PostgreSQL rolleri ve GRANT’ler (isteğe bağlı deployment)

`deploy/postgres/audit-roles.sql` migration sonrasında DB sahibi tarafından ayrıca çalıştırılır. Bu roller boş şemaya migration uygulanınca otomatik oluşmaz. LOGIN kullanıcıları ve TLS kimlikleri deployment tarafından ayrıca sağlanır. Aşağıdaki dosya yalnızca NOLOGIN yetki grupları oluşturur; uygulama rolleriyle (`authorization_roles`) aynı nesneler değildir.

| DB rolü | Amaç |
| --- | --- |
| `vmcp_audit_query` | Audit metadata ve provenance okuma; payload okuma izni verilmez. |
| `vmcp_audit_export` | Audit export kuyruğunu işleme, kimlik doğrulama ve payload dahil export. |
| `vmcp_audit_retention` | Retention süreci: hold/consumer durumunu okuyup uygun olay/payload kayıtlarını silme. |


Kaynak dosyanın tam rol/yetki tanımı:

```sql
-- Run as the database owner after migrations. Provision LOGIN roles and TLS
-- credentials separately; never grant application roles membership in owner.
CREATE ROLE vmcp_audit_query NOLOGIN;
CREATE ROLE vmcp_audit_export NOLOGIN;
CREATE ROLE vmcp_audit_retention NOLOGIN;
GRANT USAGE ON SCHEMA public TO vmcp_audit_query, vmcp_audit_export, vmcp_audit_retention;
-- Export startup verifies the dedicated database identity. INSERT defaults use
-- vmcp_tenant_id(), which runs with the caller's identity-table privileges.
GRANT SELECT ON tenant_runtime_identity TO vmcp_audit_export;
GRANT SELECT ON tenants TO vmcp_audit_export;
GRANT SELECT ON audit_events TO vmcp_audit_query, vmcp_audit_export, vmcp_audit_retention;
GRANT SELECT,INSERT,UPDATE ON audit_export_state,audit_deliveries TO vmcp_audit_export;
GRANT SELECT ON audit_export_state,audit_deliveries,audit_legal_holds TO vmcp_audit_retention;
GRANT DELETE ON audit_events TO vmcp_audit_retention;
-- The gateway's separately provisioned application LOGIN role needs INSERT on
-- audit_events plus USAGE on audit_events_id_seq, not UPDATE/DELETE on audit_events.
-- Retention SELECT FOR UPDATE also needs UPDATE on at least one audit_events
-- column in PostgreSQL; isolate this privilege to the retention role.
GRANT UPDATE(id) ON audit_events TO vmcp_audit_retention;
-- A privileged evidence operator owns legal-hold creation/removal and consumer
-- activation/deactivation. Those powers are deliberately absent above.

-- Payload tables remain separate from metadata query access.
GRANT SELECT ON audit_settings TO vmcp_audit_export;
GRANT SELECT ON audit_payloads TO vmcp_audit_export, vmcp_audit_retention;
GRANT DELETE ON audit_payloads TO vmcp_audit_retention;
GRANT UPDATE(event_id) ON audit_payloads TO vmcp_audit_retention;

-- Snapshot provenance is metadata; query identities still receive no payloads.
GRANT SELECT ON audit_access_contexts,audit_access_domains,audit_resource_epochs TO vmcp_audit_query;
-- A separately provisioned gateway writer also needs INSERT on
-- audit_access_contexts/audit_access_domains, SELECT on those context rows and
-- audit_resource_epochs, and SELECT(event_id,tenant_id) on audit_events for
-- provenance validation triggers. Data-plane snapshot readers need SELECT on
-- audit_resource_epochs. Do not grant UPDATE on immutable context/domain rows.
```

`GRANT UPDATE(id)` ve `UPDATE(event_id)` retention’ın SELECT FOR UPDATE gereksinimi içindir. Legal hold oluşturma/silme ve export tüketicisi etkinleştirme yetkileri bu gruplara ayrıca verilmez. Gateway audit writer’ı için gereken INSERT/sequence USAGE ve provenance okuma izinleri dosya yorumlarında belirtilir; LOGIN adı ortamdan geldiği için migration belirli bir uygulama kullanıcı adı oluşturmaz.

## Demo/benchmark PostgreSQL nesneleri

`benchmark/postgres/init.sql` yalnızca yerel Keycloak demosu için `keycloak` LOGIN kullanıcısı ve sahibi bu kullanıcı olan `keycloak` veritabanını oluşturur. Uygulama şemasına ait tablo değildir; demo parolası bu belgeye kopyalanmamıştır. Keycloak’ın kendi oluşturduğu tablolar bu repoda migration olarak tanımlanmadığından vMCP envanterine dahil değildir. Compose/Helm ortamındaki veritabanı adları ve LOGIN kullanıcıları kurulum parametreleridir. Benchmark veya test sırasında geçici oluşturulan fixture/veritabanları kalıcı vMCP objesi sayılmamıştır.

## Yeniden doğrulama yöntemi

1. Uygulamadan bağımsız boş PostgreSQL 17 veritabanı açın.
2. `core/migrations` dosyalarını numara sırasıyla, yalnızca Up yönünde uygulayın; her migration’ı transaction içinde çalıştırın. Gerçek kurulumda uygulamanın Goose migration akışını tercih edin.
3. `pg_attribute`/`pg_attrdef` ile kolon ve varsayılanları, `pg_constraint` + `pg_get_constraintdef` ile kısıtları, `pg_indexes` ile indeksleri, `pg_trigger` + `pg_get_triggerdef` ile kullanıcı trigger’larını, `pg_proc` + `pg_get_functiondef` ile fonksiyonları, `pg_sequences` ile sayaçları karşılaştırın.
4. Kurulu ortama özel drift incelemesi gerekiyorsa ayrıca yetkili ve salt okunur katalog erişimiyle karşılaştırın; bu belge canlı ortamda manuel değişiklik olmadığını garanti etmez.
