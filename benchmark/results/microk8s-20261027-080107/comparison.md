# PostgreSQL odaklı MicroK8s koşusu

600 saniye, hedef 2000 TPS; başarılı 1999.7 TPS; p95 104.2 ms; hata 0, gönderilemeyen 85.

PostgreSQL CPU/bellek limitsiz, üç yeni delivery indeksi var. Data-plane başına GOMAXPROCS=8. Süresi dolmuş 100.000 operation/stream fixture’ı temizliği çalıştırır. Fault ve trafik altında config mutasyonu kapsam dışıdır. Önceki 42 dakikalık koşuyla süre/yük/bakım kuyruğu farklıdır; doğrudan Kubernetes hızlanması iddiası değildir.

[Ayrıntılı rapor](report.md), [SQL örnekleri — yerel arşiv](../README.md#raw-artifacts), [sorgu sayaçları — yerel arşiv](../README.md#raw-artifacts).


Ham örnekler ve loglar Git dışında tutulur. Arşiv kimliği ve SHA-256: [reproduction.json](reproduction.json). Bkz. [arşiv politikası](../README.md#raw-artifacts).
