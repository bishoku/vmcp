# Ekip/proje sahipliği: fonksiyonel doğrulama ve MicroK8s performans karşılaştırması

26 Ekim 2026 tarihinde ekip/proje sahipliği ve tool onay akışı geliştirmesi için yapılmıştır. Geliştirme ve fonksiyonel testler tamamlandı. Yük testi bulguları nedeniyle performans/genel kabul tamamen başarılı değildir.

## Kapsam

- Ekipler doğrulanmış OIDC issuer ve grup claim değerlerine bağlanır; yerel kullanıcı veya üyelik yönetimi eklenmez.
- Platform admin ekip/proje eşleştirmelerini yönetir. Upstream tek ekibe; vMCP tek projeye bağlanır ve projenin tek sahip ekibini devralır.
- Mevcut vMCP'ler Default projeye taşınır; mevcut upstream'ler sahipsiz kalır. Global editor/viewer davranışı bu kaynaklarda korunur; eşleşmeyen kullanıcıya yeni yetki verilmez.
- Ekipler arası tool erişimi vMCP/upstream/tool bazında istek ve upstream sahibi onayı gerektirir. Sahipsiz upstream onay gerektirmez. Default proje, sahipli upstream onayını atlatamaz.
- Listeleme, düzenleme, credential kapsamı, ownership transferi ve GitOps yolları yetkilendirilir. Transfer açık grant'leri iptal eder. Dinamik discovery ve semantic search kapsam dışıdır.
- Onay filtreleme konfigürasyon snapshot'ında uygulanır; tool çağrısına ek ownership SQL sorgusu eklenmez. Onay iptali mevcut snapshot tazelik/yayılım mekanizmasına tabidir.

Ürün davranışı ve kullanım örneği: [Teams and Projects](../../docs/Teams-and-Projects.md).

## Fonksiyonel doğrulama

- Gerçek PostgreSQL ve Redis ile `go test -race ./...`: geçti.
- `go vet ./...`: geçti.
- Arayüz: 48 dosyada 250 Vitest testi; lint, build ve OpenAPI doğrulama: geçti.
- Tarayıcı: tam koşudaki 148 senaryonun 146'sı ilk koşuda geçti. Yeni menü maddesi nedeniyle iki testin bağlantı sayısı beklentisi 8'den 9'a güncellendi; ilgili üç navigasyon testi yeniden çalıştırılarak geçti. Ekip/proje oluşturma ve request/approve/revoke masaüstü ve mobil senaryoları geçti.
- Benchmark Python birim testleri: geçti.
- Eklenen kontroller: migration upgrade/rollback; legacy Default uyumu; issuer/grup izolasyonu; kaynak ve credential kapsamı; onay/version çatışması; sahiplik değişimi; API export/apply; OIDC claim tazeliği; açık MCP oturumunda list/call onay ve iptal davranışı.

## Ölçüm yöntemi

Her koşuda 15 saniye ısınma ve **600 saniye 1.000 çağrı/s hedefli sabit yük** uygulandı. 300 upstream, 300 vMCP, 60.000 tool, 600 kalıcı oturum ve 64 agent key kullanıldı. REST fixture yanıtına 5 ms gecikme eklenir; schema, routing, nonce ve PII sonuçları doğrulanır. Audit strict modu açıktır. Süresi dolmuş 100.000 operation/stream fixture'ı delivery temizliğini de çalıştırır.

Yerel tek düğümlü MicroK8s'te ayrı test namespace'leri; iki data-plane (her biri 8 CPU limit, GOMAXPROCS=8, GOMEMLIMIT=768MiB, 3GiB bellek limit), bir control-plane, PostgreSQL ve Redis. PostgreSQL tuned profili ve durable yazım ayarları bütün koşularda aynıdır. Mevcut vmcp-demo değiştirilmez; test namespace'leri koşu sonunda kaldırılır.

