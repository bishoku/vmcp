"""Package the release chart's production profile for kubectl and Argo CD."""
import argparse
import hashlib
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

import yaml


def write_kustomization(path, resources, wave=None):
    spec = {
        "apiVersion": "kustomize.config.k8s.io/v1beta1",
        "kind": "Kustomization",
        "resources": resources,
    }
    if wave is not None:
        spec["commonAnnotations"] = {"argocd.argoproj.io/sync-wave": str(wave)}
    path.mkdir(parents=True)
    (path / "kustomization.yaml").write_text(yaml.safe_dump(spec, sort_keys=False))


def package(chart, values, version, output):
    if not re.fullmatch(r"v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?", version):
        raise ValueError("expected an application version tag")
    rendered = subprocess.check_output(
        ["helm", "template", "vmcp", chart, "--namespace", "vmcp",
         "--set", "fullnameOverride=vmcp", "--skip-tests",
         *[arg for path in values for arg in ("-f", path)]], text=True)
    phases = {"prerequisites": [], "migration": [], "app": []}
    for obj in yaml.safe_load_all(rendered):
        if obj is None:
            continue
        kind = obj["kind"]
        if kind == "Secret":
            raise ValueError("release profile must use existing Secrets")
        if kind == "Job":
            if obj["metadata"]["labels"]["app.kubernetes.io/component"] != "migration":
                raise ValueError("unexpected Job in release profile")
            # Keep the Job for reconciliation. A new tag gets a new immutable Job.
            suffix = re.sub(r"[^a-z0-9-]", "-", version.lower())[:31].rstrip("-")
            digest = hashlib.sha256(version.encode()).hexdigest()[:10]
            obj["metadata"]["name"] = f"vmcp-migrate-{suffix}-{digest}"
            obj["spec"].pop("ttlSecondsAfterFinished", None)
            phase = "migration"
        elif kind in {"ServiceAccount", "NetworkPolicy"}:
            phase = "prerequisites"
        else:
            phase = "app"
        phases[phase].append(obj)
    if len(phases["migration"]) != 1 or not phases["prerequisites"] or not phases["app"]:
        raise ValueError("incomplete production profile")

    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp) / f"vmcp-kustomize-{version}"
        for phase, objects in phases.items():
            base = root / "base" / phase
            write_kustomization(base, ["resources.yaml"])
            (base / "resources.yaml").write_text(yaml.safe_dump_all(objects, sort_keys=False))
        for phase, wave in zip(phases, (-2, -1, 0)):
            write_kustomization(root / "prod" / phase, [f"../../base/{phase}"], wave)
        (root / "prod" / "kustomization.yaml").write_text(yaml.safe_dump({
            "apiVersion": "kustomize.config.k8s.io/v1beta1", "kind": "Kustomization",
            "namespace": "vmcp", "resources": list(phases),
        }, sort_keys=False))
        subprocess.run(["kubectl", "kustomize", root / "prod"], stdout=subprocess.DEVNULL, check=True)
        with tarfile.open(output / (root.name + ".tar.gz"), "w:gz") as archive:
            archive.add(root, arcname=root.name)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("chart", type=Path)
    parser.add_argument("version")
    parser.add_argument("output", type=Path)
    parser.add_argument("values", nargs="+", type=Path)
    args = parser.parse_args()
    package(args.chart, args.values, args.version, args.output)
