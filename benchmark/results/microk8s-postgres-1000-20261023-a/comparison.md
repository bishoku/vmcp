# PostgreSQL odaklı MicroK8s koşusu

300 saniye, hedef 1000 TPS; başarılı 312.1 TPS; p95 4633.3 ms; hata 24229, gönderilemeyen 181551.

PostgreSQL CPU/bellek limitsiz, üç yeni delivery indeksi var. Data-plane başına GOMAXPROCS=2. Süresi dolmuş 100.000 operation/stream fixture’ı temizliği çalıştırır. Fault ve trafik altında config mutasyonu kapsam dışıdır. Önceki 42 dakikalık koşuyla süre/yük/bakım kuyruğu farklıdır; doğrudan Kubernetes hızlanması iddiası değildir.

[Ayrıntılı rapor](report.md), [SQL örnekleri — yerel arşiv](../README.md#raw-artifacts), [sorgu sayaçları — yerel arşiv](../README.md#raw-artifacts).


Ham örnekler ve loglar Git dışında tutulur. Arşiv kimliği ve SHA-256: [reproduction.json](reproduction.json). Bkz. [arşiv politikası](../README.md#raw-artifacts).
