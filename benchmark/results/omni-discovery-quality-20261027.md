# Omni discovery quality — 2026-10-27

## Sonuç ve karar

Model/indeks tutarlılığı, vector kapsama görünürlüğü, query/document prefix ve
kalibre edilebilir similarity eşiği eklendi. Yeni metin/sıralama adayları ölçüldü;
**hiçbiri dil bazında tutarlı kalite artışı göstermediği için varsayılan yapılmadı**.
Üretim varsayılanı legacy metin ve mevcut lexical/cosine birleşimini korur.
`tool-first-v1` metni ve similarity eşiği opt-in; RRF ve alan ağırlıklı lexical
sıralama yalnızca karşılaştırma kodundan deneniyor. Bu rapor bir release duyurusu değildir.

## Yöntem

- Aynı yerel gerçek model: `sentence-transformers/paraphrase-multilingual-mpnet-base-v2`.
- 40 hedef tool, 960 sentetik distractor; toplam 1.000 kayıt.
- 80 etiketli EN/TR intent (40/dil), 10 katalog dışı intent.
- İade iptali/sipariş iptali, toplantı iptali/erteleme, fatura üretimi/önizleme gibi
  yakın ama farklı yetenekler kapsanıyor.
- Korpus ilk sıralama karşılaştırmasından önce sabitlendi. İlk 60 intent önceki
  smoke kümesiyle örtüşüyor. Küme geliştirme verisidir; bağımsız holdout veya kurum
  üretim kalitesi kanıtı değildir. Aynı kümede aday seçimi yapıldığı açıkça kabul edilir.
- Variantlar aynı embedding vektörlerini test-içi cache üzerinden kullanır.
  Bu cache ürün davranışı değildir; bu testin süreleri TPS/latency kabulü değildir.
- `baseline`, frozen legacy text/scorer; `default-current`, güncel varsayılan
  indeks/arama akışı. Default için her dilde Hit@1, Hit@5 ve MRR gerilememe;
  negatif intent false-positive artmama kapıları testte enforce edilir.

## Karşılaştırma

Hit@5: beklenen tek tool ilk beşte. MRR@5: bulunmayan tool 0 puan alır.
Negatif FP: hiçbir tool gerektirmeyen intent'e en az bir tool dönmesi.

| Variant | Hit@1 | Hit@5 | MRR@5 | Negatif FP |
|---|---:|---:|---:|---:|
| baseline | 62/80 | 75/80 | 0.845 | 10/10 |
| default-current | 62/80 | 75/80 | 0.845 | 10/10 |
| structured-rrf | 54/80 | 72/80 | 0.760 | 10/10 |
| structured-simple | 61/80 | 75/80 | 0.831 | 10/10 |
| structured-simple-floor035 | 58/80 | 68/80 | 0.777 | 0/10 |
| structured-weighted | 62/80 | 75/80 | 0.839 | 10/10 |

Dil bazında baseline ve güncel varsayılan aynı: EN Hit@1 **33/40**, Hit@5 **39/40**;
TR Hit@1 **29/40**, Hit@5 **36/40**. Dolayısıyla bu aşamada yeni bir relevance artışı
iddiası yoktur. `structured-weighted` toplamları benzer görünse de dil bazında TR
ilk sıra düşer. RRF İngilizce ilk sırayı belirgin geriletir.

Sabit **0.35** eşiği yalnızca structured-simple varyantında ölçüldü. Negatif
false-positive 10/10'dan 0/10'a düşerken Hit@5 **75/80 baseline'dan 68/80**'e iner
(eşiksiz structured-simple: 75 değil, 74/80). Eşiğin kendi etkisini aynı metinde
karşılaştırınca 74/80 → 68/80 görülür. Bu bir model-geneli öneri değildir. Yeni
varsayılan eşik **0**, yani kapalıdır; keyword/fallback aramasına cosine eşiği
uygulanamaz. Katalog dışı intent problemi varsayılanda hâlâ açıktır.

## Bu aşamada doğrulanan davranış iyileştirmeleri

- Profil endpoint, model, revizyon, prefix, beklenen boyut ve metin formatını
  fingerprint eder; farklı profile ait vector yeniden kullanılmaz.
- İlk geçerli batch boyutu belirlenir; sonraki batch ve sorgular aynı boyutta
  olmalıdır. Sıfır/nonfinite/yanlış boyutlu vector anlamlı sonuç gibi kabul edilmez.
- Eksik veya uyumsuz vector bulunan yetkili aday kümesi tek bir lexical
  `hybrid-degraded` yoluna düşer. Eksik vectore yüksek lexical skor verip başka bir
  kaydın hibrit skoruyla karıştırma hatası kaldırıldı.
- Yetkisiz tool'un eksik vectorü, yetkili adayların arama modunu bozamaz.
- Durum/metric/console vector kapsaması ve boyutu gösterir.
- API key boşken Authorization gönderilmeme, prefix ayrımı, boyut assertion'ının
  sağlayıcıya `dimensions` parametresi eklememesi, model değişimi ve audit/ACL
  regresyonları test edilir.

## Sonraki kalite adımı

1. Kurumdan etiketlenmiş, bağımsız bir TR/EN holdout intent kümesi oluşturmak.
2. Başarısız intent'leri tool açıklaması/yanlış aday/model biçimi olarak sınıflamak.
3. Küçük aday kümesinde opsiyonel reranker ve model bazında no-match kalibrasyonu
   denemek; dil bazında kalite/latency kapılarını geçmeden varsayılan yapmamak.
4. Çoklu chunk/view kullanımını yalnızca uzun/çok yönlü tool'larda ayrı deneyle
   ölçmek. Bu aşamada ek model servisi veya vector database zorunluluğu getirilmedi.

[Ham sonuçlar](omni-discovery-quality-20261027.json) ·
[Korpus](../omni/quality-cases.json) ·
[Tekrarlama](../omni/README.md#discovery-quality-comparison) ·
[Model yapılandırması](../../docs/Omni-MCP-Embedding.md)

## Regresyon ve kısa maliyet kontrolü

- `go -C core test -race ./...`: başarılı. Bu komutun başarısı, konfigüre edilmemiş
  her dış broker entegrasyonunun çalıştığı anlamına gelmez; release CI ayrı kapıdır.
- İlgili Omni/virtual/cmd paketlerinde `go vet`: başarılı.
- Önceki gerçek-model smoke testi yeniden çalıştırıldı: EN **29/30**, TR **28/30**
  Hit@5; başlangıç baseline ile aynı.
- Helm: **13/13** render/validation kontrolü başarılı; profile/prefix değerleri ve
  geçersiz dimension sınırı kapsanır.
- Web build, OpenAPI 73 operasyon/6 örnek kontrolü, Omni **4/4** mobil/masaüstü
  tarayıcı testi başarılı. Compose config ve diff whitespace kontrolü temiz.
- Gerçek-provider test servisi sadece loopback'te, sentetik metadata ile çalıştırıldı.

Son lexical-only microbenchmark (Xeon Silver 4216, Go benchmark parallelism label 64):

| Yetkili tool | Süre/query | Allocation/query |
|---|---:|---:|
| 1.000 | 0,153 ms | 2.840 byte / 18 allocation |
| 6.000 | 0,959 ms | 2.840 byte / 18 allocation |
| 60.000 | 14,075 ms | 2.840 byte / 18 allocation |

İndeks zaten hazırdır; network, embedding, authorization fetch ve invoke dahil
**değildir**. Bu bir 100 TPS end-to-end ya da 10 dakikalık kabul tekrarı değildir;
eski sürüme karşı kontrollü kapasite artışı iddiası da değildir.
