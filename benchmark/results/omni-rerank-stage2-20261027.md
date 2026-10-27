# Omni rerank / no-match — aşama 2, 2026-10-27

## Sonuç

Opsiyonel rerank servisi, query-level no-match ve belirsiz skorda retrieval sırasını
koruma politikası eklendi. **Varsayılanda reranker hâlâ kapalı; tüm eşikler 0.**
Kurum verisiyle bağımsız holdout olmadan bu sayılar genel üretim varsayılanı yapılmadı.
Yeni servis API'si, secret/TLS/timeout ayarları, hata politikası, Helm/Compose ve
konsol görünürlüğü hazır. Bu rapor release veya 100 TPS kabul duyurusu değildir.

## Deney ve kapsam

- Önceki aşamayla aynı 1.000 tool, 80 pozitif EN/TR intent ve 10 katalog dışı intent.
- Embedding: `sentence-transformers/paraphrase-multilingual-mpnet-base-v2`, legacy metin.
- Reranker: `BAAI/bge-reranker-v2-m3-int8`, FastEmbed **0.9.1**, CPU **4 thread**.
- [BGE model kartı](https://huggingface.co/BAAI/bge-reranker-v2-m3),
  [ONNX dönüşümü](https://huggingface.co/onnx-community/bge-reranker-v2-m3-ONNX).
  Test adapter'ı logit'e açıkça sigmoid uygular; skorlar kalibre olasılık değildir.
- Model ağırlıkları ürüne paketlenmez. Test servisleri loopback ve key'sizdir;
  metadata/intent'ler sentetiktir. Ağırlık SHA-256 ve boyutu pipeline JSON'undadır.
- Expected tool adına SHA256 uygulanarak EN/TR çifti aynı calibration/validation
  bölümünde tutulur. Negatif çeviri çiftleri de birlikte ayrılır. Calibration:
  20 EN + 20 TR + 6 negatif; validation: 20 EN + 20 TR + 4 negatif.
- Korpus önceki aşamada zaten bilinen geliştirme verisidir. Bölümleme onu bağımsız
  production holdout'a dönüştürmez. İkinci politika ilk deneyin başarısızlığı
  incelendikten sonra geliştirilmiştir; eşik seçici yalnızca calibration skorlarını
  kullanır, validation üzerinde seçim yapmaz. Bu, tekrarlı exploratory development'tır.

## Aday retrieval sınırı

| Aday penceresi | EN erişilen beklenen tool | TR erişilen beklenen tool |
|---|---:|---:|
| 5 | 39/40 | 36/40 |
| 10 | 39/40 | 38/40 |
| 20 | 39/40 | 38/40 |
| 40 | 39/40 | 38/40 |

Bu kümede 10 → 40 aday ek recall sağlamadı. Reranker aday havuzunda olmayan tool'u
getiremez. 40 aday içinde bile bulunamayan üç intent:

| Dil | Intent | Beklenen |
|---|---|---|
| tr | Siparişim şu an hangi aşamada? | `get_order` |
| tr | Veritabanının yedeğini al | `backup_database` |
| en | Give me the main points of this report | `summarize_document` |

Bu eksikler sonraki metadata/retrieval/model değerlendirmesinin konusu olmaya devam eder.

## İlk deney: doğrudan rerank ve tool başına eşik

40 aday bir kez skorlandı; aynı skorların ilk 10/20/40 alt pencereleri değerlendirildi.
Bu üç pencerenin toplam sonuçları aynı çıktı:

| Politika | Hit@1 | Hit@5 | Negatif FP |
|---|---:|---:|---:|
| Retrieval baseline | 62/80 | 75/80 | 10/10 |
| Doğrudan rerank, eşik 0 | 73/80 | 77/80 | 10/10 |

Toplam artışa rağmen calibration EN Hit@1 **18/20 → 17/20**, MRR **0.950 → 0.925**
oldu. Bu yüzden ilk katı promotion kapısı **geçmedi**; per-tool eşik seçilemedi.
Skorlar yükseldikçe top-N listeyi elemek no-match problemini tek başına çözmüyor;
doğru tool'ları da kaybettirebiliyor. Başarısız ilk deneyin çıktısı saklandı.

40 adayın CPU model servisindeki ayrı çağrı süreleri: p50 **2.700 ms**,
p95 **3.050 ms**, p99 **3.208 ms**. Bu ölçümde veri kaybını önlemek için provider
timeout 30s idi; ürün varsayılan timeout'u 5s olarak kaldı.

## İkinci deney: query-level gate ve belirsiz sıra

Üç karar birbirinden ayrıldı:

1. En yüksek rerank skoru `NO_MATCH_SCORE` altındaysa tüm sorgu için boş sonuç.
2. No-match sınırını geçen en yüksek skor `CONFIDENCE_SCORE` altında ise retrieval
   sırasını koru ve `-rerank-uncertain` döndür.
3. Diğer durumlarda rerank sırasını kullan. Ayrı `MIN_SCORE` tool bazlı filtrelemedir;
   bu deneyde 0 bırakıldı.

Calibration bölümünde sabit grid üzerinde seçilen, **bu modele özgü** değerler:

```env
VMCP_OMNI_RERANK_NO_MATCH_SCORE=0.0001
VMCP_OMNI_RERANK_CONFIDENCE_SCORE=0.1
VMCP_OMNI_RERANK_MIN_SCORE=0
VMCP_OMNI_RERANK_CANDIDATES=20
VMCP_OMNI_RERANK_TIMEOUT=5s
VMCP_OMNI_RERANK_FAILURE_POLICY=closed
```

Bunlar release varsayılanı veya başka modele kopyalanacak evrensel eşikler değildir.
Özellikle `0.0001`, embedding cosine eşiğiyle karşılaştırılamaz. Sayısal aralığın
[0,1] olması skorun doğruluk olasılığı olduğu anlamına gelmez.

## Gerçek ürün arama yolunda doğrulama

Seçilen politika yalnızca skor replay'iyle bırakılmadı. Index.Search, gerçek
embedding/rerank HTTP istemcileri, 20 aday, 5s timeout, request-scoped fresh ACL
callback ve tüm 90 intent tekrar çalıştırıldı. 90/90 pre-rerank ACL refresh yapıldı.
Callback bu kalite testinde in-memory'dir; gerçek HTTP/virtual auth senaryoları
ayrı race testlerinde kapsanır. Kontrollü MicroK8s yük testi değildir.

| Ölçüm | Baseline | Kalibre/guarded canlı pipeline |
|---|---:|---:|
| EN Hit@1 | 33/40 | 34/40 |
| TR Hit@1 | 29/40 | 31/40 |
| EN Hit@5 | 39/40 | 39/40 |
| TR Hit@5 | 36/40 | 36/40 |
| Toplam Hit@1 | 62/80 | **65/80** |
| Toplam Hit@5 | 75/80 | **75/80** |
| EN MRR@5 | 0.892 | 0.904 |
| TR MRR@5 | 0.798 | 0.827 |
| Katalog dışı FP | 10/10 | **0/10** |

Development-validation bölümü için her dilde Hit@1, Hit@5 ve MRR gerilememe ve
negatif FP artmama kapısı **geçti**. Sadece 4 validation negatif örnek bulunduğu
özellikle dikkate alınmalı; bu 0% production FPR garantisi değildir.

12 sorgu no-match döndü: 10 katalog dışı + **2 pozitif TR intent**. Bu iki pozitifin
beklenen tool'u 40 adayda dahi bulunamamıştı; baseline da doğru tool'u getiremiyordu.
Pozitif boş sonuçları gizlemek yerine ayrıca raporluyoruz. 34 sorgu uncertain sıra,
44 sorgu reranked sıra döndürdü. Eşikler kapalı varsayılanda bu no-match sonucu
kendiliğinden oluşmaz.

Canlı pipeline süreleri: p50 **1.421 ms**, p95 **1.644 ms**, p99 **1.709 ms**.
Bu süreler embedding + retrieval + 20-aday rerank + in-memory ACL callback içerir;
Kubernetes/gerçek DP auth ağı/invoke süresi içermez. Test sequential CPU çalışır;
100 semantic search TPS veya 100 invocation TPS kapasite kanıtı değildir.
Reranker maliyeti embedding'den ayrı planlanmalı ve gerçek ortamda ölçülmelidir.

## Güvenlik ve dayanıklılık doğrulamaları

- Reranker'a yalnızca fresh yetki listesindeki candidate metadata gider.
- Model çağrısından önce iptal edilen grant'te model hiç çağrılmaz; sonrasında
  iptal edilen grant'te sonuç filtrelenir. Auth outage'da provider'a gidilmez.
- Runtime credential modele gönderilmez; provider key boşsa Authorization yoktur.
- Self-signed TLS opt-in diğer istemcilerin transport'unu değiştirmez.
- Eksik/duplicate index, eksik/sonlu olmayan/aralık dışı skor ve bozuk JSON reddedilir.
- Timeout/cancel, closed/fallback, query gate ve uncertain sıra senaryoları test edilir.
- Server discovery'de büyük bir server aday havuzunu tek başına dolduramaz; server
  çeşitliliği korunur, server collapse rerank sonrasında yapılır.
- Model hataları reason/status/candidate/süre ile loglanır; raw metin/key/body yoktur.

## Testler ve işletim

Go race ve ilgili vet kontrolleri, **14 Helm** kontrolü, Compose config, Python
calibration testleri, web build ve **6 Omni tarayıcı** senaryosu başarılı. Dış
broker testlerinin tamamının yerelde çalıştığı iddiası yoktur; release CI ayrı kapıdır.
İki test servisi tamamlandıktan sonra kapatılır. Kullanıcının mevcut benchmark
çalışma dosyaları değiştirilmez.

[Ölçümler](omni-rerank-stage2-measurements-20261027.json) ·
[Calibration](omni-rerank-stage2-calibration-20261027.json) ·
[Canlı pipeline](omni-rerank-stage2-pipeline-20261027.json) ·
[Tekrarlama](../omni/README.md#optional-reranker-and-query-level-no-match-calibration) ·
[Kurulum/API/env](../../docs/Omni-MCP-Reranking.md)