1. **Önce:** sahiplik geliştirmesi öncesindeki runtime; Default envanter.
2. **Sonra / Default:** yeni kod, sahipsiz upstream ve Default proje.
3. **Sonra / sahipli:** aynı yeni kod; sağlayıcı ekip upstream'lerin, tüketici ekip projenin sahibi; 60.000 ekipler arası tool grant'i approved.

Sonraki iki koşu ardışık yürütüldü; ölçüm sırasında ayrı derleme/test yükü başlatılmadı. Önceki ölçüm geliştirmeyle aynı host üzerinde yürüdüğünden arka plan yükü olası karıştırıcı etkendir. Her profil bir kez çalıştırıldı; istatistiksel güven aralığı yoktur. Bu nedenle farklar gözlenen sonuçlardır, tek başına nedensellik veya üretim kapasitesi garantisi değildir.

## Sonuçlar

| Koşu | Başarılı çağrı/s | Başarı % | p95 ms | p99 ms | Hata | Gönderilemeyen | Tepe RSS MiB | Yük SLO |
|---|---:|---:|---:|---:|---:|---:|---:|---|
| [Önce](microk8s-20261026-215339/report.md) | 999.8 | 100.000 | 131.6 | 168.8 | 1 | 33 | 971.0 | Geçti |
| [Sonra / Default](microk8s-20261026-223317/report.md) | 999.1 | 100.000 | 129.9 | 421.0 | 1 | 442 | 972.8 | Geçti |
| [Sonra / sahipli](microk8s-20261026-224955/report.md) | 989.9 | 99.001 | 123.0 | 146.6 | 5993 | 33 | 970.4 | Geçmedi |

Tablo yalnızca 600 saniyelik sabit yükü gösterir. p95/p99 yalnızca başarılı çağrıların gecikmesidir; başarısız çağrılar düşük gecikme yüzdelikleriyle gizlenmemelidir. Başarı yüzdesi gönderilen çağrılara göredir. RSS iki data-plane podundan yüksek olanın koşu boyunca örneklenen tepe değeridir.

**Default:** throughput değişimi −%0,072, p95 −%1,31; p99 168,8 ms'den 421,0 ms'ye çıktı (+%149,5). Gönderilemeyen çağrı 33'ten 442'ye çıktı; 402'si 81–85. saniyelerde toplandı. Yük SLO'su geçti, ancak bu kuyruk gecikmesi farkı nedeniyle “performans etkisi yok” denemez. Bu koşu yığılmanın kök nedenini göstermiyor.

**Sahipli:** p95 123,0 ms ve p99 146,6 ms olsa da %99,001 başarı, %99,9 kabul eşiğinin altında. 5.993 hatanın 2'si HTTP 502, 5.991'i HTTP 503. İki 502 24. ve 26. saniyelerde; 503'ler 28–34. saniyelerde görüldü. Bu desen benchmark'ın pasif-health proxy'sinin transport hatasında node'u 10 saniye karantinaya alması ve her iki node karantinadayken 503 üretmesiyle uyumlu. Proxy alttaki transport hatasını kaydetmediği için kesin kök neden belirlenemedi; sahiplik değişikliğinden kaynaklanmadığı da kanıtlanmış değildir. Pod restart gözlenmedi. Bu koşu performans kabulünden **geçmedi**.

Tepe RSS üç koşuda da yaklaşık 970–973 MiB düzeyinde; belirgin kalıcı bellek artışı gözlenmedi. Medyan data-plane CPU değerleri pod başına yaklaşık %767–769 (yaklaşık 7,7 çekirdek). Bu yük CPU sınırına yakın; sonuçlar kapasite marjı olarak yorumlanmamalıdır. Snapshot oluşturma süresinin bağımsız önce/sonra ölçümü yapılmadı.

### Tutarlılık ve ortak kabul bulgusu

