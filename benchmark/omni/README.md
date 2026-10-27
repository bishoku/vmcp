# Omni acceptance

Use the isolated MicroK8s runner from the repository root. It creates and removes
only a new `vmcp-load-*` namespace, leaving existing installations untouched.

```sh
GOCACHE=/tmp/vmcp-omni-go-cache \
VMCP_ACCEPTANCE_GOMEMLIMIT=4GiB VMCP_ACCEPTANCE_DIAGNOSTICS=1 \
VMCP_OMNI_ACCEPTANCE_SECONDS=600 \
python3 benchmark/kubernetes/run.py --omni --data-plane-cpus 2 \
  --data-plane-memory-limit 6Gi --output artifacts/benchmark-runs/omni-acceptance
```

Fixed inventory: 10 upstreams/vMCPs, 100 tools each (1,000 total), explicit
provider/consumer teams and approved grants. Half the upstreams are REST and half
native MCP, with synthetic responses, unique execution nonces and PII validation.
One DP uses the requested Go/container memory settings. Omni uses 2 CPU,
512MiB Go target and a 1GiB container limit; it has no DB/master credentials.

Three sequential phases each last 600 seconds:

1. Direct calls at 100 TPS, with Omni stopped and publication disabled.
2. Omni invocation at 100 TPS, with a dedicated key revoked at the midpoint.
3. Direct calls at 100 TPS plus an independent 100 searches/s through Omni.

100 TPS means individual MCP calls, not 100 four-step agent workflows/s. Invocation
traffic alternates REST/native (50 TPS each); this is not a maximum-capacity search.
Calls use the existing open-loop generator, never automatic retries. Invocation
gates are ≥99 successful TPS, ≥99.9% response success, p95≤250ms, p99≤1s and no
invalid results. The search interference generator is independently bounded;
search success, semantic/fallback mode and p95/p99 are reported. Search quota is
explicitly raised to 12,000/min for the synthetic key. Production default remains
120/min per credential per replica.

The test checks grant removal, unpublication, Omni outage/rebuild, DP outage
fail-closed behavior, audit actor/target/trace and duplicate execution. Key revoke
must yield 401 within 5s and remain rejected. The ten-second key probe completes
inside phases of at least 30s. `VMCP_OMNI_ACCEPTANCE_SECONDS=30` is a diagnostic
smoke run, not ten-minute acceptance.

Raw `omni-report.json`, per-five-second metrics, kubelet RSS/CPU/usage, Kubernetes
pod/events, image/source hashes and GC logs remain in the ignored output directory
and its verified local archive. Do not commit raw runtime data or credentials.
The curated report separates synthetic performance from production guarantees.

## Real semantic model and relevance

`embedding_fixture.py` is a test-only OpenAI-compatible server with no auth/TLS
or response cache. It runs a real quantized ONNX model using `fastembed==0.9.1`.
Never expose it publicly or send production metadata. Bind a dedicated test node
IP reachable from the test pods. Model files are downloaded on first use.

```sh
uv venv /tmp/vmcp-omni-embed-venv
uv pip install --python /tmp/vmcp-omni-embed-venv/bin/python fastembed==0.9.1
/tmp/vmcp-omni-embed-venv/bin/python benchmark/omni/embedding_fixture.py \
  --host <test-node-ip> --port 18092 \
  --model sentence-transformers/paraphrase-multilingual-mpnet-base-v2
```

For semantic load, set these on the Kubernetes runner:

```sh
export VMCP_OMNI_EMBEDDING_URL=http://<test-node-ip>:18092/v1/embeddings
export VMCP_OMNI_EMBEDDING_MODEL=sentence-transformers/paraphrase-multilingual-mpnet-base-v2
# Only for this test fixture: enables an intentional embedding outage/recovery.
export VMCP_TEST_EMBEDDING_CONTROL_URL=http://<test-node-ip>:18092
```

For relevance evaluation, from `core/`:

