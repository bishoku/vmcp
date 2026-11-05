GitHub takip kaydı: [#31](https://github.com/bishoku/vmcp/issues/31). Uygulama ve uzak sunucu kabulü için branch: `feat/31-v2-control-plane-authorization`. Main üzerinde commit/merge veya v2.0.0 tag/release oluşturulmaz; yayın kararı sunucu doğrulamalarından sonra repo sahibine aittir.

Güncel major release sözleşmesi bölüm 25, son uygulama dilimleri bölüm 26 ve 27'dedir. Bölüm 27 önceki son-admin-ataması korumasını yürürlükten kaldırır. Önceki bölümler uygulama tarihçesini içerir; legacy/shadow rollout, otomatik eski-rol aktarımı ve aktivasyon adımları artık geçerli ürün hedefleri değildir.

# Control plane dinamik yetkilendirme uygulama planı

Tarih: 2026-11-04. Durum: tasarım ve uygulama planı; uygulama kodu değiştirilmedi.

Bu belge mevcut kodun endpoint/alan envanterini, önerilen izin kataloğunu, custom rol ve claim/scope eşlemelerini, admin arayüzünü ve migration planını birleştirir. İzin isimleri ve yeni API sözleşmeleri öneridir; mevcut ürün yetenekleri olarak yorumlanmamalıdır.

## 1. Hedef ve değişmezler

Admin control plane üzerinden custom roller oluşturabilmeli, uygulamanın tanımladığı izinleri seçebilmeli ve bu rolleri doğrulanmış kimliklere veya claim/scope eşlemelerine kaynak kapsamında atayabilmelidir.

- İzinleri uygulama kodu tanımlar; admin izinleri roller halinde birleştirir.
- Karar: doğrulanmış kimlik + işlem izni + kaynak kapsamı. Varsayılan ret uygulanır.
- Birden fazla atamanın sonuçları birleşir; bir atamadaki izin başka atamadaki kapsamla birleştirilmez.
- Takım üyeliği kaynak aidiyetini çözer; yeni modelde tek başına yazma yetkisi üretmez.
- Kaynak okuma, değiştirme, güvenlik politikası, credential ve sahiplik işlemleri ayrılır.
- Control plane rolü runtime MCP erişimi üretmez. Agent kimliği, virtual-server politikaları ve upstream tool grant kuralları ayrıca geçerlidir.
- İlk sürümde rol mirası, explicit deny, serbest script ve claim'den dinamik kaynak kimliği üretimi yoktur.
- Audit izleme hakkı request/response mesajlarını görme hakkı vermez. Redakte edilmiş payload da hassas kabul edilir.

## 2. Kod üzerinden mevcut durum

| Kaynak | İncelenen davranış |
|---|---|
| `core/internal/controlplane/server.go` | Route kayıtları; `wrap`, `permit`, `authorize`; global roller, takım üyeliği yükseltmesi, GitOps/audit-status istisnaları |
| `core/internal/store/ownership.go` | Owner/project filtreleri, platformOnly, çoklu server credential kapsamı, sahiplik transferleri |
| `core/internal/store/tool_grants.go` | Consumer tarafında talep, upstream tarafında karar; aynı takım/unmanaged otomatik onay |
| `core/internal/controlplane/upstreams.go` | Create/PUT/PATCH alanları, TLS/OAuth write-only secret semantiği |
| `core/internal/controlplane/virtualservers.go` | İçerik, politika, publication, proje ve client feature alanları |
| `core/internal/controlplane/agentkeys.go`, `bindings.go`, `admintokens.go` | Credential alanları ve secret üretimi |
| `core/internal/controlplane/rest.go`, `mock_upstreams.go`, `connect.go` | Yan etkili REST testleri, mock sync, kaydedilmemiş bağlantı testleri |
| `core/internal/controlplane/declarative.go` | Export/apply; governance ve auditSettings; secret koruma |
| `core/internal/controlplane/audit.go`, `audit_settings.go` | Audit filtreleri, preview, settings ve event-ID üzerinden payload çözme |
| `core/internal/store/audit.go` | `auditWhere`: non-admin için erişilebilir virtual server VEYA kendi actor kaydı; gruplamadan önce filtreleme |
| `core/internal/audit/policy.go` | Capture scopes, yönler, redaction, exportPayloads ve retention ayarları |
| `web/src/interaction.tsx`, `App.tsx`, `AuditSettingsPanel.tsx` | Genel edit/manageTokens yaklaşımı; rol/takım birleşimi; request/response preview |

Global viewer/editor unmanaged kaynaklarda legacy erişime sahiptir; takım sahipliğini aşmaz. Takım üyeliği viewer kullanıcıya kendi takımında yazma hakkı ekleyebilir. Admin tüm kaynakları yönetir. Control token `read/admin` scope'ları viewer/admin'e dönüşür.

`permit` audit settings/preview/payload ve Omni reindex işlemlerini admin ile sınırlar; Omni preview bir POST okuma istisnasıdır ve ayrıca runtime kimliği ister. REST execution test ve mock sync handler içinde ayrıca admin kontrolü yapar. Takım varken tenant export/apply/audit-status admin gerektirir. Bu istisnalar yalnız HTTP yöntemiyle taşınamaz.

## 3. Kapsam ve karar sözleşmesi

Kapsamlar: tenant, team, project, upstream, virtual_server ve legacy unmanaged. İzin kataloğu her izin için geçerli kapsamları açıkça belirtir. Project upstream sahipliği üretmez; upstream bağımsız team kaynağıdır.

- Team kapsamı o takımın upstream'lerini ve sahip olduğu projelerin server'larını kapsar.
- Project kapsamı projenin server'larını kapsar; bunların kullandığı başka takım upstream'lerinin bağlantı sırlarını kapsamaz.
- Key/binding üzerinde işlem, eriştiği tüm server'lar için yetki gerektirir. Kapsamsız legacy credential için unmanaged/tenant kuralı açıkça uygulanır.
- Create sırasında hedef proje/takım ve hassas başlangıç alanları kontrol edilir; henüz var olmayan kaynak için kaynak-ID ataması yeterli sayılmaz.
- PUT/PATCH mevcut kayıtla normalize edilmiş sonuç üzerinden karşılaştırılır. Silme, null, varsayılan değer ve iç içe alan temizleme de değişikliktir.
- Yetkisiz alan varsa tüm istek reddedilir. Kısmi alan kaydetme yapılmaz.
- Transfer kaynak ve hedef kapsamlarını kontrol eder; eski grant/atama etkisini önizler. Slug yerine kararlı kaynak kimliği kullanılır; silinip aynı slug ile oluşturulan kaynak eski hakları devralmaz.
- İç sistem worker'ları ayrı güven bağlamı kullanır. Mevcut access-context yokluğunun geniş erişim verdiği yollar kullanıcı isteğine taşınamaz.

## 4. Endpoint–izin matrisi

Aşağıdaki yollar aksi belirtilmedikçe `/api/v1` altındadır. `read` listelerde SQL/kapsam filtresi, detayda aynı kaynağa kontrol demektir. Güncelleme izinleri bölüm 5'teki alan izinleriyle birlikte değerlendirilir. Kimlik doğrulama, CSRF, ETag ve mevcut ağ kontrolleri ayrıca korunur.

### 4.1 Kimlik, platform ve governance

| Endpoint | Önerilen izin | Kapsam / not |
|---|---|---|
| `GET /healthz`, `GET /readyz` (prefix yok) | Kamu probe; RBAC dışı | Mevcut sağlık sözleşmesi |
| `GET /auth/config`, `/auth/login`, `/auth/callback`; `POST /auth/logout` | Auth akışı; kaynak RBAC dışı | Logout oturum/CSRF kontrollerini korur |
| `GET /me` | Doğrulanmış kimliğin kendisi | Claim/secret dökümü değil; etkili capabilities |
| `GET /info`, `/openapi.json`, `/openapi.yaml` | `platform.info.read` | API'ye kabul edilmiş kimlik; metadata |
| `GET /teams` | `teams.read` | Görünen takımlar; issuer/group ayrıntıları assignment metadata olarak değerlendirilir |
| `POST /teams` | `teams.manage` | İlk sürüm platform admin; issuer/group değişimi erişim değişikliğidir |
| `GET /projects` | `projects.read` | Görünen projeler |
| `POST /projects` | `projects.manage` + owner değişirse `ownership.transfer` | Default proje unmanaged kalır |
| `GET /tool-catalog` | `tool_catalog.read` | Paylaşılan keşif metadata'sı; bağlantı/secret erişimi vermez |
| `GET /tool-grants` | `tool_grants.read` | Consumer veya upstream tarafındaki yetkili kapsam |
| `POST /tool-grants` | pending: `tool_grants.request`; approved/rejected/revoked: ilgili `approve/reject/revoke` | Talep consumer server, karar upstream kapsamı |
| `POST /tool-grants/bulk` | Her öğe için tekil işlem izni | Mevcut öğe bazlı commit/sonuç ve version semantiği korunur |

### 4.2 Upstream ve test işlemleri

| Endpoint | Önerilen izin | Kapsam / not |
|---|---|---|
| `GET /upstreams`, `/upstreams/{slug}` | `upstreams.read` | Team/upstream/unmanaged; secret'lar okunmaz |
| `POST /upstreams` | `upstreams.create` + başlangıç alan izinleri | Hedef team/unmanaged |
| `PUT`, `PATCH /upstreams/{slug}` | `upstreams.update` + değişen alan izinleri | Aynı transaction'da güncel kaynak |
| `DELETE /upstreams/{slug}` | `upstreams.delete` | Bağımlılık/grant etkileri korunur |
| `GET /upstreams/{slug}/tools`, `/catalog` | `upstreams.catalog.read` | Bağlantı yönetiminden ayrı katalog erişimi |
| `POST /upstreams/{slug}/refresh-tools` | `upstreams.catalog.refresh` | Harici ağ erişimi + kayıtlı katalog mutasyonu |
| `POST /connection-tests` | `connections.test_unsaved` | Tenant düzeyi ağ kullanımı; supplied URL/TLS/auth; salt okuma sayılmaz |
| `POST /upstreams/{slug}/connection-test` | `upstreams.test` | Kayıtlı credential'ı kullanır ama açığa çıkarmaz |
| `POST /rest-imports` | `rest.import` | Doküman dönüştürme, kalıcı kayıt yapmaz; save izinlerini sağlamaz |
| `POST /upstreams/{slug}/rest/preview` | `upstreams.rest.preview` | arguments ve üretilen header/body hassas olabilir; mevcut masking doğrulanır |
| `POST /upstreams/{slug}/rest/test` | `upstreams.rest.execute` | Gerçek yan etki/yanıt; ETag ve caller-context kısıtları korunur; agent taklidi yapmaz |
| `GET /mock-provider` | `mock.provider.read` | İç provider URL görünürlüğü ayrıca sınırlandırılır |
| `GET /upstreams/{slug}/mock/status` | `upstreams.mock.read` | Yalnız ilgili namespace'in durumu |
| `POST /upstreams/{slug}/mock/sync` | `upstreams.mock.sync` | Harici mutasyon; ETag korunur; diğer action değerleri reddedilir |

### 4.3 Virtual server ve credential

| Endpoint | Önerilen izin | Kapsam / not |
|---|---|---|
| `GET /virtual-servers`, `/virtual-servers/{slug}` | `virtual_servers.read` | Project/team/server/unmanaged |
| `POST /virtual-servers` | `virtual_servers.create` + başlangıç alan izinleri | Hedef proje |
| `PUT`, `PATCH /virtual-servers/{slug}` | `virtual_servers.update` + alan izinleri | Politika/publication ayrı |
| `DELETE /virtual-servers/{slug}` | `virtual_servers.delete` | Kaynak ve bağlı erişim etkileri |
| `GET /agent-keys`, `/agent-keys/{slug}` | `agent_keys.read` | Metadata; tüm hedefler için kapsam |
| `POST /agent-keys` | `agent_keys.create` + `agent_keys.access.update` | Yeni secret yalnız bu yanıtla alınır |
| `PATCH /agent-keys/{slug}` | `agent_keys.update` + alan izinleri | virtualServers/sınırlar ayrıca kontrol |
| `DELETE /agent-keys/{slug}` | `agent_keys.delete` | Tüm hedefler |
| `POST /agent-keys/{slug}/rotate`, `/revoke` | `agent_keys.rotate`, `agent_keys.revoke` | Rotate yeni secret üretir |
| `GET /identity-bindings`, `/identity-bindings/{slug}` | `identity_bindings.read` | Runtime claim eşlemesi metadata'sı |
| `POST /identity-bindings` | `identity_bindings.create` + `rules.update` + `access.update` | Tüm hedef server'lar |
| `PUT`, `PATCH /identity-bindings/{slug}` | `identity_bindings.update` + alan izinleri | Claim değişimi erişimi genişletebilir |
| `DELETE /identity-bindings/{slug}` | `identity_bindings.delete` | Tüm hedefler |
| `POST /identity-bindings/evaluate` | `identity_bindings.evaluate` | Yalnız görünür binding sonuçları; gerçek token kabulü değildir |
| `GET /admin-tokens`, `/admin-tokens/{slug}` | `control_tokens.read` | Tenant; metadata |
| `POST /admin-tokens` | `control_tokens.create` + `control_tokens.access.update` | Oluşturucunun verebileceği hakları aşamaz |
| `PATCH /admin-tokens/{slug}` | `control_tokens.update` + scope/atama için `access.update` | Mevcut read/admin scope migration'ı |
| `DELETE /admin-tokens/{slug}` | `control_tokens.delete` | Tenant |
| `POST /admin-tokens/{slug}/rotate`, `/revoke` | `control_tokens.rotate`, `control_tokens.revoke` | Secret ve revoke semantiği korunur |
| `POST /revocations/jwts` | `runtime_jwts.revoke` | İlk sürüm tenant/platform; runtime token içeriği loglanmaz |
| `POST /revocations/sessions` | `sessions.revoke` | İlk sürüm tenant/platform; issuer/subject hedefi |

### 4.4 Audit, Omni ve GitOps

| Endpoint | Önerilen izin | Kapsam / not |
|---|---|---|
| `GET /audit-events` | `audit.events.read` | Metadata; activity/operation/conversation/group sorguları aynı filtreyi kullanır |
| `GET /audit-events/{event}/payload` | `audit.events.read` + kayıt yönüne göre `audit.payloads.request.read` veya `audit.payloads.response.read` | Event'in güvenilir kaynak ve capture scope bilgisi; detaylar bölüm 6 |
| `GET /audit-settings` | `audit.settings.read` | Tenant; yerel ayarlar ayrı kaynak endpoint’lerinden sunulur (11.7) |
| `PUT /audit-settings` | Değişen alanlara göre bölüm 5 izinleri | Revision korunur; tam doküman yerel kapsamı aşamaz |
| `POST /audit-preview` | `audit.redaction.preview` | Sentetik sample; editScope ve kaynak kontrol edilir; kayıtlı payload okuma hakkı vermez |
| `GET /audit-status` | `audit.status.read` | Tenant operasyonel durum; payload yok |
| `GET /omni` | `omni.status.read` | Public endpoint/basic durum; iç index ayrıntıları için `omni.status.details.read` |
| `POST /omni/preview` | `omni.preview` + mevcut runtime kimliği | Yönetim rolü runtime katalog hakkı üretmez |
| `POST /omni/reindex` | `omni.reindex` | Tenant; yan etkili |
| `GET /export` | `config.export` + içerilen kaynakların read izinleri | Secretsız; governance/audit ayarları kendi read izinlerini gerektirir |
| `POST /apply` | `config.apply` + her mutasyon/alan izni | prune ayrıca `config.prune` ve her silme izni; dry-run da görünürlük sınırlarını korur |

Bu tablolar route kayıtlarını kapsar. Gelecekte eklenen route CI envanterinde izinsiz/eşlenmemiş kalırsa kontrol başarısız olmalıdır. Scoped GitOps ilk sürüme alınmamıştır; tenant export/apply kararı bölüm 11.7’de kesindir.

## 5. Alan–izin matrisi

İzinler aynı namespace altında tam isimle yazılmıştır. Genel create/update izni ile hassas alan izni birlikte gerekir. Slug yaratılış kimliğidir; version/revision kontrol alanıdır, yetki kaynağı değildir.

| Kaynak / mevcut JSON alanları | Ek izin / kural |
|---|---|
| Upstream `name`, `description` | `upstreams.update` |
| `url`, `fallbackUrls`, `transport`, `kind` | `upstreams.connection.update`; güvenilir ağ/credential hedefini değiştirir |
| `enabled` | `upstreams.activation.update` |
| `headers` | `upstreams.credentials.update`; tamamı potansiyel secret, header adından güven çıkarılmaz |
| `auth.type/tokenUrl/clientId/clientSecret/scopes/audience` | `upstreams.credentials.update`; secret olmasa da credential'ın kime gönderileceğini değiştirir |
| `tls.caPem/serverName/clientCertPem/clientKeyPem` | `upstreams.tls.update`; key yazımı için ayrıca `upstreams.credentials.update` |
| `requestHeaderMappings`, `callerContext` | `upstreams.identity_forwarding.update`; kimlik/header aktarımı |
| `rest` (operation, request/response mapping ve mock dahil tüm alt yapı) | `upstreams.rest.manage`; secret-bearing alt alan ayrıca credentials; mock publication ayrıca mock.sync |
| `ownerTeam` | `ownership.transfer`; source+target; unmanaged'a geçiş platform |
| Virtual server `name`, `description`, `guidance`, `tools`, `catalog` | `virtual_servers.update`; guidance/tools/catalog tüm alt alanları kapsar; tool grant aşılmaz |
| `enabled`, `omniEnabled` | `virtual_servers.publication.update` |
| `authRequired`, `pii`, `policy`, `rateLimitPerMinute` | `virtual_servers.policy.update` |
| `clientFeatures` | `virtual_servers.client_features.update` |
| `project` | `ownership.transfer`; source+target project |
| Agent key `description` | `agent_keys.update` |
| `virtualServers` | `agent_keys.access.update`; önceki ve sonraki tüm hedefler |
| `ipAllowlist`, `expiresAt`, `rateLimitPerMinute` | `agent_keys.constraints.update`; null ile sınır kaldırma dahil |
| `auditCorrelation` | `agent_keys.audit_correlation.update`; izleme kimliği davranışı |
| Binding `description` | `identity_bindings.update` |
| `conditions` | `identity_bindings.rules.update`; issuer/claim koşul genişlemesi |
| `virtualServers` | `identity_bindings.access.update` |
| `ipAllowlist`, `rateLimitPerMinute`, `enabled` | `identity_bindings.constraints.update` |
| Control token `description` | `control_tokens.update` |
| `scope` ve yeni role/scope assignments | `control_tokens.access.update`; tenant ve verilebilir hak sınırı |
| `expiresAt` | `control_tokens.constraints.update` |
| Team `name` | `teams.manage` |
| Team `issuer`, `group` | `teams.manage` + `access.mappings.manage`; üyelik etkisi önizlenir |
| Project `name` | `projects.manage` |
| Project `ownerTeam` | `ownership.transfer` |
| Tool grant `virtualServer/upstream/tool` | Sabit bileşik kimlik; status işlemine göre consumer/upstream kapsamı |
| Tool grant `status`, `reason` | request/approve/reject/revoke; version mevcut davranışı korur |
| Audit `mode` | `audit.durability.update`; tenant |
| Audit `retentionDays`, `maxPayloadBytes` | `audit.retention.update`; tenant |
| Audit `scopes.*.request/response` | `audit.capture.update`; capture scope + yön |
| Audit `rules` | `audit.redaction.global.update`; tenant |
| Audit `virtualServers[slug]`, `upstreams[slug]` | `audit.redaction.local.update`; yalnız hedef kaynak; diğer map girdileri değişemez |
| Audit `exportPayloads` | `audit.export_policy.update`; dışarı veri çıkışı; görüntüleme hakkından bağımsız |
| Audit `omni` (`includeDenied`, `maxCandidates`) | `audit.diagnostics.update`; tenant; denied candidate ayrıntıları ayrıca korumalı |
| Audit preview `editScope/settings/scope/direction/virtualServer/upstream/payload` | `audit.redaction.preview`; yetkili settings alt kümesi, sentetik veri, sample persist edilmez |

Tam iç REST/policy/guidance şemaları bu alanların ilgili alt ağaç iznine dahildir. Sonraki geliştirmede yeni alt alan eklenirse güvenlik etkisi sınıflandırılmadan genel update'e düşmemelidir.

## 6. Audit izleme ve hassas request/response ayrımı

### 6.1 Mevcut açıklık ve hedef

Bugün `getAuditPayload` event UUID ile `audit_events`/`audit_payloads` kaydını doğrudan sorgular, ciphertext'i çözer ve `state/content` döndürür. Güvenlik üst katmandaki admin şartına dayanır; fonksiyon liste sorgusunun `auditWhere` filtresini kullanmaz. Custom rol açılırken yalnız bu admin şartını gevşetmek yeterli değildir.

Hedef sıra: kimlik → event görünürlüğü → payload yönü/capture scope izni → gerekiyorsa özel diagnostics izni → ciphertext okuma/çözme → yanıt. Yetkisiz istek decrypt aşamasına ulaşmamalıdır.

### 6.2 Audit izinleri ve atama boyutları

| Yetki | Açtığı bilgi | Açmadığı bilgi |
|---|---|---|
| `audit.events.read` | Yetkili event metadata, timeline, filtre ve grup özetleri | Request/response içerikleri |
| `audit.identities.read` | Actor/subject/clientIp/agent kimlik ayrıntıları | Payload |
| `audit.payloads.request.read` | Yetkili olayın kayıtlı request mesajı | Response mesajı |
| `audit.payloads.response.read` | Yetkili olayın kayıtlı response mesajı | Request mesajı |
| `audit.diagnostics.denied.read` | Yetkili Omni diagnostics içindeki yetkisiz aday örnekleri | Kaynak bağlantıları veya başka payload yönleri |
| `audit.status.read` | Depolama/durability/operasyon durumu | Mesajlar |
| `audit.settings.read` ve alan yönetim izinleri | Capture, redaction ve retention yönetimi | Kayıtlı payload okuma |

Request/response izin atamalarına capture scope allowlist'i eklenmelidir: `virtual`, `upstream`, `control`, `reads`, `system`, `omni`, `omni_diagnostics`, `omni_embedding`, `omni_rerank`. Boş/eksik allowlist tüm scope anlamına gelmez. Böylece agent response inceleyen rol otomatik control API veya model-provider yanıtlarını göremez. Bu kısıt role+resource kapsamıyla aynı atama içinde uygulanır.

`activity` UI sınıflandırması ile kayıtlı capture `scope` farklıdır. Upstream isteği listede virtual/control altında görünebilir; payload kontrolü gerçek kayıt scope'u ve yönü üzerinden yapılır. Yön kullanıcının query parametresinden alınmaz. Boş/bilinmeyen direction yön bazlı custom izinlerle açılamaz; yalnız bölüm 11’deki legacy tam-payload izni kullanılabilir.

### 6.3 Metadata ve detay koruması

- Actor/subject/clientIp ayrıntıları `audit.identities.read` yoksa sunucu yanıtında çıkarılır veya maskelenir; bunlarla filtreleme de aynı izni gerektirir.
- `reason`, `target`, action açıklamaları ve kullanıcı kaynaklı metinler hassas veri taşıyabilir. Metadata serializer allowlist/sanitization uygular; raw error/body metadata'ya taşınmaz.
- Correlation ID, trace, operation ve conversation bağlantısı yalnız ilişki gösterir; başka olayları okumaya yetki vermez.
- Conversation sayıları/özetleri yalnız yetkili olaylardan hesaplanır. Gizli olayların toplamı veya içerik snippet'i sızdırılmaz.
- Mevcut “kendi actor kaydını görme” davranışı migration'da açık self kapsamıyla modellenir; kendi isteği olmak payload izni sağlamaz.
- Çapraz takım upstream event'inde yalnız upstream sahipliği bütün consumer mesajını okuma hakkı vermez. Consumer çağrısında payload erişimini consumer domain belirler; upstream sahipliği ek şart değildir. Bağımsız yönetim çağrılarında ilgili target/tenant kapsamı aranır (11.2).
- Tarihsel kaynaklar için olay anındaki kararlı resource/project/team kimliği tutulmalı; silinmiş veya sahipliği değişmiş kaynağın geçmiş payload'ı yeni sahibine otomatik açılmamalı. Eski event'ler yalnız slug'dan tahminle genişletilmez; belirsiz kayıtlar tenant yetkisine ayrılır.
- Geçmiş kayıtlar için otomatik sahiplik backfill yapılmaz; yeni kayıtların immutable bağlamı ve legacy erişim sözleşmesi bölüm 11’de kesinleştirilmiştir.

### 6.4 API, UI ve kayıt yaşam döngüsü

Mevcut payload endpoint korunacaktır: sunucu event yönünü belirler. Yetkisiz/bilinmeyen event için tutarlı 404 politikası uygulanır; yetkili kayıtta expired/not-captured/decryption-unavailable durumları ayrılır. Liste yanıtı event bazlı `canReadRequestPayload/canReadResponsePayload` yeteneklerini içerir; bunlar backend kontrolünün yerine geçmez.

UI request ve response panellerini ayrı yönetir. Yetkisiz içerik API'den indirilmez, prefetch edilmez, gizli DOM'a konulmaz; açma, copy ve download aynı izne bağlıdır. Yetki değişimi/logout/kaynak değişiminde açık içerik ve istemci cache'i temizlenir. Payload yanıtları `Cache-Control: no-store` kullanır. Panel “kayıt yok” ve “yetki yok” durumlarını görünürlük politikasına uygun gösterir.

Redaction kayıt öncesi koruma olarak kalır; payload izni ham/orijinal veriyi geri getirmez. Maskelenmiş payload için de yön bazlı yetki zorunludur. Capture yönetme hakkı mesaj okuma hakkı üretmez. Preview sentetik veriyi dönüştürür; kayıtlı event'i okuyamaz ve sample'ı audit payload olarak yeniden kaydetmez.

Payload okuma/ret olayları metadata olarak audit edilir: actor, event, direction, kapsam, karar ve authorization revision. Görüntülenen içerik bu okumaya ait audit payload'a tekrar yazılmaz; recursive kayıt/sızıntı önlenir.

PostgreSQL export rolü, Kafka/RabbitMQ exporter'ları, object storage ve indirilen arşivler control plane RBAC dışından erişebilir. Mevcut exportPayloads, DB/IAM ve retention kontrolleri ayrıca incelenmeli; UI yetkileri bunları koruyor gibi sunulmamalıdır. Gelecekte kullanıcıya audit export API'si açılırsa ayrı export izni ile aynı scope/yön kontrolleri gerekir.

### 6.5 Örnek audit rolleri

| Rol | İzin |
|---|---|
| Audit Observer | Scoped events metadata; payload yok; identity ayrıntıları ayrıca |
| Request Investigator | Scoped events + request payload; seçili capture scope'ları |
| Response Investigator | Scoped events + response payload; request yok |
| Audit Policy Manager | Settings/redaction/capture; kayıtlı payload yok |
| Platform Admin | Açık tam yetki; kurtarma yolu; audit okumaları da kaydedilir |

## 7. Rol, assignment ve kimlik eşlemeleri

Role: immutable ID, slug, name, description, permissions, built-in/custom, version. Assignment: role ID, subject veya mapping, resource scope, audit capture-scope kısıtları, enabled, version. Principal kimliği kullanıcı için issuer+subject; token için kalıcı token ID'dir. E-posta kalıcı kimlik değildir.

Mapping: trusted provider ID, browser/machine principal türü, claim path, typed condition listesi, role+scope assignments, enabled ve version. Operatörler: exact string, array contains, space-delimited scope token, all-of. Alternatif koşullar ayrı mapping olabilir. Eksik/yanlış türde claim eşleşmez. Provider güveni mapping tarafından oluşturulamaz.

Browser OIDC doğrulanmış sign-in claim'lerini kullanır. İlk sürümde harici OAuth control-plane bearer kabulü yoktur; scope eşlemesi yalnız doğrulanmış OIDC claim kümesine uygulanır (11.8). Mevcut runtime agent tokenları control-plane kimliği olarak kabul edilmez. Mapping testi yalnız simülasyondur, oturum üretmez.

Default şablonlar: Platform Admin, Observer, Upstream Maintainer, Virtual Server Editor, Credential Manager, Access Approver, Auditor, Automation Deployer. Default roller korunur/kopyalanır. Custom roller açık izin listesi taşır; yeni izinler otomatik eklenmez. Kullanımdaki rol silinmeden atamalar taşınır. İlk sürümde rol/mapping/assignment yazma Platform Admin'e ayrılır; delegasyon sonraki tasarımdır.

## 8. Admin arayüzü ve API taslağı

Settings → Access Management: Permissions, Roles, Assignments, Identity mappings, Effective access, Migration.

- İzin editörü işlem açıklaması, desteklenen kapsam ve hassas veri etkisini gösterir.
- Audit request/response seçimi ve capture-scope allowlist'i ayrı görünür; Auditor şablonu payload seçili gelmez.
- Mapping editörü trusted provider ve claim türünü seçtirir, örnek claim ile rol/kapsam sonucunu gösterir.
- Effective access her iznin hangi atamadan geldiğini ve reddin nedenini admin için açıklar; token/claim sırlarını göstermez.
- Atama değişikliği, sahiplik transferi ve rol silme kullanım/erişim etkisini gösterir; ETag/version çatışması kaybı önler.
- Mevcut `/me` genel capabilities; kaynak yanıtları işlem bazlı capabilities döndürür. UI genel edit boolean'ına dayanmaz.

Yeni API aileleri önerisi: `/permissions`, `/roles`, `/role-assignments`, `/identity-mappings`, `/access/evaluate`, `/access/migration-status`. CRUD ve simülasyon ayrı izinlerle korunur. Son yönetim yolunu kaldırma engellenir; mevcut bootstrap kurtarma yolu korunur. Yeni API'ler OpenAPI, audit redaction ve route-policy envanterine birlikte eklenir.

## 9. Migration ve rollout

1. Envanter: global OIDC env eşlemeleri, team issuer/group kayıtları, Default/unmanaged kaynaklar, token scope'ları, aktif session'lar, CI/GitOps, audit capture/export ayarları raporlanır; yedek alınır.
2. Eklemeli şema: roller, permissions, assignments, mappings, authorization revision eklenir. Tekrar çalıştırılabilir ve sabit migration kimlikleri kullanılır.
3. Uyumluluk: global viewer/editor/admin ve takım üyeliğinin birleşik etkisi açık legacy atamalara dönüştürülür. Viewer+team yazma hakkı kaybolmaz. read token geniş payload hakkı kazanmaz; admin token mevcut admin davranışını korur. Tool grant ve runtime credential kapsamları korunur.
4. Env aktarımı: global rol grupları bir kez kontrollü import edilir. Yeni modda DB esas alınır; restart env'den admin değişikliğini geri yazmaz. Replica ayar farkları etkinleştirmeyi durdurur.
5. Audit geçmişi: mevcut kayıtlar değiştirilmeden legacy kabul edilir; otomatik kaynak/sahiplik backfill yapılmaz. Legacy kayıtlar için tenant-only izinler ve yeni kayıtlar için immutable provenance bölüm 11’e göre uygulanır. Yeni request/response ayrımı read/editor/takım üyelerine otomatik payload erişimi vermez.
6. Karşılaştırma modu: eski model karar verir, yeni model izin/ret farkını metadata olarak raporlar. Trafikte görülmeyen endpoint/alan kombinasyonları test edilir. Sadece gözlenen trafik eşitliği yeterli değildir.
7. Doğrulama: SSO, takım-only kullanıcı, viewer+team, token/CI, GitOps, audit observer ve tek yön investigator senaryoları çalıştırılır. Bootstrap ve en az bir admin girişi doğrulanır.
8. Koordineli etkinleştirme: tüm control plane replikaları uyumlu binary'ye geçer, ardından yeni karar modu açılır. Eski session'lar açık uyumluluk bağlamına çevrilir veya yeniden giriş gerektirir; eski cookie role string'i sınırsız hak kaynağı olamaz.
9. Daraltma: admin legacy roller yerine custom roller atar; bu teknik migration'dan ayrı, audit edilebilir bir işlemdir.
10. Rollback: yeni policy yazımı başlamadan önce dönüş kolaydır; custom rol kullanımı sonrası eski binary ile dönüş hakları yanlış yorumlayabilir. Desteklenen rollback sınırı, DB restore ve token/session etkileri upgrade rehberinde yazılır.

Token hash, expiry ve revoked bilgileri korunur; zorunlu toplu rotation yapılmaz. Eski GitOps dokümanında access alanlarının bulunmaması access kayıtlarını silmez. Access prune ayrı açık işlem olmalıdır. Authorization revizyonu cache'leri geçersizleştirir; yerel değişikliklerin yayılım hedefi ölçülür. IdP grup iptali mevcut doğrulanmış claim tazeliği sınırına bağlıdır; anlık senkronizasyon iddia edilmez.

## 10. Uygulama aşamaları ve çıkış kriterleri

| Aşama | Çıktı | Tamamlanma kriteri |
|---|---|---|
| 1 — Envanter ve sözleşme | Bu route/alan matrisi, audit politikası, permission registry taslağı | Her route ve yazılabilir alan açık policy'ye bağlı; hassas testler belirli |
| 2 — Çekirdek | Veri modeli, karar servisi, SQL kapsam filtreleri, revision/cache | Varsayılan ret; atamalar arası kapsam karışmıyor; kullanıcı/internal sınırı korunuyor |
| 3 — Kimlik | OIDC mapping, control token assignment, claim/scope simülasyonu | Yanlış issuer/audience/tür/scope reddi; eski davranış eşdeğerliği |
| 4 — Fonksiyonlar | Tüm route/store, alan farkları, bulk/GitOps ve audit payload yönleri | Doğrudan API ile UI kısıtları aşılamıyor; yetkisiz payload decrypt edilmiyor |
| 5 — UI | Custom rol, mapping, assignment, effective access, audit panelleri | Request-only/response-only/metadata-only kullanıcılarla browser doğrulaması |
| 6 — Migration/yayın | Karşılaştırma aracı, upgrade/rollback rehberi, OpenAPI/docs | Tekrarlı migration, çoklu replica, aktif session/token ve restore senaryoları |

### Zorunlu test matrisi

- Her route için izinli/izinsiz kimlik, aynı/kapsam dışı kaynak; yeni route policy eksikliğini yakalayan sözleşme testi.
- PUT/PATCH/create: null, omitted, default ve nested reset ile credential/policy/ownership değişimi.
- Farklı atamalardaki permission/scope'un yanlış birleşmemesi; çoklu hedef key/binding yönetimi.
- REST preview/test, mock sync, unsaved connection test, refresh gibi POST'ların farklı etkileri.
- Bulk'ta öğe bazlı yetki, version conflict ve mevcut kısmi sonuç sözleşmesi; apply için atomik yetkisiz değişiklik reddi.
- Audit metadata-only, request-only, response-only, iki yön, farklı capture scopes ve scope dışı event UUID.
- Conversation/group/operation ilişkileri ve counts üzerinden bilgi sızıntısı; kendi actor kaydının payload hakkı vermemesi.
- Bilinmeyen yön, silinen/transfer edilen/aynı slug ile yeniden yaratılan kaynak ve legacy provenance.
- Payload isteğinde yetki kontrolünden önce ciphertext/decrypt erişimi olmaması; masked payload'ın da korunması.
- Audit preview, read-capture ve payload-read audit kayıtları üzerinden içerik tekrar kaydı olmaması.
- UI prefetch/cache/copy/download ve açık panelde revoke/logout davranışı.
- Migration tekrar çalıştırma; viewer+team; read/admin token; eski GitOps dosyası; env/DB önceliği; rollback sınırı.

## 11. Kesinleşen uygulama sözleşmeleri

Bu bölüm önceki açık kararların yerini alır. Buradaki veri alanları ve davranışlar uygulanacak tasarımdır; mevcut kodda var oldukları anlamına gelmez.

### 11.1 Mimari dayanak ve audit provenance saklama

`audit.Event` ve `audit_events` bugün slug, tenant, config revision, scope, direction ve parent operation bilgilerini taşır; olay anındaki resource UUID, project UUID ve owner team UUID ilişkisini taşımaz. `config_revision` tek başına geçmiş sahipliği yeniden kuran bir tarihçe değildir. `Persist` sıfır revision için yazma anındaki revision'ı da kullanabilir. Bu yüzden güncel kaynak join'i veya revision sayısı geçmiş yetki kanıtı sayılmayacaktır.

Yeni migration ile append-only yardımcı tablolar eklenecek:

| Kayıt | İçerik |
|---|---|
| `audit_access_contexts` | event_id (PK/FK), tenant_id, context_version, classification (`resource`, `tenant`, `legacy`), origin, operation_config_revision, provenance_state |
| `audit_access_domains` | event_id, domain türü (`consumer`, `provider`, `target`), resource türü/UUID, project UUID, owner team UUID, ownership epoch UUID |
| `audit_resource_epochs` | tenant, resource türü/UUID, epoch UUID, project/team snapshot'ı, başlangıç config revision'ı, kapanış config revision'ı |

Tablo/kolon isimleri migration yazılırken mevcut naming kurallarına uyarlanabilir; semantik sabittir. Domain kayıtları mevcut resource/team/project tablolarına silmeyle cascade olan FK taşımaz. Silinen kaynakların geçmiş bağlamı korunur. Event silinince yardımcı bağlam event retention/hold yaşam döngüsüyle birlikte temizlenir; payload expiry bağlamı silmez. Ciphertext ve event-ID'ye bağlı encryption amacı değiştirilmez.

Producer bağlamı payload ile aynı operasyon başında üretir; event ve context aynı DB transaction/batch içinde atomik yazılır. Admission/outcome aynı operasyon bağlamını kullanır. Strict/relax ve exporter replay yolları bunu birlikte taşır. Eksik bağlamlı event `legacy` kabul edilir; payload scoped rollere açılmaz. Eksik bağlam sonradan güncel sahiplikten doldurulmaz.

Data plane yeni bir ownership DB sorgusunu her çağrıya eklemeyecektir. `LoadSnapshot` ve snapshot tipleri resource/project/team UUID ve epoch bilgisiyle genişletilecek; yetkili immutable snapshot'tan operation context'e kopyalanacaktır. Bugün upstream ID olsa da virtual-server snapshot'ı aynı tam provenance'ı sağlamaz; yalnız mevcut alanları kullanmak yeterli değildir. Snapshot revision kontrolü mevcut freshness/reauthorization akışını korur.

Control plane mutasyonunda target context yetki kontrolünün yapıldığı transaction'dan alınır. Multi-resource apply payload'ı tüm belgeyi içerdiğinden ilk sürümde tenant sınıfıdır; scoped GitOps ayrı sözleşmeye tabidir. Sahiplik transferi payload'ı ilk control-plane uygulamasında konservatif olarak tenant sınıfındadır (bölüm 15); yalnız eski veya yeni tarafın izniyle açılmaz. Çift-domain kapsamlı görünüm bu dilimde uygulanmaz.

### 11.2 Ownership epoch ve tarihsel erişim

Bir kaynağın team veya project aidiyeti değiştiğinde yeni ownership epoch açılır; aynı projede owner team değişirse bağlı server'ların epoch'u da yenilenir. Silme epoch'u kapatır. Aynı slug ile yeniden yaratılan kaynak yeni UUID alır. Transfer sırasında explicit resource yönetim atamaları devre dışı bırakılır; gerekiyorsa hedefte yeniden atanır. Tarihsel audit atamaları bunlardan ayrıdır.

Scoped audit için şu kurallar kesindir:

1. İzinler okuma anındaki güncel atamalardan hesaplanır; eski kullanıcı hakları olay üzerinde dondurulmaz.
2. Team/project ataması olay anındaki team/project UUID'leriyle eşleşir; güncel kaynak sahipliğiyle geçmiş yeniden yazılmaz. Eski takım yetkili araştırmacısı geçmişi okuyabilir; yeni takım yalnız transfer sonrası event'leri doğal olarak görür.
3. Project ataması ayrıca olay anındaki owner-team domain'ine bağlanır. Projenin başka takıma geçmesi eski audit hakkını proje üzerinden taşımayacaktır.
4. Resource düzeyindeki audit ataması oluşturulurken epoch sabitlenir. Eski epoch'a erişim ancak adminin açık tarihsel atamasıyla verilir; normal resource ataması geçmiş epoch'ları kapsamaz.
5. Yeni üyeye mevcut takımın geçmişini açmak takım kapsamı rolünün bilinçli sonucudur; olay anında o kişinin üye olması aranmaz. Bunun istenmediği durumda admin epoch/resource bazlı atama kullanır.
6. Unmanaged domain, managed takım domain'iyle birleşmez. Default/unmanaged'tan managed'a geçiş yeni epoch'tur.

Virtual çağrı consumer domain taşır. Onun upstream attempt'i aynı consumer domain ve provider domain taşır: payload için consumer alanı üzerinde events+ilgili yön izni ve capture scope gerekir. Upstream sahibi olmak tek başına consumer mesajını açmaz; provider tarafına ayrıca erişim şart koşulmaz, çünkü consumer araştırmacısı kendi çağrısının upstream request/response'unu inceleyebilmelidir. Bağımsız control-plane upstream testi target upstream domain'ini kullanır. Tenant-level system/Omni provider diagnostics, bir consumer'a güvenilir biçimde bağlanamıyorsa tenant sınıfıdır.

Omni diagnostics birden fazla consumer, yetkisiz aday veya tenant genelindeki keşif içeriğini karıştırabiliyorsa scoped role açılmaz. `audit.diagnostics.denied.read` ek şarttır, tenant/source kontrolünün yerine geçmez. Yeni kaynaklara göre içerik parçalama bu sürümün kapsamı değildir.

### 11.3 Legacy kayıtlar: backfill yok, açık tenant erişimi

Migration öncesi kayıtların tamamı ve yeni context taşımayan eski producer kayıtları legacy'dir. Event satırları, ciphertext, direction ve kayıt hash/envelope içerikleri yeniden yazılmaz. Ayrı context kaydının bulunmaması legacy anlamına gelir; her event'i toplu güncelleme zorunluluğu yoktur.

Otomatik slug/revision/timestamp/trace join'iyle sahiplik backfill yapılmayacak. Mevcut mimari bu verilerden kesin sahiplik kanıtı üretmiyor. Manuel UI ile “bu eski payload şu takıma ait” ataması da ilk sürümde bulunmayacak.

| Legacy erişim | Gereken izinler |
|---|---|
| Eski davranışla scoped metadata | Migration'ın açık legacy metadata/self ataması; yalnız metadata, mevcut görünürlük filtresi; geçmiş sahiplik garantisi verilmez |
| Tüm legacy metadata | Tenant `audit.events.read` + `audit.legacy.events.read` |
| Yönü bilinen legacy request | Tenant events+legacy.events + `audit.legacy.payloads.read` + request.read ve event capture-scope allowlist |
| Yönü bilinen legacy response | Aynısı + response.read ve capture-scope allowlist |
| Yönü veya capture scope'u boş/bilinmeyen legacy payload | Yukarıdaki tenant temel izinleri + `audit.legacy.payloads.unclassified.read`; request/response ayrımı garanti edilemediği UI'da belirtilir |

`audit.legacy.payloads.unclassified.read` tüm sınıflandırılmamış içeriği açan ayrı hassas izindir. Request-only veya response-only role otomatik eklenmez. Built-in Platform Admin ve mevcut admin token uyumluluğu bu hakları korur; viewer/editor/team/read tokenlara legacy payload hakkı eklenmez. Admin tenant kapsamlı custom araştırmacıya bunları açıkça atayabilir. Hiçbir project/team/resource ataması legacy payload açmaz.

Mevcut non-admin metadata görünürlüğü uyumluluk modunda korunur; yeni scoped audit rollerine bu istisna otomatik verilmez. Legacy metadata sorgusu bunun güncel slug/self temelli uyumluluk görünümü olduğunu bildirir; payload capability her zaman ayrıca hesaplanır.

### 11.4 Payload API kararı ve tutarlı hata sözleşmesi

Mevcut `GET /audit-events/{event}/payload` yolu korunacak. Sunucu ilk sorguda yalnız tenant, event metadata ve context yükler; ciphertext sorgulanmaz. Tenant sınırı, event görünürlüğü, legacy sınıfı, yön, capture scope ve ek diagnostics izni kontrol edildikten sonra ciphertext okunur/çözülür. Çok domain'li payload'ta gereken domain'lerin tamamı sağlanır. Parent/trace/conversation kimliği yetki genişletmez; child event kendi bağlamıyla kontrol edilir.

- Kimlik yok/geçersiz: 401.
- Event yok, tenant/kapsam dışında veya payload yönü izinsiz: aynı gövdeli 404. Liste capabilities izinli kullanıcıya panel açıklamasını sağlar.
- Yetkili event'te payload disabled/missing/expired: 200, içeriksiz açık state; veri varlığı yetkisiz kullanıcıya açıklanmaz.
- Yetkili event'te encryption/decrypt altyapısı kullanılamıyor: 503, içeriksiz genel hata.
- Yetkili eski event'te unknown direction yalnız unclassified izniyle döner; API yönü tahmin etmez.

Payload ve hassas metadata yanıtları no-store'dur. İzin kontrolü DB'de okuma anındaki authorization revision'a bağlıdır; uzun ömürlü payload URL veya browser tokenı üretilmez. Yetki iptali zaten tarayıcıya teslim edilmiş veriyi geri alamaz; UI bir sonraki capability yenilemesinde içeriği temizler.

### 11.5 Metadata alan ve filtre sözleşmesi

`audit.identities.read` yoksa `actorId`, `issuer`, `subject`, `clientIp`, `agentKey` JSON'dan çıkarılır; sabit veya kararlı hash ile kişileri ilişkilendirme imkânı verilmez. `actorType` kalabilir. Conversation özetlerindeki actor alanları da çıkarılır. Actor/subject/agentKey filtreleri aynı izin olmadan 403 döndürür; filtre sonucu üzerinden kimlik doğrulaması yapılamaz.

`reason` serbest metni varsayılan metadata'da gösterilmez; standart error/status code ve güvenli kısa açıklama gösterilir. Ayrıntılı reason için scoped `audit.details.read` eklenir; bu izin payload açmaz ve secret redaction'ı aşmaz. Action route şablonundan, target yetkili resource kimliği/slug'ından üretilir; raw URL/query/body kullanılmaz. Target'ın kapsam dışı bileşenleri çıkarılır. Conversation/correlation kimlikleri yetkili event üzerinde kalabilir; ilişki sorguları yeniden filtrelenir.

Metadata identity izni payload içini alan bazında maskelemez: request/response izni kayıtlı redakte edilmiş mesajın tamamını açar ve kimlik/kişisel veri içerebilir. Bu etki rol editöründe açıkça gösterilir. Alan bazlı payload görünümü bu sürümde vaat edilmez.

### 11.6 Yerel yetki iptali ve claim tazeliği

Control plane zaten browser session geçerliliğini PostgreSQL'den her istekte kontrol eder. İlk sürüm de authorization revision'ı her istekte DB'den okuyacak; yalnız aynı revision'a ait derlenmiş rol/mapping verisi process cache'inde tutulacaktır. DB/revision doğrulanamıyorsa 503 ile kapalı davranılır; eski olumlu cache kullanılmaz.

Yerel değişiklik commit edildikten sonra başlayan istekler yeni revision'ı görür; periyodik polling gecikmesi yoktur. Commit öncesi kabul edilmiş uçuş halindeki isteğin iptali garanti edilmez. Mutasyonlarda revision/kaynak kilidi transaction içinde yeniden doğrulanır. Bu kural runtime data-plane snapshot yayılımını anlık hale getirmez; mevcut snapshot freshness sınırları ayrıca geçerlidir.

Yeni OIDC session modelinde mapping için gereken typed claim alt kümesi server-side browser session kaydında saklanır; çerez kimlik/session referansı taşır. Raw ID/access token saklanmaz. Hassas claim alt kümesi şifrelenir ve loglanmaz. Yeni mapping başka claim gerektiriyorsa mevcut oturum o claim'i üretmez; yeni giriş gerekir.

Tüm yeni dinamik claim atamaları sign-in'den itibaren 15 dakika tazedir. Süre bitince claim tabanlı ek haklar durur; UI yeniden giriş ister. Sessiz refresh bu sürümde yoktur. Doğrudan issuer+subject ataması geçerli 8 saatlik session içinde çalışabilir. Legacy global rolün eski TTL davranışı yalnız açık uyumluluk atamasıyla korunur; yeni custom role taşınmaz. Eski session'lar yeni modele geçişte yeniden giriş yapar. Subject bazlı revoke provider kimliğiyle ayrıştırılır.

### 11.7 Audit settings ve GitOps sınırı

Mevcut tam `GET/PUT /audit-settings` tenant endpoint'i olarak kalır. Tenant yetkisi olmayan kullanıcıya tam settings belgesi verilmez. Yerel redaction için yeni `/virtual-servers/{slug}/audit-settings` ve `/upstreams/{slug}/audit-settings` GET/PUT yolları eklenir: yalnız ilgili yerel rules ve ortak settings revision'ı döner. Ortak revision 409 çatışma semantiğini korur. Global kurallar read-only effective preview özetiyle uygulanır; yerel endpoint bunları silemez. Preview editScope'u sunucu route bağlamından doğrular.

İlk sürümde `/export` ve `/apply` tenant işlemidir; project-scoped GitOps ertelenmiştir. `config.export/apply` yanında içerilen alan izinleri aranır; kapsam dışı kısmı sessizce atmak yerine tüm istek reddedilir. Team kullanıcıları resource API'lerini kullanır. `prune` tenant içinde bile ayrı silme izinleri gerektirir. Access roller/mapping'leri bu sürümde eski vmcp/v1 apply belgesine eklenmez; ayrı Access Management API'sinden yönetilir. Böylece eski doküman access yapılandırmasını değiştiremez. Migration'ın no-teams legacy GitOps uyumluluğu açık tenant compatibility atamasıyla sağlanır.

### 11.8 İlk sürüm OAuth sınırı

Control plane ilk sürümde mevcut tek OIDC provider'ın browser oturumları, mevcut named control-plane tokenları ve bootstrap tokenını kabul eder. Harici OAuth access tokenlarını doğrudan control-plane bearer kimliği olarak kabul etme bu sürüme alınmayacaktır. Runtime OAuth ve identity-binding davranışı değişmez.

Claim mapping OIDC'nin doğrulanmış ID-token claim'leriyle çalışır. Scope-token operatörü ancak aynı doğrulanmış claim kümesinde provider'ın güvenilir şekilde sunduğu `scope` benzeri string için kullanılabilir; OAuth access tokenından otomatik scope çekilmez. UI bunu “OIDC claim içindeki scope değeri” olarak açıklar. Machine erişimi custom rol atanan named control tokenlarıyla sağlanır. Sonraki harici OAuth desteği ayrı audience/issuer sözleşmesi ve migration gerektirir.

### 11.9 Kesinleşen yayın kapıları

- Scoped payload erişimi yeni producer context'i tüm event/batch/export yollarında atomik taşınmadan açılmaz; rolling upgrade sırasında eksik context legacy kalır.
- Transfer, silme, slug reuse, project owner transferi ve stale snapshot event'leri epoch testlerinden geçer.
- Legacy yön/scope bilinmeyen payload yalnız unclassified tenant izniyle açılır; backfill ile erişim genişletilmez.
- Identity/details maskesi listede, conversation özetinde, filtrelerde ve API yanıtlarında aynı uygulanır.
- Yetki değişikliği birden fazla control-plane replikasında commit sonrası yeni istekle doğrulanır; provider claim expiry ayrı test edilir.
- Audit settings ve GitOps için yukarıdaki ilk sürüm sınırları endpoint matrisinden daha geniş bir vaat olarak yorumlanmaz.

Bu kararlar uygulama planını netleştirir; uygulama kodunda değişiklik yapma yetkisi içermez.

## 12. İlk uygulama dilimi — veri modeli ve çekirdek

Bu dilimde eklenenler:

- `00036_control_plane_authorization.sql`: permission/role/assignment/mapping tabloları, authorization revision, sekiz built-in rol şablonu, audit context/domain/epoch altyapısı. Başlangıç modu `legacy`; kullanıcı veya token ataması oluşturulmaz. Önceki event'lere provenance backfill yapılmaz.
- `internal/authz`: izin kataloğu, typed claim koşulları, tenant/team/project/resource kararları, epoch ve yön bazlı audit payload kararı, legacy uyumluluk kapısı. Eksik/geçersiz policy veya eşleşmeyen atama erişim sağlamaz.
- `store.LoadAuthorization`: repeatable-read transaction ile tenant/revision/rol/mapping/assignment yükleme ve doğrulama. Veritabanı hatasında eski olumlu karara dönüş yoktur.
- Control plane `permit` mevcut semantiği ve hata mesajlarını koruyarak merkezi `LegacyDenial` fonksiyonuna yönlendirildi. Store ownership, handler istisnaları ve CSRF kontrolleri yerinde kaldı.
- Migration testi 35 → 36 upgrade, tekrar migrate, down/up, katalog eşitliği, revision/revoke, legacy event koruması ve provenance kısıtlarını doğrular. Çekirdek testleri scope laundering, issuer/tenant ayrımı, request/response, capture scope, epoch, legacy ve mapping davranışlarını kapsar.

Bu dilim dinamik yetkilendirmeyi canlı endpoint'lerde etkinleştirmez. Audit producer'larının yeni context yazması, snapshot'lara epoch eklenmesi, payload endpoint'inin yeni karara bağlanması, SQL liste filtreleri, claim tazeliği/session değişiklikleri, compatibility atamalarının import'u ve admin API/UI sonraki dilimlerin işidir. Context'i olmayan kayıtlar yeni çekirdeğe aktarılırken legacy kabul edilmelidir; yeni schema'nın varlığı tek başına güvenilir provenance anlamına gelmez. Shadow/enforced mode değeri bu dilimde bir rollout anahtarı olarak kullanılmaz; mevcut HTTP istekleri legacy akışında kalır.

## 13. İkinci uygulama dilimi — audit payload API bağlantısı

- Migration 37, `authorization_state.audit_payload_mode` alanını ekler. `legacy` varsayılandır; `shadow` yeni kararı eski yanıtı değiştirmeden karşılaştırır; `enforced` yalnız payload endpoint'inde yeni çekirdeği uygular. Global `mode` ve diğer endpoint'ler bu anahtarla etkinleşmez. Mod değişimi authorization revision'ı artırır.
- Payload route'u policy ve immutable event context'i ciphertext'ten önce yükler. Eksik context legacy'dir; live resource/slug join'i yapılmaz. Consumer/target domain'ler kontrol edilir; provider sahipliği consumer mesajlarına erişim vermez.
- Request/response/capture scope, resource epoch ve legacy/unclassified izinleri API üzerinde uygulanır. Platform admin/administrative tokenlar açık geçiş uyumluluğunu korur. Doğrudan subject/token atamaları desteklenir; OIDC claim mapping çözümü ve oturum tazeliği bağlantısı henüz eklenmemiştir.
- Yetkisiz payload ve olmayan event enforced modda aynı 404'ü verir. Query parametresi kayıtlı yönü değiştiremez. Tenant sınırı admin için de geçerlidir. Policy/context altyapısı hataları 503 ile kapalı davranır; no-store başarı ve hata yanıtlarına uygulanır.
- Payload okuma ve shadow karşılaştırmaları içeriksiz audit metadata'sı üretir. Görüntülenen mesaj reads capture açıkken dahi tekrar payload'a yazılmaz. Mevcut expiry cleanup, legal hold ve exporter retention davranışı korunur.
- Omni discovery diagnostics, event seviyesinde denied-sample hassasiyeti henüz kaydedilmediği için konservatif olarak ek tenant diagnostics izni gerektirir.
- OpenAPI ve Audit Log dokümantasyonu güncellendi. API testleri rollout, yön ayrımı, legacy kapsamı, revoke, transfer epoch'u, provider/consumer ayrımı, bozuk ciphertext öncesi ret, tenant ayrımı ve policy altyapısı arızasını kapsar.

Bu dilimde canlı veritabanında mod açılmadı; migration varsayılanı legacy'dir. Producer provenance/snapshot/epoch entegrasyonu tamamlanmadığından bugün üretilen bağlamsız kayıtlar legacy kalır. Testler verified context senaryolarını kontrollü fixture ile kurar. Kaynak bazlı payload rollout'u producer yayın kapıları tamamlanmadan üretimde açılmamalıdır. Admin rol/atama arayüzü ve metadata liste/capability filtreleri sonraki dilimlerdedir.


## 14. Üçüncü uygulama dilimi — snapshot kaynaklı audit provenance

Bu bölüm 12/13'te gelecek iş olarak belirtilen data-plane producer entegrasyonunun tamamlanan kısmını günceller.

- Migration 38 mevcut kaynaklara başlangıç epoch'u oluşturur; geçmiş event'leri değiştirmez. Deferred veritabanı trigger'ları final config revision'ında upstream owner, server project ve project owner değişimlerini atomik izler. Metadata güncellemesi epoch döndürmez; silme ve slug reuse eski kimliği devralmaz. Doğrudan SQL ve GitOps yazımları da kapsanır.
- Resource-level canlı atamalar transferde devre dışı bırakılır; epoch-pinned geçmiş atamalar korunur. Epoch-pinned atama canlı yönetim kontrolünde başka epoch'a taşınamaz.
- Snapshot read-only repeatable-read transaction'ında deployment tenant, immutable resource/project/team/epoch UUID'leri yüklenir. Aktif bir kaynak için epoch yoksa snapshot yüklemesi reddedilir. Dispatch'e yeni ownership sorgusu eklenmez.
- Virtual tool ve catalog operasyonları consumer context'i başlangıç snapshot'ından alır. MCP/REST adapter observation'ları bu context'i ve gerçek provider domain'ini taşır. Auth/approval ret kayıtları da snapshot kaynak bağlamını alır. Omni root, diagnostics ve model-provider kayıtları sealed ticket üzerinden tenant context'ini korur.
- Runtime, operation inheritance, async queue ve batch katmanlarında context kopyalanır ve başlangıç revision'ına sabitlenir. Metadata/context/domain/ciphertext aynı transaction'da yazılır. Batch iki pipelined bölümde önce yeni event'leri belirler; replay eski legacy event'e provenance veya payload ekleyemez. Queue byte limitleri provenance'ı da sayar.
- Testler strict/relax producer sınırları, çağrı sırasında proje transferi, eski/yeni takım erişimi, provider-only erişim reddi, upstream transferi, silme/slug reuse, rollback, epoch-preserving metadata edit, canlı/tarihsel assignment ayrımı, atomik hata ve idempotent replay'i kapsar. Payload API üzerinde gerçek snapshot context'iyle transfer testi eklenmiştir. Migration 38 down/up geçmiş context ve epoch'u korur.

Bu dilimde control-plane request üreticileri ve bağlamsız bakım kayıtları scoped provenance'a çevrilmedi; bunların payload'ları legacy tenant izinleri altında kalır. Metadata liste/capability filtreleri, custom rol UI/API ve OIDC session mapping bağlantısı sonraki dilimlerdir. Payload rollout varsayılanı legacy'dir ve üretim mod değişikliği yapılmadı. Migration 38 rollback'i servisler durdurularak yapılmalıdır; epoch kanıtı korunur ve re-upgrade canlı baseline'ı yeniden uzlaştırır.


## 15. Dördüncü uygulama dilimi — control-plane request üreticileri

Bu bölüm 14'te ertelenmiş olan idari request/outcome üretici bağlantısını tamamlar. Yeni migration gerektirmez; 36–38 veri modelini kullanır.

- Audited request pipeline admission öncesi deployment tenant ve config revision'ını pinler. Açık route allowlist'indeki mevcut upstream/virtual-server detay GET'leri, tools/catalog okumaları ve normal PUT/PATCH/DELETE kaynak UUID, project/team UUID ve epoch ile resource sınıfına alınır. Yeni veya bilinmeyen route varsayılan tenant'tır.
- Oluşturma, bulunamayan hedefe PUT, koleksiyonlar, credential/binding/governance, GitOps, import ve ağ test/refresh işlemleri tenant'tır. Top-level ownerTeam/project alanı taşıyan PUT/PATCH de aynı değer gönderilse bile tenant'tır. JSON decoder'ın kabul ettiği büyük/küçük harf varyantları ve null/malformed alanlar dar kapsamı aşamaz. Request body'den yetki veya kaynak kimliği türetilmez; yalnız kapsamın daraltılmasından vazgeçilebilir.
- Bu, önceki çift-domain transfer tasarımının güvenli ve konservatif ilk uygulamasıdır: transfer request/response'u için tenant payload hakkı gerekir. Aynı ownership alanını her kayıtta gönderen istemcilerin audit dokümanları da tenant kalır; alan-farkına göre daha dar görünüm sonraki optimizasyondur.
- Resource-scope pinleme repeatable-read transaction ile yapılır. Store.Do, mevcut revision lock'undan sonra epoch'u mutasyondan önce doğrular. Scope eskidiyse 412 ile değişiklik yapmadan reddeder. Request/outcome parent context'i değiştirilmeden mutation fact için clone alınır ve post-write revision ayarlanır.
- Kaynak response'u persist edilmeden epoch yeniden kontrol edilir. Handler sırasında transfer/deletion olduysa response tenant sınıfına yükseltilir; request ilk denemenin scope'unu korur. Bu control-plane okuma güvenliği kuralıdır; snapshot'tan çalışan data-plane operasyonlarının request/response epoch sabitlemesi değişmez.
- Ağ/protokol alt kayıtları tenant root context'ini miras alır. Kayıtların scope'u control/reads/upstream olarak ayrılır; request.read/response.read izinleri ve capture allowlist aynı merkezi çekirdekte uygulanır. Legacy permission şartı yalnız gerçekten context'siz eski kayıtlarda kalır.
- Settings, preview ve payload-read response'larının capture exclusion'ı korunur. Key/token'ın tamamı maskelenir; keyPrefix gibi mevcut kamusal metadata saklanabilir. Payload audit okumasının kendisi yeni bir payload kopyası üretmez.

Testler strict/relax modlarında upstream ve virtual-server PATCH ve GET payload'larını, request-only/response-only haklarını, farklı takım reddini, control/reads/virtual scope ayrımını, tenant sınıflı key create/connection test/create/transfer işlemlerini, idari upstream alt kayıtlarını ve secret redaction'ı doğrular. Deterministik admission trigger'larıyla araya transfer sokulup yeni sahip verisinin eski scope'a yazılmadığı ve stale mutasyonun rollback olduğu test edilir. Mevcut audit/migration/store/virtual/snapshot/export regresyonları ayrıca çalıştırılır.

Kaynak yönetiminin normal API rol/sahiplik/CSRF kontrolleri korunur. Payload rollout varsayılanı legacy'dir; üretim modu açılmaz. Metadata liste/capability filtreleri, custom rol/atama arayüzü ve OIDC session mapping bağlantısı sonraki dilimlerdir. Bağlamsız bakım/standalone metadata olayları için geriye dönük context üretimi yapılmaz.


## 16. Beşinci uygulama dilimi — metadata ve audit ekran yetkileri

Bu bölüm 15'te ertelenen metadata liste/capability entegrasyonunu tamamlar. Yeni migration gerektirmez. `audit_payload_mode` audit görünürlüğü rollout'u olarak hem liste hem payload için kullanılır; konfigürasyon endpoint'leri etkilenmez. Legacy/shadow metadata davranışı korunur; enforced modunda events.read zorunludur.

- Persist edilmiş context/domain üzerinden SQL policy predicate üretilir. Kaynak/takım/proje/epoch erişimi pagination, conversation gruplama ve sayımdan önce uygulanır. Resource domain'lerinin tamamı yetkili olmalıdır; provider sahipliği consumer erişimi üretmez. Legacy metadata için explicit tenant legacy.events.read gerekir. SQL kararları merkezi policy ile tenant/team/project/resource/unmanaged ve çok-domain fixture'larında karşılaştırılır; NULL project değerleri fail-closed değerlendirilir.
- Identity alanları identities.read, reason/target details.read ile event bazında korunur. Yetkisiz alanlar API yanıtından çıkarılır. Identity filtresi için izin yoksa 403; kısmi kapsam izninde yalnız yetkili identity domain'leri filtrelenir. Conversation actor alanı tüm sayılan kayıtlar yetkili değilse çıkarılır. Incomplete sorgusu yalnız görünür outcome'ları dikkate alır.
- API her event'e metadata/identity/details, generic payload, request ve response capabilities ile authorizationRevision ekler. Event context'leri batch sorguda yüklenir; ciphertext seçilmez. Her payload GET bağımsız yeniden yetkilendirilir. Listelerdeki yetenekler backend kontrolünün yerine geçmez.
- Audit listesi, filtreler, conversation ve detay ekranları sunucu yeteneklerini kullanır. manageTokens/admin rolü payload açma koşulu olmaktan çıkarılmıştır. Yetkisiz yönde buton/fetch yoktur; açılışta veya step seçerken prefetch yapılmaz. Payload capability yoksa admin UI rolü dahil fail-closed davranılır. Bilinmeyen yön lifecycle stage'den tahmin edilmez.
- Client metadata allowlist'i raw/copy görünümlerine bilinmeyen inline body alanı sokmaz. Sensitive identity/details alanları ikinci kez temizlenir. Modal kapanışı, step/capability/revision değişimi, session expiry ve liste refresh'i payload state'ini temizler; AbortSignal ve geç gelen sonuç kontrolü eski mesajı geri yükleyemez. 401/403 refresh eski metadata'yı temizler; genel ağ hatalarında önceki yetkili liste uyarıyla kalabilir. Browser/API no-store uygular.
- Canlı yetki push'u eklenmedi. Yerel revoke sonraki API isteğinde DB revision ile uygulanır; UI refresh/yeniden girişte güncellenir. Daha önce teslim edilmiş veri geri alınamaz.

Doğrulama: Backend testleri scope-before-pagination, group counts, metadata alanları, identity filter oracle, request/response capabilities, legacy izinleri ve gizli outcome completeness davranışını kapsar. UI testleri metadata-only/admin, request-only/viewer, response-only/viewer, capability eksikliği, revoke/pending response, modal kapanışı, session expiry ve revision değişimini doğrular. Playwright desktop/mobile testleri gerçek network request sayısını izleyerek yetkisiz payload endpoint'ine istek yapılmadığını, yalnız izinli yönün açık tıklamayla yüklendiğini ve refresh/revoke sonrası eski içeriğin kalmadığını kontrol eder.

Rollout varsayılanı legacy kaldı; üretim modu değiştirilmedi. Admin custom rol/assignment/mapping yönetim arayüzü ve OIDC session claim mapping bağlantısı sonraki dilimlerdir.

## 17. Altıncı uygulama dilimi — izin kataloğu ve rol yönetim API'leri

Yönetim katmanı küçük dilimler halinde uygulanır. Bu dilimde izin kataloğu ve rol CRUD tamamlanmıştır; atama/mapping CRUD ve simülasyon henüz tamamlanmış sayılmaz. Yeni migration gerekmez; 36'daki veri modeli kullanılır.

- `GET /api/v1/permissions`: uygulamanın sabit izin kataloğu; `id`, `scopes`, `captureRestricted`. Kataloğa API üzerinden yeni izin eklenemez.
- `GET/POST /api/v1/roles`, `GET/PUT/DELETE /api/v1/roles/{slug}`: hazır ve özel rolleri okuma, özel rol oluşturma/güncelleme/silme. Bu ilk sürümde bütün işlemler global platform admin gerektirir. Team üyeliği veya editor rolü yeterli değildir. Store yazma katmanı da doğrulanmış admin context'i olmadan erişimi reddeder.
- `copyFrom`, kaynak rolün slug'ını alır; o anki açık izin listesi kopyalanır. Hazır rolün sonraki sürümlerde genişlemesi özel rolü genişletmez. Hazır roller değiştirilemez/silinemez; custom rol `builtin` alanını belirleyemez. Slug değişmez.
- İzinler kayıtlı ve benzersiz olmalıdır. Açık boş liste kabul edilir; alanın atlanması kabul edilmez. Create sırasında `permissions` veya `copyFrom` seçilir. PUT tam izin listesi ister, `copyFrom` kabul etmez.
- GET/POST/PUT, role ait `ETag` döndürür. PUT/DELETE açık sayısal `If-Match` ister: eksik veya wildcard 428; eski sürüm 412. Konfigürasyon revision kilidinden sonra authorization revision kilidi alınır; iki eşzamanlı aynı-sürüm yazmadan yalnız biri başarılı olur. Authorization state trigger'ları policy revision'ını ilerletir.
- Disabled atamalar dahil role referans varsa silme 409 ile engellenir. Capture allowlist'i boş mevcut atamalar varsa role request/response payload izni ekleme 409 ile engellenir. Önce atamalara açık capture kapsamları verilmelidir. Böylece rol düzenlemesi belirsiz payload erişimi üretmez.
- Yönetim yanıtları `Cache-Control: no-store` taşır. Okuma ve değişiklikler audit metadata üretir; request/response içerikleri capture ayarları açık olsa da kaydedilmez. Role mutation fact'i yalnız `role:slug` hedefini içerir. Rol adı, açıklama ve izin listesi audit payload'ına kopyalanmaz. Sonraki claim simülasyonu da aynı exclusion altında kalacaktır.
- Global `mode` ve `audit_payload_mode` değiştirilmez. Oluşturulan rol tek başına kimseye erişim vermez; mevcut audit policy loader DB rol değişikliklerini yeni kararlarda okuyabilir. Konfigürasyon endpoint'leri eski rol/sahiplik kapılarında kalır.

Doğrulama: rol CRUD, hazır rol koruması, kopyanın izin eşitliği, bilinmeyen/tekrarlanan izinler ve unknown body alanları, read-token reddi, If-Match 428/412, kullanılan rol silme ve payload capture kapsam koruması PostgreSQL üzerinde test edilir. Ek testler eşzamanlı aynı-sürüm güncellemelerden yalnız birinin commit olmasını, mutation audit kaydını, capture açıkken yönetim payload'larının saklanmamasını, no-store ve rollout modlarının korunmasını doğrular. Mevcut controlplane/store/authz testleri race detector ile çalıştırılır. Bu testler yük testi değildir.

Sıradaki işler:

1. Atama CRUD: issuer+subject / kalıcı token UUID, kaynak seçiciler, owner/epoch pinleme, capture allowlist doğrulaması ve son yönetici koruması.
2. Claim/scope mapping CRUD: yapılandırılmış güvenilir issuer sınırı, typed koşullar, sürümlü değişiklikler; örnek claim'lerle salt simülasyon ve migration-status API'leri.
3. Admin rol/atama/mapping ekranları ve OIDC session mapping bağlantısı; ardından kalan konfigürasyon endpoint'lerinin izin matrisine geçirilmesi ve migration/rollout işletim adımları.
4. Fonksiyonlar tamamlandıktan sonra ayrı yük testiyle MCP çağrı gecikmesi, throughput ve DB/audit maliyetinin ölçülmesi; ölçüme göre iyileştirme. Bu aşamada yük testi veya performans optimizasyonu yapılmaz.

## 18. Yedinci uygulama dilimi — rol atamaları ve kapsam seçicileri

17'deki ilk madde tamamlandı. Bu dilim atama yönetimini canlı audit policy okumalarına bağlar; claim/scope mapping CRUD, simülasyon, migration-status ve admin ekranları sonraki dilimlerdir. Yeni migration gerekmez; 36–38 tabloları kullanılır.

### API ve kimlik modeli

- `GET/POST /api/v1/role-assignments`, `GET/PUT/DELETE /api/v1/role-assignments/{id}`. Bütün endpoint'ler global platform admin gerektirir. Store okuma/yazma metotları da admin context'i arar; iç worker bypass'ı yoktur.
- `roleId`, kalıcı rol UUID'sidir. Ya `principal: {kind: control_token, id: <token UUID>}` / `{kind: oidc, issuer: <issuer>, id: <subject>}` ya da `mappingId` verilir; iki yöntem birleştirilemez. Bootstrap token için yapay bir principal oluşturulmaz. Named token secret/prefix/slug kimlik yerine kabul edilmez.
- Enabled named-token ataması mevcut, süresi dolmamış ve revoke edilmemiş token ister. OIDC subject için dizin sorgusu yapılmaz; enabled atamada issuer, sunucunun yapılandırılmış OIDC provider issuer'ı ile tam eşleşmelidir. Mapping referansı mevcut olmalı; enabled atamada mapping issuer'ı aynı güven sınırına tabidir. Mapping'in kendisi disabled ise etkin bir eşleşme üretmez.
- `enabled` verilmezse false olur. Disabled OIDC taslakları yapılandırılmamış issuer ile saklanabilir; bu issuer'ı güvenilir yapmaz ve enable sırasında tekrar kontrol edilir. Revoke edilmiş ama mevcut token için disabled taslak tutulabilir. Kaynak/rol/mapping referansları taslaklarda da doğrulanır.
- PUT tam atama içeriğini değiştirir. GET/POST/PUT `ETag` döndürür; PUT/DELETE açık `If-Match` ister (eksik/wildcard 428, eski sürüm 412). Policy loader artık atama `version` değerini de okur.
- Liste `limit`, `cursor`, `enabled` destekler; enabled filtresi pagination'dan önce uygulanır. Yanıt `authorizationRevision` taşır. Snapshot tutarlıdır; sonraki sayfa yeni bir policy revision görebilir.

### Kapsam ve audit geçmişi

- Scope UUID'leri doğrulanır. Tenant/unmanaged scope ID veya pin taşımaz. Team yalnız mevcut takım UUID'si taşır. Project mevcut proje UUID'si taşır ve owner team DB'den sabitlenir. Kaynak, upstream/virtual_server türü ve kalıcı UUID ile seçilir.
- Herhangi bir `audit.*` izni içeren role ait yeni kaynak ataması, açık epoch verilmemişse mevcut epoch'a sabitlenir. Böylece sadece metadata izni de sahiplik transferi üzerinden yeni sahibin kayıtlarına taşınmaz. Salt konfigürasyon rolünün canlı kaynak ataması epoch'suz kalabilir; 38'in transfer trigger'ı bu atamayı disable eder ve sürümünü artırır.
- Aynı scope üzerinde PUT'ta epoch atlanırsa mevcut epoch korunur; proje owner alanı atlanırsa mevcut owner pin korunur. Basit bir düzenleme yeni sahipliğe sessizce taşınmaz. Açık başka epoch/owner vermek veya scope değiştirmek admin'in yeni bir yetki verme işlemidir ve DB kanıtına karşı doğrulanır. Unmanaged geçmişine boş owner ile yeniden yetki vermek gerekiyorsa yeni atama oluşturulur; aynı atamanın eksik owner alanı eski pinini korur.
- Açık resource epoch'u aynı deployment, kaynak türü ve kaynak UUID'sine ait olmalıdır; owner varsa epoch ile eşleşir. Kapalı epoch, kaynak silinse de atanabilir. Epoch'suz yeni atama canlı kaynak gerektirir. Yeni olayların context'i veya eski kayıtların sınıflandırması değiştirilmez; legacy kayıtlar resource epoch üzerinden otomatik erişilebilir olmaz.
- Projeye açık historical owner verilirse güncel owner ile veya kayıtlı project epoch kanıtıyla eşleşmelidir. Bu API proje geçmişini geriye dönük üretmez. Team/project varlığı kapsam seçimi sırasında doğrulanır; silinmiş kaynakların geçmişi için resource epoch ataması kullanılabilir.
- Role ait en az bir izin seçilen scope türünü desteklemelidir (boş rol de kabul edilir); scope desteklemeyen diğer izinler merkezi karar çekirdeğinde erişim üretmez. Payload izni olan rol için capture allowlist boş olamaz; kapsamlar kayıtlı ve benzersiz olmalıdır. Request ve response ayrı izinler olarak kalır.
- Sonradan role audit izni eklenirken mevcut epoch'suz resource ataması varsa rol güncellemesi 409 verir. Admin önce atama API'siyle açık epoch pinlemelidir; rol düzenleme tarihsel kapsamı kendiliğinden genişletmez.

### Seçiciler ve yönetici koruması

- `GET /api/v1/access/scopes?kind=team|project|upstream|virtual_server|control_token`: mevcut kapsamların UUID/slug/name bilgileri; kaynaklarda güncel epoch/project/owner, token'larda yalnız aktif kimlik UUID'si ve slug. Credential/secret içermez. `limit/cursor` destekler.
- `GET /api/v1/access/resource-epochs?kind=upstream|virtual_server&resourceId=<UUID>`: güncel ve kapanmış epoch UUID'leri, project/owner ve başlangıç/bitiş revision'ları; silinmiş kaynak UUID'siyle de kullanılabilir. `limit/cursor` destekler.
- Önceden en az bir enabled builtin `platform-admin` tenant ataması varsa atama API'leri bu sayıyı sıfıra indiremez (409). Mapping ataması sayılırken mapping'in enabled olması gerekir. Aynı transaction içinde config revision → authorization revision kilit sırası kullanılır; eşzamanlı son iki silmeden biri reddedilir ve rollback edilir.
- Bu koruma çalışabilen son login'i kanıtlamaz: IdP subject durumu, token'ın daha sonraki revoke/expiry işlemleri ve doğrudan SQL yazarları bu kontrolün dışındadır. Bootstrap ve eski global admin yolu korunur; bir atama oluşturmak yönetim API'sine yeni giriş yöntemi açmaz. Mapping CRUD ve rollout readiness bu sınırı ayrıca dikkate almalıdır.

### Güvenlik ve doğrulama

Tüm endpoint'ler no-store'dur. Yeni atamalar ve seçiciler 17'nin audit metadata/capture exclusion'ı altındadır; principal subject, mapping referansı ve kapsam içeriği request/response payload olarak kaydedilmez. Mutation hedefi `role-assignment:<UUID>` olarak tutulur. Aktif audit enforced modunda direct token atamasının oluşturulması ve disable edilmesi sonraki payload isteğinde uygulanır; yeni atama rollout modunu değiştirmez. OIDC claim çözümlemesinin gerçek session pipeline bağlantısı bu dilimde açılmaz.

Test kapsamı: CRUD ve sürüm kontrolleri; admin olmayan erişim reddi ve store context savunması; rol/token/mapping UUID ve referans doğrulaması; güvenilir issuer sınırı ve disabled taslaklar; capture scope doğrulaması; proje pininin transferde korunması; resource epoch pinleme, canlı atamanın transferde disable edilmesi, eski/yeni sahiplik kararları ve silinmiş kaynak geçmişine açık atama; audit iznini sonradan ekleme koruması; scope/epoch seçicileri ve filtreli pagination; son admin atamasını eşzamanlı silme; gerçek request-only audit payload erişimi ve atama disable sonrası erişimin kesilmesi; capture açıkken atama payload'larının saklanmaması. OpenAPI ve embedded spec güncellenir. Yük testi yapılmaz.

Bu dilimin doğrulama sonucu: controlplane, store, authz, audit, virtual ve config paketleri PostgreSQL ile `-race` altında geçti. Ayrı WireMock servisi isteyen `TestMockUpstreamThroughVirtualMCP` bu koşuda hariç tutuldu; ilk geniş koşuda yalnız eksik `VMCP_TEST_WIREMOCK_URL` nedeniyle çalışamadı. Yeni atama testleri dahil kalan testler geçti. OpenAPI doğrulaması 87 operation / mevcut 6 captured example için başarılı; `go vet` ve `git diff --check` temiz. Yeni migration veya rollout modu aktivasyonu yoktur.

## 19. Sekizinci uygulama dilimi — claim/scope mapping, simülasyon ve migration-status

18'de ertelenen mapping CRUD, simülasyon ve durum API'leri tamamlanmıştır. Yeni migration gerekmez; 36–38 modeli kullanılır. Bu dilim canlı OIDC session claim çözümlemesini veya konfigürasyon endpoint'lerinin granular enforcement'ını açmaz.

### Mapping API ve issuer güven sınırı

- `GET/POST /api/v1/identity-mappings`, `GET/PUT/DELETE /api/v1/identity-mappings/{id}`. Tüm endpoint'ler global platform admin gerektirir; store katmanı da açık admin context'i olmadan çalışmaz. Liste `limit/cursor/enabled` destekler ve policy revision'ını döndürür. Mapping modeline API JSON adları ve DB'den okunan `version` eklenmiştir.
- Input: `issuer`, `principalKind: oidc`, `conditions`, `enabled` (varsayılan false). Koşullar 1–32 adet, benzersiz ve AND'dir. `path` en fazla 512 byte; `value` string, en fazla 2048 byte; boş/whitespace-only değer kabul edilmez. `equals` string eşitliği, `array_contains` string-array elemanı, `scope_contains` string claim içindeki tek, tam whitespace-delimited scope token'ıdır. Regex/substring/wildcard yoktur. Tam claim anahtarı dotted-path çözümünden önce gelir; tür dönüşümü yapılmaz.
- Enabled mapping yalnız sunucunun mevcut OIDC provider `Issuer()` değerine tam eşit issuer ile saklanır. Sondaki slash, başka host/path veya farklı issuer eşit sayılmaz. API, body içindeki issuer/audience alanlarından yeni trust root kurmaz; discovery yaparak otomatik provider eklemez. Scope, doğrulanmış ID-token claim kümesinde bulunabilecek bir string'in eşleme operatörüdür; harici OAuth bearer kabulü değildir.
- Disabled mapping taslağı yapılandırılmamış issuer ile tutulabilir; enable sırasında tekrar doğrulanır. Enabled assignment referansı varsa mapping disabled olsa bile issuer'ı yapılandırılmış provider dışına değiştirmek 409'dur. Önce assignment'lar kapatılmalıdır. Provider değişikliği sonrası eski issuer'lı enabled kayıtlar migration-status'ta blocker olarak görünür; bunların simulation'ı yeni issuer adına çalıştırılamaz.
- GET/POST/PUT ETag taşır. PUT/DELETE açık sayısal `If-Match` ister; eksik/wildcard 428, stale 412. Disabled assignment'lar dahil herhangi bir referans varsa DELETE 409'dur.
- Mapping disable işlemi son enabled builtin `platform-admin` tenant atamasını ortadan kaldırıyorsa 409 ile rollback edilir. Son admin mapping'inin AND koşullarını değiştirmek de başka enabled admin ataması gerektirir; koşul değişikliği login kapsamını sıfıra indirebilir. Kontrol config revision → authorization revision kilitleri altında çalışır; eşzamanlı son iki mapping'i disable etme senaryosu korunur. Bu konservatif kontrol hâlâ IdP'de gerçekten giriş yapabilen kullanıcı bulunduğunu kanıtlamaz.

### Salt simülasyon

`POST /api/v1/access/evaluate` input'u `principal`, opsiyonel `claims`, `permission` ve tam olarak bir `resource` veya `auditEventId` içerir. Raw token veya istemci kaynaklı tenant/owner/project/epoch audit bağlamı kabul edilmez.

- Yalnız admin çalıştırabilir. OIDC örneğinde principal issuer mevcut provider ile tam eşleşir. `iss/sub` örnek claim'leri verilmişse seçilen principal ile aynı string olmalıdır. Named-token örneği mevcut aktif token UUID'sini kullanır; bu principal türüne claim eklenemez. Örnek veri imza, audience, authentication veya claim tazeliği doğrulaması değildir.
- `resource` yalnız `kind/id` seçer. Tenant ID'si deployment'tan, canlı kaynak owner/project/epoch bilgileri DB'den alınır. Team/project için ilgili mevcut kapsam okunur. Türler tenant/team/project/upstream/virtual_server'dır. Unmanaged, kaynak türü değil assignment scope'udur; unmanaged kaynak, owner'ı olmayan gerçek upstream/virtual_server üzerinden değerlendirilir.
- Audit metadata, identity, details ve request/response payload görünürlüğü için mevcut `auditEventId` zorunludur. Persist edilmiş immutable provenance okunur; ciphertext seçilmez veya decrypt edilmez. Merkezi `CanReadAuditPermission/CanReadPayload` kararları kullanılır; consumer/target domain'lerinin tamamı ve legacy izinleri aynen geçerlidir. Request izni response olayını açmaz; bilinmeyen direction request/response diye tahmin edilmez. Genel legacy-unclassified payload hakkı ayrı kaldığından bu API bilinmeyen yönü directional izin için başarılı saymaz.
- Policy, rollout modları ve olay/kaynak metadata'sı aynı repeatable-read snapshot'ta okunur. `matchedMappingIds` AND koşulları eşleşen mapping'leri; `candidateAssignmentIds` principal'a bağlanan enabled adayları gösterir. Aday olmak scope veya permission izni değildir. `decision` yalnız seçilen izin/kapsam kararını verir.
- Yanıt daima `simulationOnly: true`, `authenticationValidated: false`, `legacyCompatibilityIncluded: false`, `configurationEnforcement: legacy`, `claimMappings: simulation-only` taşır. Konfigürasyon için policy izin kararı mevcut coarse-role/ownership endpoint kontrollerinin yerine geçmez. Eski admin uyumluluk atamaları simülasyona sessizce eklenmez. Mode/auditMode ve authorizationRevision sonuç bağlamını gösterir.
- Oturum, cookie veya token üretilmez; kullanıcı/claim/assignment kalıcılaştırılmaz. Örnek claims veya subject yanıtla geri gönderilmez. Simülasyonun kendisi normal audit metadata üretir, ancak request/response payload capture exclusion'ı örnek hassas verileri saklamaz.

### Migration-status

`GET /api/v1/access/migration-status`, admin için salt durum/teşhis endpoint'idir. Aynı snapshot'tan authorization revision, uygulanmış migration version, global/audit modları ve aşağıdaki aggregate sayaçları döndürür:

- Toplam/custom rol, toplam/enabled assignment, toplam/enabled mapping.
- Mevcut provider dışında kalan enabled mapping ve enabled OIDC/mapping assignment sayıları.
- Artık aktif named token'a bağlanmayan enabled assignment'lar ve epoch pin'i eksik audit resource assignment'ları.
- Legacy (context'siz veya explicit legacy), resource ve tenant sınıfındaki audit kayıtları. Context'siz kayıtlar slug/tarih/revision ile kaynaklara atanmaz; counter sorgusu backfill değildir.

Yanıt `externalOAuthBearer: false`, `legacyAdminCompatibility: true`, `configurationEnforcement: legacy`, `claimMappings: simulation-only`, `configurationEnforcementReady: false` ile henüz bağlanmamış iki aşamayı açık blocker olarak gösterir. Riskli kayıt sayaçları sıfırdan büyükse ayrıca blocker olur. Bu bir otomatik rollout veya “migration güvenlidir” sertifikası değildir; aktif session, CI/GitOps ve gerçek IdP erişilebilirliği envanterinin tamamının yerine geçmez. Mevcut bootstrap/admin kurtarma yolu korunur. Status isteğinin kendi audit metadata'sı yeni kayıt oluşturabilir; konfigürasyon/authorization modu, revision veya eski olay bağlamını değiştirmez.

### Doğrulama ve sonraki işler

API testleri gerçek yapılandırılmış test OIDC provider'ı üzerinden trusted/untrusted issuer ayrımını, provider dışındaki taslakların enable reddini, typed AND/equals/array/scope koşullarını ve tam scope sınırını doğrular. Referanslı silme, stale/missing If-Match, son admin mapping koşulu/disable koruması ve eşzamanlı disable senaryosu kapsanır. Simülasyon testleri spoof edilmiş tenant/owner/epoch girişini, raw token alanını, yanlış issuer/iss/sub ve hatalı principal/claim türlerini reddeder; imzalı OIDC access tokenının mevcut control API'ye bearer olarak kabul edilmediğini doğrular. Legacy payload izin şartları, gerçek request-only audit erişiminin korunması ve migration-status'un mevcut mod/revision/context'i değiştirmemesi test edilir. Capture açıkken hassas sample claim/subject/koşul değerleri audit payload/metadata veya simülasyon yanıtına sızmaz. Store admin-context savunması yeni metotları da kapsar.

OpenAPI ve embedded spec bu endpoint ve şemaları içerir. Canlı OIDC claim-alt-kümesi saklama/freshness/session entegrasyonu, admin yönetim ekranları, kalan konfigürasyon izin matrisi ve rollout işletim araçları sonraki işlerdir. Yük testi ve performans iyileştirmeleri fonksiyonlar tamamlandıktan sonra ayrı ele alınır.

Bu dilimin son doğrulaması: controlplane, store, authz, audit, virtual, config ve oidc paketleri PostgreSQL ile `-race` altında geçti. Ayrı WireMock servisi gerektiren mevcut `TestMockUpstreamThroughVirtualMCP` hariç tutuldu. `go vet`, `git diff --check` ve OpenAPI doğrulaması (94 operation, mevcut 6 captured example) başarılı. Yük testi, yeni migration, canlı claim-session aktivasyonu veya rollout modu değişikliği yapılmadı.

## 20. Dokuzuncu uygulama dilimi — canlı OIDC claim-session bağlantısı

Canlı OIDC session mapping, audit liste/payload enforcement hattına bağlandı. Konfigürasyon endpoint'lerinin granular enforcement'ı ve admin rol/atama/mapping ekranları sonraki dilimlerdir. Global/audit rollout modları otomatik değişmez; harici OAuth bearer kabulü açılmaz.

### Sunucu tarafında doğrulanmış session

- Migration **39** `browser_sessions` tablosuna `issuer`, `model_version` ve `session_ciphertext` ekler. Eski satırlar version 0/issuer boş kalır; issuer veya claim kanıtı uydurulmaz. Production factory, mevcut keyring/deployment namespace ile encrypted `PostgresSessions` kullanır.
- ID token'ın imza, issuer, audience ve nonce doğrulaması tamamlandıktan sonra mevcut enabled, güvenilir issuer'lı mapping'lerin gerektirdiği claim path'leri alınır. Sadece string/string-array alt kümesi kaydedilir. Tam anahtar dotted-path'ten önce gelir; kayıt full path anahtarlarıyla düzleştirilir. Bir object path seçerek sibling alanlar toplanamaz. OAuth exchange access/ID tokenları kayıt modelinin parçası değildir.
- En fazla 256 path, path başına 512 byte, string başına 8192 byte, array başına 256 eleman ve toplam 64 KiB claim alt kümesi sınırı vardır. Desteklenmeyen tip veya fazla büyük değer scope/string eşlemesine dönüştürülmez; alan yoksa pozitif AND koşulları izin üretemez. Toplam/path sınırı aşılırsa giriş fail-closed başarısız olur. Mapping API'nin daha dar koşul/value limitleri korunur.
- Session'ın ad, eski rol, grup ve typed claim bilgileri keyring ile şifrelenir. Versioned authenticated envelope session ID'sine ve purpose'a bağlıdır; DB issuer/subject/expiry metadata'sı decrypt edilen kayıtla doğrulanır. Yapılandırma ciphertext yazımı legacy olsa bile yeni session kayıtları `BoundOnly` reader/writer ile v1 envelope kullanır; diğer ciphertext davranışı değiştirilmez. DB'nin revoke araması için issuer/subject index'i metadata olarak kalır.
- Yeni **`vmcp_session_v2`** cookie'si yalnız imzalı `sid/v/exp` referansı taşır. Claim, grup, ad, rol veya subject cookie'ye yazılmaz. Yeni login eski cookie'yi temizler. Farklı cookie adı ve v2'ye özel HMAC imza domain'i, referansın cookie adı değiştirilerek eski node üzerinde client-owned identity gibi yorumlanmasını da önler. Production reader eski `vmcp_session` cookie'sini kabul etmez; yeniden giriş gerekir.
- Her browser isteğinde authoritative session DB'den yüklenir. Issuer değişimi, revoke, expiry, DB/decrypt hatası, kayıt kimlik uyuşmazlığı veya başka session ID'sine taşınmış ciphertext authentication'ı reddeder. Browser referansı veya body, issuer/subject/rol/claim kaynağı olamaz.

### Tazelik ve canlı policy

- Captured claim'ler sign-in'den itibaren en fazla **15 dakika**, ayrıca kaynak ID token expiry'sine kadar tazedir. Claim alt kümesi boşsa mapping tazeliği hakkı veya gereksiz UI deadline üretilmez. Süre sonunda claim map karar girdisinden çıkarılır; sessiz claim refresh veya token saklama yoktur.
- Doğrudan issuer+subject ataması geçerli session TTL içinde çalışmaya devam eder. Eski viewer/editor/admin rolü ve eski team group TTL davranışı uyumluluk için korunur; admin'in açık geçiş hakları claim mapping'ten türetilmez. Legacy global rol değişikliği eskisi gibi yeniden girişte alınır. Yeni custom mapping hakkı eski rolün 8 saatlik TTL'ine taşınmaz.
- Audit policy her istekte DB revision'ıyla yüklenir ve yalnız fresh verified session alt kümesiyle `ResolveMappings` uygulanır. SQL metadata filtreleri, event capability'leri ve payload request/response kararları aynı resolved policy'yi kullanır. Mapping veya assignment disable/role değişikliği sonraki istekte etkili olur. Önceden kabul edilmiş uçuş halindeki isteği iptal etme garantisi eklenmez.
- Yeni mapping başka bir claim path'i gerektiriyorsa eski session'da o değer yoktur. Provider'dan arka planda claim toplanmaz veya örnek simülasyon verisi session'a taşınmaz. Yeni giriş mevcut path alt kümesini yeniden yakalar. Aynı zaten-yakalanmış claim'e dayanan yeni koşullar ise mevcut taze oturumda değerlendirilebilir.
- Legacy/shadow audit modları canlı mapping hakkıyla genişletilmez. Yalnız açık `audit_payload_mode=enforced` yeni audit yetkilerini uygular. Runtime MCP/OAuth identity-binding ve named-token authentication değişmez.

### Custom-only giriş ve yetki sınırı

Legacy global/team rolü olmayan bir doğrulanmış kullanıcı, yalnız enforced audit yüzeyinde enabled `audit.events.read` atamasıyla giriş yapabilir. Doğrudan subject veya fresh mapping ataması kabul edilir. Bu oturum `custom` olarak işaretlenir; viewer/editor/admin gibi coarse rol verilmez.

`custom` session yalnız `/me` ve açık enforced audit liste/payload yüzeyine erişebilir. Eski generic GET kuralı, team-member editor promotion'ı ve konfigürasyon/management rotaları bu oturumu genişletemez. Mapping'de builtin platform-admin seçmek session'ı eski global admin yapmaz. Yönetim API'leri hâlâ legacy global admin ister. Bu, konfigürasyon permission matrisi bağlanıncaya kadar konservatif sınırdır.

UI custom kullanıcıyı audit ekranına yönlendirir ve konfigürasyon menülerini/yazma yeteneklerini sunmaz. Backend aynı sınırı bağımsız uygular. `/me` ham claim içermez; yalnız opsiyonel `claimsFresh/claimsExpire` döndürür. Deadline dolduğunda UI yeniden giriş seçeneği gösterir, audit metadata/payload görünümünü temizler, bekleyen payload isteğini abort eder ve geç yanıtı göstermez. Oturumun diğer geçerli hakları iptal edilmiş gibi davranılmaz; kullanıcı doğrudan subject/legacy haklarıyla yeniden sorgu yapabilir. Browser timer güvenlik kararının yerine geçmez.

### Issuer'a göre revocation ve durum

`POST /api/v1/revocations/sessions` artık subject'i issuer ile birlikte kapsar. `issuer` verilmezse yapılandırılmış provider, provider yoksa legacy issuer boş değeri seçilir. Açık `issuer: ""` yalnız legacy satırları seçer; başka tarihi issuer açıkça verilerek yalnız o provider'ın session'ları revoke edilebilir. Bu alan trust root eklemez, sadece var olan erişimi kaldırır. Aynı subject'e sahip başka issuer etkilenmez.

Production encrypted claim callback'leri bağlıysa migration-status ve simülasyon yanıttaki runtime göstergesi `claimMappings: live-audit` olur; simülasyon yine authentication yapmaz ve session üretmez. Güvenli session bağlantısı olmayan provider'da `simulation-only` kalır. Global `configurationEnforcement: legacy` ve `configurationEnforcementReady: false` korunur. Status ayrıca aktif legacy/server browser session ve expired browser session aggregate sayaçlarını verir; claim ciphertext'lerini sayaç için decrypt etmez. Yeni model bağlıysa `oidc_claim_sessions_not_connected` blocker'ı çıkarılır; konfigürasyon enforcement blocker'ı kalır.

### Kurulum geçişi ve rollback

1. DB yedeği/keyring erişimi doğrulanır; additive migration 39 uygulanır. Eski session satırlarına issuer/claim backfill yapılmaz.
2. Control-plane node'ları aynı yeni session reader sürümüne geçirilir; geçiş sırasında eski/yeni cookie adlarının farklı node'larda kabul edilmemesi nedeniyle yeniden giriş gereksinimi beklenir. Fleet tamamlanmadan oturum sürekliliği varsayılmaz. Data-plane veya runtime OAuth rollout modu değiştirilmez.
3. SSO kullanıcıları yeniden giriş yapar. `/me` raw claim olmadan kimliği; migration-status `live-audit` kabiliyetini ve kalan legacy kayıt sayılarını gösterir. Audit modu mevcut değerinde kalır; enforced aktivasyonu ayrı planlı rollout adımıdır.
4. İlgili mapping/assignment/capture/legacy izinleri simülasyon ve gerçek audit testleriyle doğrulanır. Eksik yeni claim path'i için yeniden giriş yapılır. Simülasyonun örnek claim sonucu gerçek login/session kanıtı değildir.
5. Rollback gerekirse control-plane yazmaları durdurulur; migration 39 down yeni version-1 session satırlarını siler, eski version-0 satırlarını korur ve alanları kaldırır. Yeni referans cookie eski modele dönüştürülmez; yeniden giriş gerekir. Audit provenance/epoch/ciphertext tabloları değiştirilmez.

15 dakika sınırı authorization freshness içindir; encrypted session satırının fiziksel retention süresi değildir. Süresi geçmiş claim verisi yetki girdisi olmaz, ancak şifreli session kaydı fiziksel cleanup'a kadar DB'de kalabilir. Operasyonel session retention/cleanup politikası buna göre uygulanmalıdır.

### Doğrulama

Gerçek test IdP login/callback akışı signature/issuer/audience/nonce doğrulamasından önce claim capture/session yazımı yapılmadığını, typed alt kümenin şifrelenmesini ve reference-only cookie'yi doğrular. Diğer testler 15 dakika boundary, legacy rolün korunması, cross-node logout/revocation, issuer sınırı, ciphertext ID/subject manipülasyonu, legacy cookie reddi, v2 referansının eski cookie adına taşınarak tekrar kullanılmasının reddi ve yeni claim path'i için yeniden giriş gereksinimini kapsar. Migration down/up testinde yalnız yeni session'ların silindiği ve eski satırların issuer kanıtı kazanmadığı doğrulanır.

API testleri legacy/shadow/enforced audit davranışını, request-only/response reddini, mapping disable sonrası aynı session'da yeni kararı, stale claim reddini ve stale claim'e rağmen direct subject erişimini doğrular. Custom session GET/POST/management/CSRF sınırları ve explicit/default issuer revocation kapsanır. UI testleri custom yetenekleri/navigation, deadline bildirimi, açık/pending/late payload temizliğini; desktop/mobile browser testleri custom kullanıcı için konfigürasyon isteği gönderilmediğini ve payload'ın claim expiry olayında kapandığını doğrular. Yük testi yapılmaz.

Sıradaki dilim: admin rol/atama/mapping/simülasyon ekranları; ardından kalan konfigürasyon endpoint'lerinin granular permission matrisi, rollout readiness ve işletim araçları. Performans ölçümü fonksiyonlar tamamlandıktan sonra ayrı yapılacaktır.

Bu dilimin son doğrulaması: 8 backend paketi (controlplane/store/authz/audit/virtual/config/oidc/secrets) PostgreSQL ile `-race` altında geçti; control-plane binary'si derlendi. Ayrı WireMock servisi isteyen mevcut `TestMockUpstreamThroughVirtualMCP` geniş koşuda hariç tutuldu. Cookie imza domain ayrımı sonrası oidc/controlplane/store/secrets regresyonları tekrar geçti. 278 UI unit testi ve 11 desktop/mobile browser testi geçti; frontend build, OpenAPI (94 operation / mevcut 6 captured example), `go vet` ve `git diff --check` başarılı. Migration 39 yalnız izole test DB'lerinde uygulandı; gerçek kurulum rollout modu değiştirilmedi ve yük testi yapılmadı.

## 21. Onuncu uygulama dilimi — admin yetki yönetimi arayüzü

Rol, atama ve claim/scope mapping yönetimi ile simülasyon ve migration durumu `/access` ekranına bağlandı. Yeni backend endpoint'i veya migration gerekmez; 17–20'de tamamlanan API/session sözleşmeleri kullanılır. Yönetim yetkisi ilk sürümde hâlâ eski global **admin** rolüdür; custom role içine `access.*` koymak bu sınırı delege etmez.

### Erişim sınırı ve veri ömrü

- Navigasyondaki **Access management** bağlantısı yalnız global admin için görünür. Viewer/editor/team kullanıcısının doğrudan `/access` URL'si bile yönetim component'ini mount etmez; role/permission/assignment/mapping/status/selector verisi indirilmez. Custom session mevcut audit yönlendirmesinde kalır. Görünürlük kontrolü güvenlik sınırının yerine geçmez; backend bağımsız admin/CSRF/issuer kontrollerini sürdürür.
- Yönetim API yanıtları frontend'de de `cache: no-store` ile alınır. Liste request'leri ve yazmalar AbortSignal taşır. 401/403, role değişimi veya session-expired olayı bütün çalışma alanını unmount eder; açık form, claim koşulları, örnek JSON ve değerlendirme sonuçları UI'den çıkarılır. Geç dönen yanıtlardan eski veri tekrar gösterilmez.
- Kaynak/token/epoch selector'ları yalnız ilgili editör ve kapsam için yüklenir. Bunların 403 yanıtı da sadece küçük alanı değil tüm yönetim çalışma alanını kapatır. Yetki kaybından önce açılmış ama daha sonra onaylanan delete confirmation yeni bir mutasyon başlatamaz. Önceden sunucuda kabul edilmiş bir isteğin commit'ini geri alma garantisi eklenmez.
- Örnek claims ve yönetim draft'ları yalnız component memory'sinde tutulur; localStorage/sessionStorage veya URL'ye yazılmaz. Simülasyon panelinden ayrılmak, access refresh veya oturum/yetki kaybı örnekleri ve sonuçları bırakır. Scope/payload/audit kayıtları yönetim UI'sine inline indirilmez.

### Roller ve izin kataloğu

- **Roles** bölümü builtin/custom rol listesini, açıklamayı, sürümü ve izin sayısını gösterir. İsim/slug/izin kimliğine göre arama ve uzun listelerde daha fazla gösterme bulunur.
- Builtin rol yalnız okunur; edit/save/delete yolu sunulmaz. Permission araması readonly görünümde de çalışır. **Copy** açık izin listesini yeni custom draft'a taşır; yeni rol source role'ün gelecekteki izin genişlemelerini miras almaz. UI create/update body'lerine id/builtin/version gibi readonly alanlar eklemez.
- Permission kataloğu namespace'lere ayrılmış checkbox listesiyle gösterilir; her iznin desteklediği scope'lar ve capture allowlist gereksinimi görünür. Request ve response ayrı seçimlerdir. Boş izin listesi izin üretmeyen rol olarak açıkça gösterilir; wildcard veya otomatik yeni izin ekleme yoktur.
- Rolün atamaları varsa (disabled olanlar dahil) delete kapalıdır. Eşzamanlı yeni referans oluşursa backend 409 yine otoritedir. Sonradan payload/audit izni eklemenin capture/epoch gereksinimleri backend'den gelen hata ile draft korunarak gösterilir.

### Atamalar ve sahiplik kapsamı

- **Assignments** rol, principal veya mapping kaynağı, scope, recorded owner/epoch, capture allowlist, enabled ve sürümü gösterir. Yeni atama varsayılan disabled'dır.
- Direct OIDC principal, issuer+subject ister. Machine principal mevcut aktif named token UUID'siyle seçilir; token secret/prefix asla kimlik yerine kullanılmaz veya UI tarafından istenmez. Mapping kaynağı seçilirse direct principal body'den çıkarılır; direct principal seçilirse mappingId çıkarılır. Control-token body issuer taşımaz.
- Enabled OIDC ataması için issuer UI'de de configured issuer ile tam karşılaştırılır. Kaynak olarak disabled mapping seçilebilir ama kendi mapping'i enabled olmadıkça erişim üretmeyeceği listede görünür. Backend reference/trust doğrulaması her yazmada devam eder.
- Tenant/unmanaged/team/project/upstream/virtual_server scope seçimi ve kalıcı kaynak UUID selector'ları bulunur. Kaynak UUID elle girilerek silinmiş kaynakların epoch'ları da seçilebilir. Ownership period listesi current/historical, revision aralığı ve epoch UUID'si gösterir; server'a yalnız açık seçilmiş pin gönderilir.
- Aynı kaynağın mevcut epoch/owner pin'i sıradan editte korunur. Yeni current/historical epoch'u seçmek açık bir yeniden yetkilendirme işlemidir; seçilen epoch'un owner bilgisi birlikte taşınır ve backend DB kanıtına göre doğrular. Project owner için opsiyonel UUID alanı mevcut pin/otomatik seçim semantiğini açıklar. UI, eski olayları yeni sahibin kapsamına yeniden yazmaz.
- Seçili role ait hangi izinlerin seçilen scope'ta aktif olmadığı görünür; hiçbir izin uygulanmıyorsa client yazmayı engeller. Scope'lar arası destek kontrolünün son otoritesi backend'dir. Capture listesi kayıtlı dokuz scope'tan açık seçim ister; bu liste tek başına request/response hakkı vermez. Payload yetkili rol boş capture listesiyle kaydedilemez.

### Claim/scope mapping

- **Claim mappings** issuer, enabled durumu, typed AND koşulları ve sürümü gösterir. Claim path/operator/value editörü string equals / array contains string / exact scope token operatörlerini sunar; yeni provider veya audience trust ayarı sunmaz.
- Configured issuer read-only kaynak bilgisi olarak görünür. Başka issuer disabled draft olarak saklanabilir; enable edilmişse client bunu server'a göndermeden reddeder. Koşul sayısı en fazla 32; scope_contains değerinde birden fazla token kabul edilmez. JSON örnekleri koşul veya session verisine otomatik aktarılmaz.
- Yeni claim path'lerinin mevcut SSO session'ına arka planda eklenmediği, yeni sign-in gerektirdiği açıklanır. Referanslı mapping silinemez; backend'in son admin/AND koşulu ve issuer değiştirme 409 korumaları UI'de draft kaybetmeden gösterilir.

### Sürüm ve hata davranışı

- Rol/mapping/atama PUT ve DELETE, yüklenen kaydın `version` değerini açık `If-Match` olarak taşır. Yazma otomatik yeniden denenmez. 412'de draft korunur, Save kapanır ve explicit reload/discard seçeneği gerekir. Reload güncel kaydı ve yeni version'ı birlikte yükler; eski draft'ın sürümü sessizce yükseltilmez.
- 409/422 açıklamaları ve field hata mesajları görünür kalır. Delete confirmation seçilmiş kaydı ve sürümü hedefler. Delete için 412 alınırsa çalışma alanı Refresh yapılıncaya kadar yeni yazma açılmaz.
- Paginated assignment/mapping sayfaları cursor döngüsüne ve revision değişimine karşı doğrulanır. Rol/atama/mapping/status catalog revision'ları aynı değilse UI yazma için eksik/tutarsız birleştirilmiş görünüm sunmaz; Refresh ister. Selector query'lerinin kind/resourceId filtreleri cursor sayfalarında korunur. Authorization API'leri fail-closed olduğu için UI hata durumunda eski olumlu karara dönmez.

### Simülasyon ve migration durumu

- **Simulation** direct OIDC veya token principal, registered permission ve canlı kaynak ya da persisted audit event UUID seçer. OIDC için typed sample JSON object alınır; raw JWT/ID/access token istenmez. Invalid JSON/object ve untrusted issuer client'ta reddedilir. Audit event yolu request/response mesajını hiçbir zaman `auditPayload` endpoint'iyle yüklemez.
- Sonuç allowed/denied ve reason, policy revision, matched mapping/candidate assignment UUID'lerini gösterir. `simulationOnly`, authentication doğrulanmadığı, legacy compatibility'nin dahil olmadığı, config/audit mode ve runtime claim-session kabiliyeti açıkça belirtilir. Aday assignment scope/permission grant'i olarak sunulmaz. Input değişiminde veya refresh'te sonuç düşürülür, pending değerlendirme abort edilir; geç yanıt eski sonucu geri getirmez. Simülasyondan role atama, session veya token üreten yol yoktur.
- **Migration status** schema/policy revision, configured issuer, global/audit modları, config enforcement, claim sessions, external bearer, legacy admin compatibility ve readiness'i readonly gösterir. Blocker ve aggregate sayaçlar açıklanır. Aktivasyon, apply, backfill veya eski audit payload sahipliği atama düğmesi bulunmaz.
- UI hem ana başlıkta hem ilgili editörlerde konfigürasyon izinlerinin henüz legacy API kapılarının yerini almadığını açıklar. Bu ekranların kullanımı audit/global rollout modunu değiştirmez. Gerçek audit görünürlüğü mevcut explicit audit enforced moduna ve ilgili role/assignment haklarına tabidir.

### Doğrulama ve kalan işler

Unit testler builtin koruması/copy, explicit request-only izinler, capture allowlist, OIDC/token/mapping body ayrımı, scope desteği ve epoch/owner pin koruması, intentional repin, trusted issuer, typed AND koşulları, 409/412 davranışı, revision tutarlılığı, readonly migration ve payload'sız simülasyonu kapsar. Auth kaybı sırasında hassas form/sonuç temizliği, geç cevap, selector 403 ve gecikmiş delete confirmation güvenliği ayrıca test edilir. Nav ve doğrudan sayfa testleri viewer/editor/team/custom için yönetim request'i yapılmadığını kontrol eder.

Desktop/mobile browser testleri gerçek network isteklerini izleyerek rol/mapping/atama create-update-delete akışlarını, If-Match, tarihsel ownership seçimini, simülasyon/status bağlantısını ve payload endpoint'ine istek olmamasını doğrular. Admin olmayan direct URL testinde management API request sayısı sıfırdır. Server 403 sonrası açık draft ve önceki metadata'nın kaldırıldığı kontrol edilir. Mevcut backend admin/issuer/audit/custom-session testleri gerçek PostgreSQL ile yeniden çalıştırılır.

Sıradaki işler: kalan konfigürasyon endpoint'lerinin granular permission matrisi ve merkezi enforcement'a taşınması; audit local settings/GitOps kapsamları ve migration/rollout readiness işletim araçlarının tamamlanması. Mevcut rol yönetimi ekranları delegated yönetim yetkisi açmaz. Yük testi ve performans iyileştirmesi bu fonksiyonel aşamalar tamamlandıktan sonra ayrı yapılacaktır.

Bu dilimin son doğrulaması: 311 UI unit testi, 18 desktop/mobile browser testi, frontend build, ilgili yeni dosyalarda temiz lint, OpenAPI doğrulaması (94 operation / mevcut 6 captured example) ve `git diff --check` geçti. Gerçek PostgreSQL üzerinde controlplane authorization/custom-session/live-claim güvenlik testleri `-race` ile, merkezi authz paketinin testleri ayrıca `-race` ile geçti. Mobil assignment ve masaüstü migration ekranları screenshot üzerinden görsel olarak kontrol edildi. Yeni migration veya backend rollout değişikliği yoktur; test verileri izole DB/mock sunucularındadır. Yük testi yapılmadı.

## 22. Konfigürasyon endpoint yetkilendirmesi ve Access Management tasarım uyumu

Bu dilimde konfigürasyon API'leri merkezi izin kataloğuna bağlandı. Mevcut global mode okunur: legacy eski sonuçları korur, shadow sadece metadata içeren değerlendirme kaydeder, enforced izin ve kapsam denetimlerini uygular. Varsayılanlar, mevcut kurulumların modları ve bağımsız audit payload modu değiştirilmedi; yeni migration eklenmedi. Önceki bölümlerdeki konfigürasyon enforcement bağlantısının eksik olduğuna dair durum bilgileri bu bölümle güncellenmiştir.

### Endpoint ve alan kapsamı

- Upstream ve virtual server CRUD, katalog/refresh, bağlantı testleri, REST preview/import/mock işlemleri kayıtlı izinlere bağlandı. Liste kapsamı pagination öncesinde SQL'de uygulanır.
- Genel update izni URL/bağlantı, aktivasyon, credentials, TLS, forwarding, REST, publication, policy ve client features değişikliklerini tek başına açmaz. PATCH'te korumalı alanın gönderilmesi ek izin gerektirir; aynı değer veya null göndermek bu kontrolü atlatmaz. Tam PUT varsayılanlara dönebilen korumalı alanların tüm izinlerini gerektirir. Bu konservatif enforced davranışı otomasyon rolleri için ayrıca gözden geçirilmelidir.
- Sahiplik değişikliklerinde mevcut kaynak ve hedef takım/proje ayrı değerlendirilir. Kaynak-ID izni yeni hedefi yetkilendirmez; unmanaged geçişler tenant transfer izni gerektirir. Proje tam replacement sırasında eksik/null sahiplik de transfer kabul edilir.
- API key ve identity binding işlemlerinde önceki ve yeni bütün virtual server hedefleri kontrol edilir. Hedefsiz credential tenant izni ister; tam replacement ile hedefleri silmek kapsam denetimini atlatmaz.
- Team/project, tool grant request/approve/reject/revoke ve bulk işlemleri ayrı kontroller kullanır. Bulk her öğeyi mevcut kısmi sonuç semantiğiyle denetler. Takım issuer/group değişikliği ayrıca mapping yönetim izni gerektirir.
- Control token oluşturma, scope/access/expiry/constraints ve rotate kontrolleri ayrıdır. Custom rol global admin token üretmeye veya başka dinamik kimliğin tokenını döndürerek yetkisini devralmaya izin vermez.
- Audit settings tam belge erişimi/değişikliği tenant kapsamındadır; replacement tüm ilgili ayar gruplarının izinlerini gerektirir. Preview redaction ve settings okuma iznini birlikte ister. Mevcut audit metadata/request/response erişim sınırları bağımsız kalır.
- GitOps export tenant config.export ve içerdiği ailelerin okuma izinlerini; apply config.apply ve her kaynağın alan/hedef izinlerini gerektirir. Dry-run validate, prune ek prune/delete, refresh ek catalog.refresh izni ister. Scoped kısmi export uygulanmadı.
- Omni temel durum ve detay durum izinleri ayrıdır. Runtime JWT/session revoke, kaydedilmemiş bağlantı testi ve yardımcı endpoint'ler açık izin envanterindedir. Bilinmeyen route/action enforced modunda kapalıdır.
- Paylaşılan tool catalog mevcut ortak metadata görünürlüğünü korur; tool_catalog.read gerektirir, kaynak bazlı özel katalog görünümü sağlamaz.

### Tutarlılık ve geçiş sınırları

Yazma transaction'ı config kilidinden sonra authorization revision ve güncel kaynak/hedef bağlamını yeniden doğrular. Arada iptal edilen atama stale isteği 412 ile durdurur. Okumalar repeatable-read snapshot içinde izin, veri ve audit admission bağlamını eşleştirir. Tenant kimliği SQL kapsamlarında da kontrol edilir. OIDC eşlemeleri yalnız güvenilir issuer ve taze doğrulanmış claim snapshot'ından çözülür; legacy admin uyumluluk adaptörü korunur.

Migration status artık gerçek configuration mode değerini bildirir; otomatik hazır/aktivasyon kararı vermez. configuration_migration_review_required blocker'ı kalır. Custom konfigürasyon oturumlarının API erişimi desteklenir; konfigürasyon ekranlarının kaynak/alan bazlı capability uyarlaması bu aşamada kalan işti ve 23. bölümde tamamlandı. Access management yönetimi hâlâ yalnız gerçek global admin içindir. Lokal audit ayarlarının daha dar kapsamlı delegasyonu daha sonra 24. bölümde tamamlandı.

### Arayüz

Access Management, Upstreams/Virtual Servers ile aynı ServerList filtreleri, sıralama, sayfalama, özet kartları, surface-card/panel, DetailRow ve aksiyon düzenini kullanır. Rol, atama ve mapping kartları; simülasyon ve migration panelleri ortak tasarım diline geçirildi. Mevcut admin sınırı, policy revision, stale editör ve oturum temizleme davranışları korunur.

### Doğrulama

Gerçek PostgreSQL testleri legacy/shadow sonuçlarını, pagination öncesi kapsamı, alan ayrımını, tüm credential hedeflerini, ownership transferini, replacement boşluklarını, GitOps sınırlarını ve transaction öncesi atama iptalini kapsar. Route envanteri testi kayıtlı endpoint'lerin eşlemeyi atlamasını önler. SQL kapsamları merkezi karar motoruyla karşılaştırılır; başka tenant bağlamı reddedilir. Frontend ve tarayıcı regresyonları ayrıca çalıştırılır. Yük testi ve rollout aktivasyonu yapılmaz.

Son doğrulama: PostgreSQL üzerinde 7 backend paketinin race regresyonları geçti. Web tarafında 55 dosyada 311 test ve 390/1440 px genişliklerini kapsayan 7 Playwright testi geçti; arama filtreleri, mobil taşma ve yetkisiz yönetim verisinin indirilmemesi doğrulandı. TypeScript/Vite build, ilgili frontend lint, OpenAPI üretim/doğrulama (94 operation, 6 örnek), control-plane build ve diff whitespace kontrolü geçti. Harici WireMock gerektiren TestMockUpstreamThroughVirtualMCP bu backend koşusunda hariç tutuldu; yük testi yapılmadı.

## 23. Custom konfigürasyon ekranlarında kaynak ve alan yetkileri

22. bölümde kalan ekran capability bağlantısı bu dilimde tamamlandı. Global ve audit rollout değerleri, migration verileri ve varsayılan roller değiştirilmedi. Legacy/shadow istemci davranışı korunur; custom konfigürasyon ekranları yalnız mevcut global enforced modunda açılır.

### Self-capability sözleşmesi

- /me mevcut configurationMode değerini bildirir; claim/policy belgelerini açmaz.
- POST /configuration-access yalnız oturumdaki gerçek kimliği değerlendirir. En fazla 64 check; method, göreli API path, isteğe bağlı body ve aynı aileye ait alan/action permission kabul eder. Başka principal, issuer, assignment veya rol verilerek simülasyon yapılamaz.
- Route çözümü uygulamanın kayıtlı ServeMux envanterinden gelir; bilinmeyen endpoint, access management/audit payload yolu, dış URL ve alakasız permission kapalıdır. Kaynak/sahiplik bilgileri DB'den çözülür. Yanıt sadece boolean listesi, revision ve mode içerir; veri nesnesi/claim/assignment içermez.
- Kaynaksız permission sorgusu yalnız herhangi bir uygulanabilir grant bulunduğunu belirten koleksiyon affordance'ıdır. Hedef/create yetkisi değildir. Tam operasyon ve gerçek endpoint kaynak, hedef ve alanları yeniden denetler. Bulk ön kontrol bütün öğeleri ayrı değerlendirir; gerçek bulk API'nin mevcut kısmi sonuç semantiği değişmez.
- Capability gövdeleri örnek credential/payload içerebileceğinden audit request/response payload capture dışında tutulur; metadata kaydı korunur. Yanıtlar no-store'dur. Bu sonuçlar yetki tokenı değildir ve rollout'u aktive etmez.

### Ekran ve ağ sınırı

Navigasyon ve doğrudan URL sınırları izin kontrolü tamamlanmadan konfigürasyon sayfasını mount etmez. Ortak typed API istemcisi enforced modunda her konfigürasyon okuması ve yazması öncesi güncel tam operasyon kontrolü yapar; izin yoksa gerçek veri/işlem endpoint'ine istek göndermez. Asıl API kendi DB/transaction kapsam kontrolünü tekrar uygular; kontrol ile istek arasındaki iptal/transfer yarışı güvenlik açmaz. Bu ek kontrol control plane etkileşimlerindedir; MCP çağrı yoluna eklenmez. Toplu affordance sorguları 64'lük dilimlenir; performans/yük ölçümü sonraki çalışmadır.

Upstream/VS formlarında update ile connection, activation/publication, credentials, TLS, forwarding, REST, policy ve client features ayrı kontrollerdir. TLS istemci kimliği ayrıca credentials ister. Tool policy override da policy izni olmadan değiştirilemez. Kaydedilmiş nesnelerde PATCH yalnız değişen alanları gönderir; TLS/auth alt nesnelerinde de değişmeyen değerler gönderilmez. Böylece metadata izniyle başka alanın yeniden yazılması denenmez.

Yeni kaynaklarda sadece izinli optional alanlar gönderilir; diğerleri mevcut sunucu varsayılanlarına bırakılır. VS create POST kullanır; mevcut VS'lerin tam PUT replacement sınırları korunur. Takım/proje seçimi hedef kapsamını belirler; create tek başına başka hedefe izin vermez. Picker seçenekleri ilgili read izinlerine tabidir.

Credential metadata, rules, constraints, correlation, access, rotate/revoke/delete ayrı değerlendirilir. Mevcut ve önerilen bütün VS hedefleri korunur; picker yetkisiz hedef eklenmesini engeller. Ownership editörü tam önerilen POST'u kontrol eder, issuer/group değişikliği ek mapping iznine tabidir. Tool grant request/approve/reject/revoke ve bulk onayı ayrı değerlendirilir. Settings/GitOps, audit settings, Omni, REST preview/execute ve mock sync kendi izinlerini kullanır. Access Management gerçek global admin sınırında kalır.

Oturum/kimlik değişimi devam eden preflight sonucunun yeni kimlikle işleme dönüşmesini engeller. Önceden okunmuş kaynak için erişim reddi veya claim freshness süresi dolduğunda mount edilmiş konfigürasyon ve draft'lar temizlenip haklar tekrar kontrol edilir. Veri/payload, claim örneği ve capability browser storage'a yazılmaz; mutasyon otomatik tekrarlanmaz.

### Doğrulama kapsamı

Self-capability entegrasyonları canlı kaynak/alan kararını, atama iptalini, başka kimlik enjeksiyonunun reddini, mode değerlerinin değişmemesini, kontrolün işlem gerçekleştirmemesini ve strict audit capture açıkken probe gövdelerinin payload tablosuna yazılmamasını doğrular. Ağ seviyesindeki frontend testleri denied GET/PATCH'in gerçek endpoint'e gitmediğini, cancellation/authentication/no-store davranışını ve legacy/shadow akışlarını kapsar. Tarayıcı senaryoları metadata-only upstream/VS düzenleme, doğrudan yetkisiz kaynak, açık draft sırasında iptal, bağımsız credential rotate/field sınırları ve scoped create'te yasak varsayılan alanların gönderilmemesini kapsar. Mevcut admin Access Management ve navigasyon regresyonları da korunur.

Migration readiness otomatik true yapılmaz. Gerçek IdP/CI/GitOps rol envanteri ve kurtarma erişimi incelemesi, ardından planlanan yük testi ve ölçüme dayalı iyileştirmeler rollout öncesi devam eder.


Son doğrulama: 7 backend paketinin PostgreSQL/race regresyonları ve son capability/kaynak kontrollerinin ek race koşusu geçti. Frontend 56 dosyada 320 test, tarayıcıda 18 test geçti. Buna claim expiry sonrası draft temizliği ve kimlik değişimi sırasında onaylanmış preflight'ın gerçek işleme dönüşmemesi de dahildir. TypeScript/Vite build, control-plane build, ilgili lint, OpenAPI üretim/doğrulama (95 operation, 6 örnek) ve diff whitespace kontrolü geçti. Harici WireMock gerektiren TestMockUpstreamThroughVirtualMCP regresyon koşusunda hariç tutuldu. Geçici PostgreSQL konteyneri kapatıldı. Yük testi yapılmadı; rollout/migration modları değiştirilmedi.

## 24. Kaynak bazlı yerel audit redaction yönetimi

Upstream ve virtual server için GET/PUT /{family}/{slug}/audit-settings ve POST /{family}/{slug}/audit-settings/preview eklendi. GET sadece ilgili local rules ve ortak settings revision'ını döndürür; global regex, diğer kaynaklar, capture, retention, exporter ve durability ayarları indirilmez. PUT transaction içinde güncel ayarları okuyup yalnız ilgili map girdisini değiştirir; ortak revision çatışması 409'dur. Boş rules dizisi yerel kuralları temizler; eksik/null rules kabul edilmez.

GET audit.settings.read, PUT audit.redaction.local.update; preview hem audit.settings.read hem audit.redaction.preview ister. Kaynak UUID/team/project/epoch DB'den çözülür. Bu yeni endpoint'ler mevcut geçiş modu değerlerinden bağımsız olarak doğrudan kaynak yetkisi uygular; mevcut rollout değerleri değiştirilmez. Gerçek global admin uyumluluğu bu dilimde korunur. Genel upstream/VS update izni bu audit izinlerinin yerine geçmez.

Preview yalnız revision/rules/direction/sentetik payload kabul eder. editScope, upstream, virtualServer, global settings veya başka kaynak haritaları kabul edilmez. Sunucu güncel global ve mandatory kuralları ekler; scope ve resource route'tan sabitlenir. Preview capture kapalıysa production motoruyla uyumlu disabled sonucu verir. Preview bir kayıtlı audit mesajı okuma yolu değildir.

Yerel GET/PUT/preview metadata audit'i kaynak snapshot'ına bağlıdır; gövdeler ve preview örnekleri audit payload olarak saklanmaz. Ayarlar/sentetik mesajlar no-store kullanır. Yazma öncesi authorization revision ve provenance yeniden doğrulanır; admission sonrası revoke veya ownership transferi eski kapsamla işlemi tamamlatmaz. GET/preview repeatable-read snapshot kullanır.

Kaynak ekranları ayrı LocalAuditSettingsPanel kullanır. Yalnız yerel endpoint indirilir; global settings veya ilişkili kaynak listesi indirilmez. Okuma, düzenleme ve preview hakları ayrıdır; global capture/durability kontrolleri yerel ekranda bulunmaz. Geçersiz JSON iki yönden herhangi birindeyse preview isteği gönderilmez. Başarısız save draft'ı korur; açık reload draft ve örnekleri temizler.

### Major release kararı — önceki rollout/migration hedeflerinin güncellenmesi

Kullanıcı uygulamanın henüz kullanılmadığını ve yeni major release yayınlayacağını belirtti. Hedef mimari yalnız enforced yetkilendirmedir; legacy/shadow desteğini sürdürme, eski kullanım için otomatik rol import'u ve aşamalı legacy→shadow→enforced işletimi artık ürün gereksinimi değildir. Önceki bölümlerdeki bu geçiş işleri major release hedefinden çıkarılmıştır.

Bu yerel audit dilimi mevcut rollout değerlerine dokunmaz. Ayrı sonraki sadeleştirme diliminde backend legacy/shadow dalları, UI mode bağımlılıkları, mode alanları/kolonları ve status/simulation sözleşmeleri birlikte gözden geçirilerek tek enforced modele geçirilecektir. Global admin/bootstrap kurtarma politikası açıkça korunmalı; runtime OAuth ile control-plane kimlik ayrımı ve issuer güven sınırı sürdürülmelidir. Eski audit provenance sınıflandırması olan legacy, rollout modu ile aynı kavram değildir: tarihsel/bağlamsız kayıtları güncel kaynağa bağlayarak yetki genişletmek sadeleştirme kapsamında da yapılmaz.

Yük testi bu oturumda yapılmaz. İşletim dokümanı yeni major release kurulumuna göre sadeleştirilir; desteklenmeyen eski binary/session geçişlerini otomatik destekleme vaadi verilmez.

Son doğrulama: 7 backend paketinin PostgreSQL/race regresyonları ve yeni yerel audit testlerinin son ek race koşusu geçti. 320 frontend testi ve yerel kaynak/konfigürasyon yetkilerini kapsayan 12 tarayıcı testi geçti. Build, ilgili lint, OpenAPI üretim/doğrulama (101 operation, 6 örnek) ve diff whitespace kontrolü başarılı. Harici WireMock isteyen tek test geniş backend koşusunda hariç tutuldu. Yeni migration veya rollout değeri değişikliği yoktur; yük testi yapılmadı.

## 25. Major release — yalnız zorunlu yetkilendirme modeli

Legacy/shadow runtime desteği kaldırıldı. Migration 40 authorization_state.mode ve audit_payload_mode kolonları ile authorization_mode_revision trigger/function'ını kaldırır; authorization revision artırılır. Önceki versioned migration dosyaları tarihçe olarak korunur. Down yalnız önceki şemayı enforced varsayılanlarıyla geri getirir; sessizce yetkiyi kapatmaz. Gerçek uygulama hiçbir mode selector okumaz.

### Backend sözleşmesi

Konfigürasyon, audit metadata ve payload erişimi her istekte merkezi izin/kapsam kararına tabidir. Coarse permit/LegacyDenial ve shadow değerlendirme kaydı kaldırıldı. Politika yükleme tek fonksiyon ve tek snapshot kullanır; DB yoksa bootstrap dahil korunan API 503 ile kapalıdır. Bilinmeyen endpoint/action için genel read/update fallback'i yoktur. Transaction revision, live ownership, tenant, multi-target credential, field/PUT/default ve issuer/freshness sınırları korunur.

Global admin/bootstrap gerçek yönetici kimliğidir; merkezi politikaya açık tenant izinleri eklenir. Custom rolün platform-admin slug'ı veya access.* izinleri gerçek yönetici kimliği üretmez. Viewer/editor/team/custom etiketleri ve takım üyeliği tek başına konfigürasyon veya audit hakkı üretmez. Non-admin control token'ın wire scope adı read olarak korunur; bu artık GET-only veya otomatik okuma hakkı değildir, açık atamalarla yetkilendirilen kimliktir. Atanmış write izinleri uygulanabilir. Scope admin üretme/rotation ve başka atanmış kimliğin tokenını devralma korumaları sürer.

Audit provenance sınıflandırması legacy korunur: bu bir rollout modu değildir. Bağlamsız/tarihsel kayıtları tenant legacy izinleri olmadan okumak mümkün değildir. Request ve response ayrı, capture scope ve epoch/multi-domain sınırları zorunludur. Audit durability strict/relax ve runtime tool policy modları bu değişiklikten bağımsızdır.

### Arayüz ve API

/me içinden configurationMode; capability yanıtlarından mode; simulation/status içinden mode, auditMode, configurationEnforcement, readiness ve legacy compatibility alanları kaldırıldı. Eski /access/migration-status yerine GET /access/status vardır; Access status ekranı yalnız issuer/session olanakları, uyarılar ve sayımlar sunar. Otomatik activation/rollout düğmesi veya garantisi yoktur.

Konfigürasyon arayüzü yalnız server-owned capability sonucunu kullanır; rol etiketinden edit/read fallback'i kaldırıldı. Her veri/işlem isteği ön kontrol ve gerçek API kararıyla korunur; eksik/başarısız kontrol admin-shaped UI oturumunda bile veri indirmez. Yalnız değişen alanların PATCH'i, source-bound local audit ayarı/preview ve oturum/claim süresi sonrası temizleme korunur.

Birim bileşen regresyonları açık senkron test-policy fixture'ları kullanır; gerçek asynchronous hook/transport sınırı tarayıcı testleri ve transport birim testlerinde doğrulanır. Ürün koduna test veya legacy bypass eklenmedi.

### Yayın ve doğrulama

Bu major release kırıcıdır: eski mod kolonuna yazan araçlar, eski response alanlarını bekleyen istemciler ve migration-status URL'si güncellenmelidir. Yeni kurulumda bootstrap/admin ile giriş yapılıp non-admin rol/atama/claim eşlemeleri açıkça oluşturulur. Eski coarse role davranışını otomatik içeri alma veya gölge geçiş gereksinimi yoktur.

Yeni testler query/header üzerinden legacy/shadow/enforced adlarının erişimi değiştirmediğini, atamasız token reddini, açık read ve request-only grant'in hemen çalışmasını, response reddini, mode kolon/alanlarının yokluğunu ve politika DB'si yokken fail-closed davranışı doğrular. Mevcut kaynak, alan, sahiplik transferi, audit payload, local rules, issuer, session, CSRF ve management admin sınırları regresyonlarda korunur. Yük testi yapılmaz.

Son doğrulama: 7 backend paketinin PostgreSQL/race regresyonları, son authorization/audit/kaynak kontrollerinin ek race koşusu ve migration 39→40→39→40 koruma testi geçti. 314 frontend testi ve 31 gerçek tarayıcı senaryosu geçti; eski coarse-role/mod testleri yeni zorunlu-model beklentileriyle değiştirildi. TypeScript/Vite build, control-plane build, Go vet, frontend lint, OpenAPI üretim/doğrulama (101 operation, 6 örnek) ve diff whitespace kontrolü başarılı. Harici WireMock isteyen TestMockUpstreamThroughVirtualMCP geniş koşuda hariç tutuldu. Yerel mock provider entegrasyonunda scoped non-admin sync/execute ve başka kaynak reddi ayrıca doğrulandı. Yük testi veya yayın/deployment yapılmadı.


## 26. Hazır rol iş akışları, bağımsız OIDC atamaları ve demo kurulumu

### Kapsam ve uygulama sırası

Major release değerlendirmesindeki ilk üç eksik bu dilimde ele alınır: (1) hazır rolün ekran bağımlılıkları, (2) takım etiketli OIDC oturumunun merkezi politikadan önce reddedilmesi, (3) quickstart/demo için açık console rol eşlemeleri. Yük testi ve yayın bu dilimin dışındadır. Gerçek IdP ile uçtan uca kabul, çok control-plane düğümlü işletim kabulü ve diğer yayın incelemeleri ayrı kalır.

### 1. Hazır rol ve ekran bağımlılıkları

Migration 41 yalnız migration tarafından yönetilen builtin şablonların izinlerini ve sürümlerini günceller. Mevcut atamalar korunur; yeni atama oluşturulmaz. Daha önce builtin'den kopyalanmış custom roller sabit izin kümeleridir, genişletilmez. Yeni şablonu isteyen admin kopyasını açıkça günceller veya yeni kopya oluşturur. Mevcut builtin atamaları yeni bağımlılıkları hemen kazanır; bu genişleme release incelemesinde görünür olmalıdır. Down yalnız eklenen izinleri çıkarır; 40→41→40→41 korunumu test edilir.

| Hazır rol | Tamamlanan iş akışı / ek izin | Korunan sınır |
| --- | --- | --- |
| Virtual Server Editor | `projects.read`: kendi kapsamındaki proje seçimi, draft oluşturma ve metadata/tool düzenleme. `tool_catalog.read` mevcut şablonda vardır. | Publication, policy, client features, ownership transfer ve upstream konfigürasyonu eklenmez. Project/resource ataması upstream okuma hakkına dönüşmez. |
| Credential Manager | `virtual_servers.read`: kendi kapsamındaki hedefleri seçerek key/binding yönetimi. | Hedef okuma, VS yazma hakkı değildir. Çok hedefli credential için bütün hedefler kapsamda olmalıdır; diğer takım hedefi eklenemez. |
| Automation Deployer | Tam export için `identity_bindings.read`, `teams.read`, `projects.read`, `tool_grants.read`, `audit.settings.read`; kaynak apply için upstream activation/credentials/TLS/identity forwarding/REST ve VS publication/policy/client features alan izinleri; catalog read/refresh. | Tenant ataması gerekir. Kaynak odaklı document validate/apply ve export desteklenir. Tam export'un geri yüklenmesi ayrıca governance/binding/audit yazma izinleri isteyebilir; şablon bu hakları otomatik vermez. Prune/delete, ownership transfer, access management, control token ve audit event/payload hakları verilmez. Managed mock içeren REST document için `upstreams.mock.sync` ayrıca atanır. |
| Observer / Upstream Maintainer / Access Approver / Auditor | İzinleri değiştirilmez; mevcut görev sınırları korunur. Observer takım/proje kaynak listelerinde ayrı bir rol olarak birleştirilebilir. | Maintainer create/delete/activation veya secret yönetimi anlamına gelmez. Approver, editör veya platform yöneticisi değildir. Auditor yalnız metadata görür. |

Virtual server editöründe upstream konfigürasyonu ve kaynak tool katalogları yardımcı veri olarak ele alınır. Yalnız istemcinin `configuration_access_denied` ön kontrol sonucu yardımcı kataloğu boşaltır; 401, sunucudan gelen 403 ve ağ hataları görünür kalır. Gerçek istek gönderilmez ve önceki yardımcı veri temizlenir. Paylaşılan tool kataloğu kendi izniyle okunmaya devam eder. Ana kaynak yükleme hatası gizlenmez. Oturum/izin iptalindeki mevcut ekran temizleme mekanizması korunur.

### 2. Takım oturumu ve merkezi karar

OIDC `team` etiketi mevcut takım üyeliğini bir yetkilendirme ön koşuluna dönüştürmez. Doğrulanmış, iptal edilmemiş oturumun kimliği session TTL boyunca geçerlidir. Takım grubu değiştiğinde `/me.teamMember` güncel durumu gösterir; bu değişiklik aynı subject'e verilmiş bağımsız rolü iptal etmez. Direkt atamanın iptali sonraki istekte uygulanır.

Groups freshness dolduğunda groups temizlenir, kimlik oturumu sonlandırılmaz. Claim freshness dolduğunda claim eşlemeleri kullanılamaz; geçerli direkt subject atamaları kullanılabilir. Issuer, encrypted/server-held session, session expiry/revocation ve CSRF kontrolleri korunur. Takım etiketi veya grup üyeliği açık atama yokken hiçbir kaynak hakkı vermez. Takım sahipliği ile claim eşlemesi ayrı kavramlardır: grup bazlı yetkiyi iptal etmek için eşleme/atama veya IdP grup üyeliği ayrıca değiştirilmelidir.

### 3. Quickstart ve demo açık rol eşlemeleri

Keycloak etkin demo kurulumu takım UUID'lerini admin scope selector API'sinden çözer. Güvenilir issuer ve `groups array_contains <exact group>` koşuluyla takım başına eşleme oluşturur. Takım kapsamına Observer, Upstream Maintainer, Virtual Server Editor, Credential Manager ve Access Approver rolleri atanır. Bu profil kendi takımının kaynaklarını okuma, belirli bakım/draft/credential ve onay işlemlerini gösterir; bütün konfigürasyon alanlarını yönetme veya kaynak silme vaadi yoktur. Console yardım çıktısı bu erişimi açık takım kapsamlı roller olarak tanımlar.

Auditor grubuna tenant kapsamında yalnız Auditor atanır: event metadata okunabilir; request/response, identity/details ve bağlamsız tarihsel audit izinleri verilmez. Gerçek platform admin grubu admin kimliği olarak kalır. Viewer/editor coarse group ayarları demo yetkisinin kaynağı olmaktan çıkarılır. Runtime identity binding ve MCP token erişimi ayrı provision edilir; console rolü runtime erişim üretmez.

Seeder create-only çalışır: mevcut disabled mapping/assignment güncellenmez, kullanıcı düzenlemeleri korunur. Oluşturulan eşleme ve atamaların ID makbuzları mevcut installer ledger'ına yazılır; silinmiş veya değiştirilmiş haklar yeniden seed sırasında geri açılmaz. Eşleme grubu değişmişse eski grup adına yeniden eşleme oluşturulmaz. Farklı issuer veya değiştirilmiş group taşıyan mevcut demo takımı için yeni console grant oluşturulmaz. Ledger korunmalıdır; kaybı durumunda seeder geçmiş kullanıcı silmelerini bilemez. Installer API ile ledger yazımı tek transaction değildir; başarısız/yarıda kalan çalıştırmada mevcut kayıtlar tekrar listelenerek çoğaltma önlenir. IdP kurulumu ve gerçek SSO roundtrip bu dilimde otomatik canlı test edilmemiştir.

### Kabul ölçütleri

- Gerçek builtin şablonlarıyla scoped credential hedef seçimi/oluşturma/rotation ve VS proje seçimi/draft oluşturma başarılıdır; kapsam dışı hedefler ve korumalı alanlar reddedilir.
- Automation export ve kaynak document dry-run/apply başarılı; prune, access management, control token ve audit event erişimi reddedilir.
- OIDC takım üyeliği kaybolsa da direkt grant çalışır; atama iptali hemen etkili olur. Süresi geçen grup verisi kullanılmaz.
- Tarayıcıda VS editörü yetkisiz upstream konfigürasyonunu indirmeden çalışır; yardımcı kaynak hatası gerçek kimlik/sunucu hatalarını gizlemez.
- Migration custom kopyaları ve mevcut atamaları korur; demo tekrarı iptal/disabled/değiştirilmiş erişimi geri açmaz.

Son doğrulama: 7 backend paketinin PostgreSQL/race regresyonu geçti; son takım üyeliği/claim/direct grant kontrolleri ayrıca race ile yeniden çalıştırıldı. Migration 40→41→40→41 testi custom kopyaları, atamaları ve şablon sürümlerini korudu; migration 40'ın ayrı testi kendi hedef sürümüne sabitlendi. 319 frontend testi, konfigürasyon yetkileri için 10 gerçek tarayıcı senaryosu ve 40 Helm/quickstart Python regresyonu geçti. Frontend build/lint, OpenAPI doğrulama (101 operation, 6 örnek), Go vet ve whitespace kontrolü başarılı. Geniş backend koşusunda harici WireMock gerektiren tek TestMockUpstreamThroughVirtualMCP hariç tutuldu. Canlı cluster/gerçek Keycloak kurulumu, yayın ve yük testi yapılmadı.


## 27. Platform yöneticisi ve kurtarma sözleşmesi

Kullanıcı bu sözleşmeyi onayladı. Gerçek platform yöneticiliği yalnız deployment bootstrap token, admin scope'lu named control token veya yapılandırılmış OIDC admin grubundan doğrulanmış oturum ile oluşur. Custom rol, claim/scope eşlemesi veya builtin ataması gerçek yönetici kimliği üretemez. Access Management gerçek admin sınırında kalır. Gerçek admin bütün konfigürasyon ve audit request/response payload erişimine sahiptir; normal CSRF, DB/policy, tenant ve audit admission/durability kontrolleri devam eder.

Migration 42 builtin platform-admin rolünü aynı UUID ile platform-operator / Platform Operator olarak yeniden adlandırır; access.* izinlerini şablondan çıkarıp sürümünü artırır. Diğer operasyon izinleri ve açık capture kapsamına bağlı audit payload hakları korunur. Atamalar aynı role bağlı kalır; custom kopyalar otomatik değiştirilmez. Yeni isimle çakışan custom rol varsa migration transaction'ı hata verir; custom rolün üzerine yazılmaz. Eski slug kullanan scriptler güncellenir; geriye dönük alias eklenmez. Down önceki builtin şablonunu geri getirir, runtime yönetici kimliği yaratmaz.

enabledPlatformAssignments ve buna bağlı son atama/son mapping condition-disable engelleri kaldırılır. Bütün operator atamaları/mapping'leri iptal edilebilir. Referanslı rol/mapping silme koruması, optimistic locking, transaction serialization, issuer güven sınırı ve kapsam/payload pin kuralları korunur. Bir rol ataması sayısı IdP girişinin, erişilebilir secret'ın veya gerçek yönetim yetkisinin garantisi değildir. Access status ve simülasyon kurtarma hazır sertifikası değildir.

VMCP_ADMIN_TOKEN uygulama başlangıcında zaten zorunludur; API üzerinden kapatılamaz. Son named admin token revoke/expire edilebilir; bunun yerine sahte bir son-login garantisi eklenmez. İşletim sözleşmesi deployment ve secret-store erişimini bağımsız kurtarma kökü olarak kabul eder. Kurtarmada bootstrap secret değiştirilir ve bütün CP replikaları yeniden başlatılır. OIDC başlangıcı bloke ediyorsa deployment'ta issuer geçici kaldırılarak token-only başlatılır. DB/master key veya rol tabloları sıfırlanmaz. Normal admin erişimi doğrulanıp geçici kurtarma secret'ı tekrar değiştirilir; boş credential ile kapatma desteklenmez. Eski bootstrap, named token ve mevcut OIDC session iptalleri ayrı işlemlerdir.

Admin arayüzünde rol atamalarının platform yöneticiliği vermediği açıklanır; hayali son-admin-ataması uyarısı kaldırılır. OpenAPI son-atama 409 sözleşmesini kaldırır; gerçek referans ve issuer conflict'leri sürer. Operasyon adımları docs/SECURITY_OPERATIONS.md içindeki Administrator recovery bölümündedir.

Kabul: operator/custom access.* ataması Access Management veya admin token üretme/rotate yetkisi vermez; gerçek bootstrap/named/OIDC admin korunur; operator bütün atamaları ve mapping'leri eşzamanlı iptal edilebilir; referans/issuer sınırları devam eder; atamasız ve named admin iptal edilmiş token-only kurulum bootstrap ile yönetilebilir; secret değişince eski bootstrap reddedilir. Migration 41→42→41→42 rol UUID, custom kopya ve atama korunumu ile test edilir. Yük testi yapılmaz. Gerçek IdP, secret-store ve çok replikalı kurtarma tatbikatı yayın kabulü olarak ayrıca yapılmalıdır.

Son doğrulama: 7 backend paketinin PostgreSQL/race regresyonları ve migration 41→42→41→42 koruma testi geçti. 320 frontend testi, Access Management ve konfigürasyon kapsamları için 17 gerçek tarayıcı senaryosu, 40 Helm/quickstart Python testi başarılı. Tarayıcı öncesi TypeScript/Vite build, frontend lint, OpenAPI üretim/doğrulama (101 operation, 6 örnek), ilgili Go vet ve whitespace kontrolü geçti. Bootstrap kurtarma işleminin actor_type=bootstrap-token / actor_id=bootstrap metadata'sıyla kaydı ve credential'ın metadata'da bulunmaması test edildi. Geniş backend koşusunda harici WireMock isteyen TestMockUpstreamThroughVirtualMCP hariç tutuldu. Canlı IdP/çok replika kurtarma tatbikatı, yayın/deployment veya yük testi yapılmadı.


### Quickstart ve uzak yük testi hazırlık kontrolü

Yalnız --keycloak akışı da --mock --keycloak ile aynı açık console eşlemelerini
oluşturacak şekilde tamamlandı; runtime fixture oluşturmaz. Tekrar kurulumdaki
silinmiş/disabled atamalar ledger ile korunur. Quickstart kılavuzundaki eski global
viewer açıklaması kaldırıldı; gerçek admin, Platform Operator ve metadata-only
auditor ayrımı belgelendi. Helm/quickstart Python regresyon sayısı 41 oldu.

Mevcut benchmark/kubernetes düzeneği güncel çalışma ağacını derler ve geçici DB'yi
güncel migration'a taşır; uzak sunucuda MCP/metadata-audit baseline ölçümüne uygundur.
Varsayılan profil payload capture, control-plane granüler kararları ve audit okuma
API trafiği üretmez; bu yolların performansının ölçüldüğü iddia edilmez. Tam kaynak
veya yeni major image/digest sunucuya aktarılmalıdır; eski yayınlanmış image'lar
lokal değişiklikleri içermez. Bu makinede smoke veya yük testi çalıştırılmadı.
