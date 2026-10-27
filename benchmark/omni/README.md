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
