# Historical performance summaries

Aggregate phase measurements retained for comparison. Full reports, environment manifests, logs and diagnostics are local-only. Missing verdicts are not passes. These different workloads and environments are not interchangeable capacity claims. See [issue 31 acceptance](../../maintainer/ISSUE31_ACCEPTANCE.md) for the selected profile and limitations.

## acceptance-full/report.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.013 | 17.642 | 19.05 | 0 | 0 | pass |
| steady | 200 | 199.985 | 17.782 | 20.387 | 0 | 0 | pass |
| spike | 600 | 599.233 | 26.668 | 33.366 | 0 | 0 | pass |
| recovery | 200 | 199.963 | 17.653 | 19.939 | 0 | 0 | pass |
| node-loss | 200 | 199.885 | 18.005 | 20.217 | 8 | 0 | pass |
| node-rejoin | 200 | 199.968 | 18.138 | 21.28 | 0 | 0 | pass |
| soak | 100 | 99.998 | 18.595 | 20.097 | 0 | 0 | pass |

## acceptance-smoke/report.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 10 | 10.397 | 31.2 | 34.83 | 0 | 0 | pass |
| steady | 20 | 20.213 | 23.2 | 24.891 | 0 | 0 | pass |
| spike | 60 | 60.038 | 17.527 | 19.195 | 0 | 0 | pass |
| recovery | 20 | 20.278 | 22.505 | 25.503 | 0 | 0 | pass |
| node-loss | 20 | 19.96 | 24.204 | 26.912 | 1 | 0 | pass |
| node-rejoin | 20 | 20.137 | 24.539 | 30.713 | 0 | 0 | pass |
| soak | 10 | 10.155 | 26.165 | 29.352 | 0 | 0 | pass |

## acceptance-smoke-race/report.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 10 | 10.386 | 29.921 | 32.258 | 0 | 0 | pass |
| steady | 20 | 20.157 | 24.829 | 27.2 | 0 | 0 | pass |
| spike | 60 | 59.937 | 20.36 | 21.726 | 0 | 0 | pass |
| recovery | 20 | 20.298 | 23.643 | 24.574 | 0 | 0 | pass |
| node-loss | 20 | 19.953 | 24.207 | 27.375 | 1 | 0 | pass |
| node-rejoin | 20 | 20.101 | 25.16 | 28.958 | 0 | 0 | pass |
| soak | 10 | 10.143 | 27.056 | 29.354 | 0 | 0 | pass |

## microk8s-20261023-192851/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 10 | 10.407 | 30.352 | 33.385 | 0 | 0 | pass |
| steady | 20 | 20.21 | 26.128 | 32.133 | 0 | 0 | pass |
| spike | 60 | 59.907 | 19.843 | 23.082 | 0 | 0 | pass |
| recovery | 20 | 20.322 | 21.184 | 29.828 | 0 | 0 | pass |
| node-loss | 20 | 19.958 | 21.197 | 42.564 | 1 | 0 | pass |
| node-rejoin | 20 | 20.118 | 22.457 | 26.914 | 0 | 0 | pass |
| soak | 10 | 10.165 | 21.959 | 28.596 | 0 | 0 | pass |

## microk8s-20261023-193023/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.941 | 69.052 | 98.544 | 0 | 0 | pass |
| baseline | 200 | 199.115 | 143.743 | 360.834 | 0 | 0 | pass |
| steady | 600 | 191.238 | 3280.455 | 3629.124 | 2008 | 120232 | fail |
| spike | 2000 | 184.019 | 5091.667 | 5520.999 | 649 | 107964 | fail |
| recovery | 600 | 191.437 | 3312.879 | 3675.626 | 278 | 17633 | fail |
| config-refresh | 600 | 190.067 | 4542.361 | 4891.252 | 351 | 11562 | fail |
| node-loss | 600 | 44.911 | 431.122 | 682.7 | 49955 | 3 | fail |
| node-rejoin | 600 | 597.102 | 631.376 | 842.105 | 1 | 59 | pass |
| soak | 400 | 172.081 | 5583.794 | 5981.083 | 20197 | 389676 | fail |

## microk8s-20261026-215339/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.73 | 93.753 | 106.382 | 0 | 0 | pass |
| steady | 1000 | 999.802 | 131.646 | 168.751 | 1 | 33 | pass |

## microk8s-20261026-223317/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.87 | 63.717 | 88.875 | 1 | 0 | pass |
| steady | 1000 | 999.085 | 129.916 | 421.036 | 1 | 442 | pass |

## microk8s-20261026-224955/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.01 | 83.281 | 98.777 | 0 | 0 | pass |
| steady | 1000 | 989.858 | 122.996 | 146.625 | 5993 | 33 | fail |