| Koşu | Başarılı çağrı / eşleşen admission / outcome | Çift çalıştırma | Açıklanamayan eksik outcome | Polling p95 / max ms |
|---|---:|---:|---:|---:|
| Önce | 600716 / 600716 / 600716 | 0 | 0 | 276.7 / 293.0 |
| Sonra / Default | 600306 / 600306 / 600306 | 0 | 0 | 265.2 / 272.0 |
| Sonra / sahipli | 594724 / 594724 / 594724 | 0 | 0 | 273.4 / 285.9 |

Bu tutarlılık sayıları ısınma ve sabit yük toplamıdır. Polling testi üç koşuda da 500 ms bütçesi kontrolünden geçti (24 örneğin her biri bütçe içinde).

**Üç koşunun genel kabul sonucu da başarısızdır:** `key_revocation_propagated_and_rejected_on_both_nodes` kontrolü geliştirme öncesinde ve sonrasında başarısız oldu. Mevcut hata mesajı key'in kabul edilmesi ile snapshot revision bekleme zaman aşımını ayırmıyor. Bu önceki bulgu düzeltilmiş sayılmadı; yeni tool-grant iptal testinin başarılı olması agent-key kabul bulgusunu kapatmaz. Üretime geçiş öncesinde bu bulgu ve sahipli koşunun transport/503 kesintisi ayrıca çözülmelidir.

### Kaynak kimliği ve kanıtlar

Önce kaynak hash'i: `995115ff7259ed6f7f18c3565ecdf099d09d4c15123f0be909b89c6ddcdac9cc`.
Default ve sahipli kaynak hash'i: `521193b877caaa33a6857d9fbc0ee9f6c49c12efbc83cc6934b892bcb25cb1a3`.
İki sonraki koşuda kaynak aynıdır. Çalışma ağacı değişiklikleri binary'ye dahildir; HEAD tek başına sürümü tanımlamaz.

- [Önce: özet ve arşiv kimliği](microk8s-20261026-215339/reproduction.json)
- [Default: özet ve arşiv kimliği](microk8s-20261026-223317/reproduction.json)
- [Sahipli: özet ve arşiv kimliği](microk8s-20261026-224955/reproduction.json)

Ham kanıtlar `artifacts/benchmark-runs/ownership-{baseline,default,managed}-20261026` ve `artifacts/benchmark-archives` altında yereldir. Başlangıç rapor üreticisindeki sabit “300 saniye” metni yayımlanan raporda 600 olarak düzeltildi; ham profile ve PlannedSeconds alanları 600 saniyeyi doğrular, ham arşiv değiştirilmedi.

Tekrarlama (her komutta yeni output dizini kullanılmalı):

```sh
VMCP_ACCEPTANCE_STEADY_SECONDS=600 python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8 --postgres-tuned --output artifacts/benchmark-runs/ownership-default-repeat
VMCP_ACCEPTANCE_STEADY_SECONDS=600 VMCP_ACCEPTANCE_OWNERSHIP=1 python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8 --postgres-tuned --output artifacts/benchmark-runs/ownership-managed-repeat
```


## Sınırlamalar

Bu profil yük altında ownership/grant mutasyonu, IdP oturum açma throughput'u, proje yönetimi API kapasitesi veya çok sayıda farklı ekibin yönetim sorgularını ölçmez. İki ekip ve tek sahipli proje ile 60.000 grant'in veri düzlemi maliyetini ölçer. Grant iptal davranışı fonksiyonel testte doğrulanır; bu yük koşusu altında grant iptal yayılım gecikmesi için bir SLO çıkarılamaz. Üretim ingress'i, gerçek upstream ağları, uzun soak, PostgreSQL HA/failover ve sınırlı Redis quota profili kapsam dışıdır.

İlk sürüm ekip/proje silme/arşivleme ve proje kapsamlı GitOps içermez. OIDC ekip claim'leri 15 dakika tazelik sınırına sahiptir; anlık LDAP üyelik bildirimi/SCIM senkronizasyonu değildir.
