# 6.000 tool kapasite taraması

Data-plane: 1; routing: pod; arama sonucu: {'passing_rps': 1300, 'failing_rps': 1500, 'upper_bound_not_found': False}

| Aşama | Süre s | Hedef TPS | Başarılı TPS | p95 / p99 ms | Hata / gönderilemeyen | SLO |
|---|---:|---:|---:|---:|---:|---|
| warmup | 15 | 50 | 50.0 | 20.1 / 30.0 | 0 / 0 | True |
| capacity-500 | 60 | 500 | 499.7 | 37.9 / 46.5 | 0 / 0 | True |
| capacity-1000 | 60 | 1000 | 999.1 | 79.4 / 102.9 | 0 / 11 | True |
| capacity-1500 | 60 | 1500 | 1318.5 | 778.6 / 802.5 | 10101 / 98 | False |
| recovery | 30 | 100 | 99.9 | 45.6 / 53.9 | 0 / 0 | True |
| refine-1200 | 60 | 1200 | 1198.9 | 110.3 / 134.9 | 0 / 0 | True |
| recovery | 30 | 100 | 99.9 | 47.3 / 56.9 | 0 / 0 | True |
| refine-1300 | 60 | 1300 | 1298.6 | 159.3 / 186.2 | 0 / 0 | True |
| recovery | 30 | 100 | 99.9 | 45.4 / 55.1 | 0 / 0 | True |
| steady | 600 | 1300 | 1299.1 | 644.7 / 701.4 | 372 / 54 | False |

Tarama aşamaları kısa süreli sınır aramasıdır; sürdürülebilir kapasite için steady doğrulaması esas alınmalıdır. [Ayrıntılı rapor](report.md).


Ham örnekler ve loglar Git dışında tutulur. Arşiv kimliği ve SHA-256: [reproduction.json](reproduction.json). Bkz. [arşiv politikası](../README.md#raw-artifacts).