## microk8s-20261027-054711/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.836 | 88.464 | 105.539 | 0 | 0 | pass |
| steady | 1000 | 999.755 | 123.113 | 147.568 | 2 | 59 | pass |

## microk8s-20261027-060146/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.991 | 22.411 | 31.938 | 0 | 0 | pass |
| steady | 1000 | 991.612 | 46.097 | 66.36 | 1 | 18 | pass |

## microk8s-20261027-070807/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.994 | 20.124 | 29.95 | 0 | 0 | pass |
| capacity-500 | 500 | 499.655 | 37.939 | 46.518 | 0 | 0 | pass |
| capacity-1000 | 1000 | 999.138 | 79.354 | 102.885 | 0 | 11 | pass |
| capacity-1500 | 1500 | 1318.48 | 778.641 | 802.469 | 10101 | 98 | fail |
| recovery | 100 | 99.929 | 45.639 | 53.926 | 0 | 0 | pass |
| refine-1200 | 1200 | 1198.908 | 110.28 | 134.856 | 0 | 0 | pass |
| recovery | 100 | 99.936 | 47.252 | 56.902 | 0 | 0 | pass |
| refine-1300 | 1300 | 1298.636 | 159.28 | 186.217 | 0 | 0 | pass |
| recovery | 100 | 99.925 | 45.436 | 55.097 | 0 | 0 | pass |
| steady | 1300 | 1299.149 | 644.658 | 701.38 | 372 | 54 | fail |

## microk8s-20261027-072708/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.019 | 19.632 | 29.073 | 0 | 0 | pass |
| steady | 1200 | 1199.809 | 108.877 | 152.791 | 0 | 50 | pass |

## microk8s-20261027-073909/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.007 | 21.05 | 28.353 | 0 | 0 | pass |
| capacity-500 | 500 | 499.646 | 35.81 | 44.936 | 0 | 5 | pass |
| capacity-1000 | 1000 | 999.312 | 57.56 | 79.286 | 0 | 0 | pass |
| capacity-1500 | 1500 | 1498.917 | 71.457 | 96.527 | 0 | 0 | pass |
| capacity-2000 | 2000 | 1998.537 | 98.184 | 120.606 | 0 | 0 | pass |
| capacity-3000 | 3000 | 2367.712 | 475.013 | 535.842 | 31976 | 5314 | fail |
| recovery | 100 | 99.931 | 39.257 | 51.069 | 0 | 0 | pass |
| refine-2500 | 2500 | 2393.648 | 438.566 | 467.319 | 4758 | 1021 | fail |
| recovery | 100 | 99.934 | 41.694 | 56.496 | 0 | 0 | pass |
| refine-2200 | 2200 | 2195.561 | 123.49 | 148.417 | 0 | 115 | pass |
| recovery | 100 | 99.949 | 40.169 | 50.516 | 0 | 0 | pass |
| refine-2300 | 2300 | 2287.406 | 277.105 | 483.202 | 522 | 89 | fail |
| recovery | 100 | 99.935 | 39.617 | 49.631 | 0 | 0 | pass |
| steady | 2200 | 2189.715 | 374.228 | 523.38 | 4994 | 943 | fail |

## microk8s-20261027-080107/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.009 | 19.999 | 29.023 | 0 | 0 | pass |
| steady | 2000 | 1999.695 | 104.173 | 143.363 | 0 | 85 | pass |

## microk8s-20261027-081515/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.009 | 19.206 | 26.406 | 0 | 0 | pass |
| steady | 1200 | 1199.81 | 109.365 | 155.791 | 0 | 41 | pass |

## microk8s-20261027-082711/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.009 | 19.137 | 23.767 | 0 | 0 | pass |
| steady | 1200 | 1199.875 | 107.596 | 143.204 | 0 | 13 | pass |

## microk8s-20261027-083911/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.998 | 19.884 | 29.836 | 0 | 0 | pass |
| steady | 1200 | 1199.87 | 108.837 | 152.636 | 1 | 10 | pass |

## microk8s-20261027-085716/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.009 | 19.284 | 26.298 | 0 | 0 | pass |
| steady | 1000 | 999.904 | 81.183 | 106.616 | 0 | 12 | pass |

## microk8s-20261027-092020/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.008 | 19.915 | 27.919 | 0 | 0 | pass |
| steady | 1000 | 999.849 | 81.291 | 107.818 | 30 | 18 | pass |

## microk8s-20261030-111358/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.003 | 19.227 | 29.851 | 0 | 0 | pass |
| steady | 500 | 499.939 | 52.926 | 69.593 | 0 | 0 | pass |

