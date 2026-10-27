# 6.000 tool kapasite taraması

Data-plane: 2; routing: pod; arama sonucu: {'passing_rps': 2200, 'failing_rps': 2300, 'upper_bound_not_found': False}

| Aşama | Süre s | Hedef TPS | Başarılı TPS | p95 / p99 ms | Hata / gönderilemeyen | SLO |
|---|---:|---:|---:|---:|---:|---|
| warmup | 15 | 50 | 50.0 | 21.1 / 28.4 | 0 / 0 | True |
| capacity-500 | 60 | 500 | 499.6 | 35.8 / 44.9 | 0 / 5 | True |
| capacity-1000 | 60 | 1000 | 999.3 | 57.6 / 79.3 | 0 / 0 | True |
| capacity-1500 | 60 | 1500 | 1498.9 | 71.5 / 96.5 | 0 / 0 | True |
| capacity-2000 | 60 | 2000 | 1998.5 | 98.2 / 120.6 | 0 / 0 | True |
| capacity-3000 | 60 | 3000 | 2367.7 | 475.0 / 535.8 | 31976 / 5314 | False |
| recovery | 30 | 100 | 99.9 | 39.3 / 51.1 | 0 / 0 | True |
| refine-2500 | 60 | 2500 | 2393.6 | 438.6 / 467.3 | 4758 / 1021 | False |
| recovery | 30 | 100 | 99.9 | 41.7 / 56.5 | 0 / 0 | True |
| refine-2200 | 60 | 2200 | 2195.6 | 123.5 / 148.4 | 0 / 115 | True |
| recovery | 30 | 100 | 99.9 | 40.2 / 50.5 | 0 / 0 | True |
| refine-2300 | 60 | 2300 | 2287.4 | 277.1 / 483.2 | 522 / 89 | False |
| recovery | 30 | 100 | 99.9 | 39.6 / 49.6 | 0 / 0 | True |
| steady | 600 | 2200 | 2189.7 | 374.2 / 523.4 | 4994 / 943 | False |

Tarama aşamaları kısa süreli sınır aramasıdır; sürdürülebilir kapasite için steady doğrulaması esas alınmalıdır. [Ayrıntılı rapor](report.md).


Ham örnekler ve loglar Git dışında tutulur. Arşiv kimliği ve SHA-256: [reproduction.json](reproduction.json). Bkz. [arşiv politikası](../README.md#raw-artifacts).