```sh
VMCP_TEST_EMBEDDING_URL="$VMCP_OMNI_EMBEDDING_URL" \
VMCP_TEST_EMBEDDING_MODEL="$VMCP_OMNI_EMBEDDING_MODEL" \
VMCP_TEST_SEMANTIC_REPORT=/tmp/omni-semantic.json \
go test -tags=acceptance -count=1 -v -run TestRealModelSemanticAcceptance ./internal/omni
```

`intents.tsv` contains 30 English tool descriptions and 60 labeled paraphrases
(30 EN, 30 TR); another 970 synthetic operational statistics tools distract the
search. The check requires recall@5≥90% for each language. This is a development
smoke corpus, including examples inspected while correcting lexical matching;
it is **not a held-out institution-specific relevance benchmark**. Validate your
own metadata and model with a separate held-out corpus before making quality SLAs.
Models are operator-selected; the product has no bundled model or default provider.
See [FastEmbed supported models](https://qdrant.github.io/fastembed/examples/Supported_Models/).

## Governance changes under load

Run the same command with `VMCP_OMNI_FUNCTIONAL_ONLY=1` for a separate 30-second,
100 TPS fault exercise. Traffic uses other vMCPs while an owned upstream tool grant
and a vMCP's Omni publication are removed. Each must become inaccessible within
5 seconds, stay undiscoverable/uninvocable, and leave direct access to the
unpublished vMCP intact. Fault traffic allows the known global-revision guard
rejections (at least 99% success); this is not a steady-state latency benchmark.
Use a new output directory. The normal ten-minute phases remain unchanged.

For the separate 16/32 provider concurrency diagnostic, run
`/tmp/vmcp-omni-embed-venv/bin/python benchmark/omni/provider_diagnostic.py --output /tmp/omni-provider.json`.
It runs two 60-second 100 RPS windows directly against local model fixtures.
This diagnostic does not replace the ten-minute gateway acceptance. The fixture's
`--concurrency` setting is independent of Omni's request bound and search quota.

## Discovery quality comparison

`quality-cases.json` freezes 40 tool definitions, 80 labeled EN/TR intents
(including near-neighbor actions) and 10 out-of-catalog intents. Another 960
synthetic statistics tools bring the catalog to 1,000. The corpus was authored
before the ranking comparison but is development data, not an independently
held-out production evaluation. Original smoke queries overlap the previous corpus.

```sh
GOCACHE=/tmp/vmcp-omni-go-cache \
VMCP_TEST_EMBEDDING_URL=http://127.0.0.1:18094/v1/embeddings \
VMCP_TEST_EMBEDDING_MODEL=sentence-transformers/paraphrase-multilingual-mpnet-base-v2 \
VMCP_TEST_QUALITY_REPORT=/tmp/omni-quality.json \
go -C core test -tags=acceptance -count=1 -v -run TestDiscoveryQualityComparison ./internal/omni
```

The comparison uses the same real model for frozen legacy text/scoring, the
current default pipeline, structured
text with field-weighted lexical/cosine scoring, and structured text with bounded
RRF candidates. Embeddings are cached inside this evaluation process only, so
repeated variants use identical vectors. This cache is not a product search cache
and timings from this test are not performance acceptance. Outcomes include
Hit@1, Hit@5, reciprocal rank and negative-query false positives, including failed
cases rather than filtering them out. Existing SDK/ACL/revocation and race tests
remain required independently of semantic quality.

The default pipeline has non-regression gates for each language's Hit@1, Hit@5 and
reciprocal-rank sum and for negative-query false positives. Experimental variants
are reported even if worse; they are not promoted simply because the test ran.
A fixed 0.35-floor experiment measures the recall/false-positive tradeoff and does
not establish a universal release default. The original acceptance remains a
separate legacy smoke check.

## Optional reranker and query-level no-match calibration

The test-only `rerank_fixture.py` exposes the product's rerank API contract using
FastEmbed 0.9.1 and `BAAI/bge-reranker-v2-m3-int8`. It downloads ONNX weights to
`/tmp/vmcp-omni-reranker-model`; no model is bundled with the product. BGE returns
logits; this fixture explicitly applies sigmoid to obtain [0,1] scores. Those
scores are not calibrated probabilities. Sources:
[original BGE model](https://huggingface.co/BAAI/bge-reranker-v2-m3),
[ONNX conversion](https://huggingface.co/onnx-community/bge-reranker-v2-m3-ONNX).
Bind both test services to loopback; use only synthetic metadata.

```sh
HF_HUB_DISABLE_TELEMETRY=1 /tmp/vmcp-omni-embed-venv/bin/python \
  benchmark/omni/rerank_fixture.py --host 127.0.0.1 --port 18095
```

Keep the previous embedding fixture running on 18094. Capture 40 candidates/query
once, then compare 10/20/40 windows and an initial strict per-tool threshold grid:

```sh
GOCACHE=/tmp/vmcp-omni-go-cache \
VMCP_TEST_EMBEDDING_URL=http://127.0.0.1:18094/v1/embeddings \
VMCP_TEST_EMBEDDING_MODEL=sentence-transformers/paraphrase-multilingual-mpnet-base-v2 \
VMCP_TEST_RERANK_URL=http://127.0.0.1:18095/v1/rerank \
VMCP_TEST_RERANK_MODEL=BAAI/bge-reranker-v2-m3-int8 \
VMCP_TEST_RERANK_REPORT=/tmp/omni-rerank-measurements.json \
go -C core test -tags=acceptance -count=1 -v -run '^TestRerankQualityCalibration$' ./internal/omni
```

This measurement uses a 30-second provider timeout to retain latency evidence;
the product default remains 5 seconds. The split hashes each expected tool name,
keeping EN/TR paraphrases together; negative translation pairs also stay together.
The corpus is known synthetic development data, not independent production holdout.
The initial strict calibration can fail promotion while the measurement itself
succeeds; inspect `calibration_candidate_found` and `validation_non_regression_gate`.

A second exploratory policy selects query-level no-match and uncertain-order
guards from the recorded calibration partition only. Validation results do not
select thresholds; they are reported separately. This is a subsequent experiment
on the same known corpus, not a new unseen-data claim:

```sh
python3 benchmark/omni/calibrate_rerank.py /tmp/omni-rerank-measurements.json \
  --output /tmp/omni-rerank-calibration.json
python3 -m unittest discover -s benchmark/omni -p 'test_*.py'
```

Validate the selected policy in the real product search path with **20 candidates,
5-second timeout and closed failure policy**, rather than assuming replay is enough:

```sh
GOCACHE=/tmp/vmcp-omni-go-cache \
VMCP_TEST_EMBEDDING_URL=http://127.0.0.1:18094/v1/embeddings \
VMCP_TEST_EMBEDDING_MODEL=sentence-transformers/paraphrase-multilingual-mpnet-base-v2 \
VMCP_TEST_RERANK_URL=http://127.0.0.1:18095/v1/rerank \
VMCP_TEST_RERANK_MODEL=BAAI/bge-reranker-v2-m3-int8 \
VMCP_TEST_RERANK_CALIBRATION=/tmp/omni-rerank-calibration.json \
VMCP_TEST_RERANK_PIPELINE_REPORT=/tmp/omni-rerank-pipeline.json \
go -C core test -tags=acceptance -count=1 -v -run '^TestRerankCalibratedPipeline$' ./internal/omni
```

This last test exercises Index.Search, real embedding/rerank HTTP calls, the
request-scoped fresh-ACL callback, configured policies and final ordering. Its
ACL callback is in memory; full HTTP/virtual authorization/revocation is covered
separately by Go race tests. It is not a Kubernetes 100-TPS load test. Inspect
positive-query empty results as well as Hit@1/5, negative false positives and
latency. Do not copy the resulting numeric thresholds to another model/version.