## microk8s-20261030-112917/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.009 | 20.135 | 27.678 | 0 | 0 | pass |
| steady | 500 | 499.938 | 58.736 | 79.158 | 0 | 2 | pass |

## microk8s-20261105-193915/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 10 | 10.401 | 36.63 | 37.317 | 0 | 0 | pass |
| steady | 20 | 20.199 | 22.476 | 24.22 | 0 | 0 | pass |
| spike | 60 | 59.979 | 34.856 | 48.384 | 0 | 0 | pass |
| recovery | 20 | 20.308 | 23.159 | 33.199 | 0 | 0 | pass |
| node-loss | 20 | 19.96 | 23.006 | 46.522 | 1 | 0 | pass |
| node-rejoin | 20 | 20.119 | 23.849 | 34.243 | 0 | 0 | pass |
| soak | 10 | 10.167 | 23.892 | 26.406 | 0 | 0 | pass |

## microk8s-20261105-195505/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.007 | 21.112 | 31.251 | 0 | 0 | pass |
| steady | 1000 | 690.165 | 1412.844 | 1449.934 | 185131 | 125 | fail |

## microk8s-20261105-200832/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 50.01 | 21.463 | 28.765 | 0 | 0 | pass |
| steady | 1000 | 733.118 | 1414.396 | 1457.433 | 159158 | 204 | fail |

## microk8s-20261105-202806/summary.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 16.671 | 23.382 | 34.434 | 500 | 0 | pass |
| steady | 1000 | 999.598 | 168.838 | 366.135 | 164 | 8 | pass |

## microk8s-polling-1000-20261023-c/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.864 | 90.478 | 107.573 | 0 | 0 | pass |
| steady | 1000 | 933.4 | 161.996 | 1046.361 | 9311 | 10633 | fail |

## microk8s-polling-1000-20261023-d/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.879 | 85.467 | 106.188 | 0 | 0 | pass |
| steady | 1000 | 999.829 | 128.933 | 196.259 | 5 | 7 | pass |

## microk8s-polling-1000-20261023-e/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.97 | 89.378 | 100.722 | 0 | 0 | pass |
| steady | 1000 | 998.658 | 135.741 | 485.412 | 3 | 358 | pass |

## microk8s-postgres-1000-20261023-a/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.949 | 67.778 | 89.676 | 0 | 0 | pass |
| steady | 1000 | 312.077 | 4633.345 | 5060.663 | 24229 | 181551 | fail |

## microk8s-postgres-1000-20261023-b/summary.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.82 | 85.889 | 98.736 | 0 | 0 | pass |
| steady | 1000 | 306.645 | 4394.074 | 4716.585 | 19938 | 187553 | fail |

## rest-release-20261020/after-fix/report.json

Recorded overall verdict: pass.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.946 | 52.117 | 63.295 | 0 | 0 | pass |
| steady | 200 | 199.942 | 45.622 | 51.268 | 0 | 0 | pass |
| spike | 600 | 584.689 | 193.108 | 356.816 | 0 | 285 | pass |
| recovery | 200 | 199.843 | 45.816 | 50.969 | 0 | 0 | pass |
| config-refresh | 200 | 199.552 | 78.16 | 211.117 | 3 | 0 | pass |
| node-loss | 200 | 199.846 | 47.914 | 53.645 | 10 | 0 | pass |
| node-rejoin | 200 | 199.966 | 46.836 | 52.412 | 0 | 0 | pass |
| soak | 100 | 99.997 | 48.382 | 55.639 | 0 | 0 | pass |

## rest-release-20261020/before-fix/report.json

Recorded overall verdict: fail.

| Phase | Target/s | Completed/s | p95 ms | p99 ms | Errors | Dropped | Gate |
|---|---:|---:|---:|---:|---:|---:|---|
| warmup | 50 | 49.986 | 51.69 | 63.672 | 0 | 0 | pass |
| steady | 200 | 199.971 | 45.995 | 52.175 | 0 | 0 | pass |
| spike | 600 | 599.158 | 61.958 | 71.371 | 0 | 0 | pass |
| recovery | 200 | 199.837 | 45.907 | 51.029 | 0 | 0 | pass |
| config-refresh | 200 | 197.82 | 193.753 | 244.842 | 0 | 0 | pass |
| node-loss | 200 | 4.933 | 70.328 | 84.552 | 17556 | 0 | fail |
| node-rejoin | 200 | 99.969 | 45.359 | 51.293 | 6000 | 0 | fail |
| soak | 100 | 49.993 | 163.521 | 554.129 | 15000 | 0 | fail |

