#!/usr/bin/env python3
"""Vendor the external JSON schemas that the chart's values schema refers to.

values.schema.{yaml,json} used to `$ref` several schemas over the network:

    https://raw.githubusercontent.com/guacsec/trustify/<ref>/modules/importer/schema/importer.json
    https://raw.githubusercontent.com/guacsec/trustify/<ref>/common/auth/schema/auth.json
    https://raw.githubusercontent.com/yannh/kubernetes-json-schema/.../_definitions.json#/...

Helm resolves remote `$ref`s while validating values, so those URLs made
`helm template`/`install` fail outright in an air-gapped cluster.

The chart now refers to them by `urn:` instead. Helm (>= 3.20) resolves the
`urn:` scheme through the pluggable `chartutil.URNResolver`, which the operator
installs from the copies vendored here; see pkg/valuesschema. An unresolved
`urn:` is not an error -- Helm logs a warning and substitutes a permissive
schema -- so a standalone `helm install` still works offline, it just does not
validate those subtrees.

This script maintains the vendored copies. It is idempotent.

    hack/vendor-values-schemas.py            # check only (same as --check)
    hack/vendor-values-schemas.py --check    # fail if anything is stale
    hack/vendor-values-schemas.py --fetch    # re-download, then check

Checking covers three things: every vendored copy matches the digest in
SOURCES.json, every URN the chart references is vendored, and no remote `$ref`
has crept back into the chart schemas.

Bumping a version: edit "upstream" in pkg/valuesschema/schemas/SOURCES.json and
run with --fetch, then commit the refreshed copies.

The Kubernetes schemas are not whole upstream documents: `_definitions.json`
holds 577 definitions, so for each type the chart needs we extract that
definition plus the transitive closure of its internal `$ref`s into a
self-contained document. The `urn:` targets must stay whole documents with no
JSON-pointer fragment -- Helm's permissive fallback for an unresolved URN is a
bare `true` schema, and a `#/...` fragment against it fails to resolve.
"""

import argparse
import hashlib
import json
import pathlib
import re
import sys
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent
VENDOR_DIR = ROOT / "pkg" / "valuesschema" / "schemas"
SOURCES = VENDOR_DIR / "SOURCES.json"
CHART = ROOT / "helm-charts" / "redhat-trusted-profile-analyzer"
SCHEMA_JSON = CHART / "values.schema.json"
SCHEMA_YAML = CHART / "values.schema.yaml"

TRUSTIFY_RAW = "https://raw.githubusercontent.com/guacsec/trustify"

# Any leftover remote $ref in the chart schemas is a bug: it reintroduces the
# network dependency this vendoring exists to remove.
REMOTE_REF = re.compile(r'\$ref"?\s*:\s*"(https?://[^"]+)"')


def load_sources():
    return json.loads(SOURCES.read_text())


def canonical(doc):
    return json.dumps(doc, indent=2, ensure_ascii=False, sort_keys=True) + "\n"


def get(url):
    print(f"fetching {url}")
    with urllib.request.urlopen(url, timeout=60) as resp:  # noqa: S310
        return resp.read()


def closure(defs, root_name):
    """Collect root_name plus every definition reachable from it."""
    seen, queue = set(), [root_name]
    while queue:
        name = queue.pop()
        if name in seen:
            continue
        seen.add(name)
        queue.extend(n for n in refs_in(defs[name]) if n not in seen)
    return seen


def refs_in(node):
    """Yield the definition names referenced by '#/definitions/X' refs."""
    if isinstance(node, dict):
        for key, value in node.items():
            if key == "$ref" and isinstance(value, str):
                if value.startswith("#/definitions/"):
                    yield value[len("#/definitions/") :]
            else:
                yield from refs_in(value)
    elif isinstance(node, list):
        for item in node:
            yield from refs_in(item)


def build_kubernetes(upstream):
    """Extract each needed type into a self-contained draft-07 document.

    These have no upstream byte equivalent -- `_definitions.json` holds all 577
    definitions in one file -- so they are written canonically and their digest
    is a tamper check, not a provenance check. `origin` records that.
    """
    defs = json.loads(get(upstream["url"]))["definitions"]
    out = {}
    for filename, name in upstream["files"].items():
        if name not in defs:
            sys.exit(f"{filename}: {name} not found in {upstream['url']}")
        needed = closure(defs, name)
        doc = dict(defs[name])
        doc["$schema"] = "http://json-schema.org/draft-07/schema#"
        # The root's own definition is reachable from itself for recursive
        # types; keeping it in `definitions` costs nothing and keeps refs valid.
        doc["definitions"] = {n: defs[n] for n in sorted(needed)}
        out[filename] = (canonical(doc).encode(), "extracted")
    return out


def build_trustify(upstream):
    """Fetch the upstream documents byte for byte.

    They are already self-contained, so they are stored verbatim and their
    digest is a real provenance check against the published file.
    """
    out = {}
    for filename, path in upstream["files"].items():
        body = get(f"{TRUSTIFY_RAW}/{upstream['ref']}/{path}")
        json.loads(body)  # fail early on anything that is not valid JSON
        out[filename] = (body, "verbatim")
    return out


def fetch(sources):
    docs = {}
    docs.update(build_trustify(sources["upstream"]["trustify"]))
    docs.update(build_kubernetes(sources["upstream"]["kubernetes"]))
    for filename, (body, origin) in docs.items():
        (VENDOR_DIR / filename).write_bytes(body)
        sources["schemas"][filename]["origin"] = origin
        sources["schemas"][filename]["sha256"] = hashlib.sha256(body).hexdigest()
    SOURCES.write_text(canonical(sources))


def verify(sources):
    """Every vendored file must be present, parseable, and match its digest.

    For `verbatim` files the digest is the digest of the published upstream
    file, so this also checks provenance. For `extracted` ones it only proves
    the copy has not been hand-edited since it was generated.
    """
    problems = []
    for filename, meta in sources["schemas"].items():
        path = VENDOR_DIR / filename
        if not path.exists():
            problems.append(f"{filename}: missing; re-run with --fetch")
            continue
        body = path.read_bytes()
        try:
            doc = json.loads(body)
        except json.JSONDecodeError as err:
            problems.append(f"{filename}: not valid JSON: {err}")
            continue
        if meta.get("origin") == "extracted" and body.decode() != canonical(doc):
            problems.append(f"{filename}: not canonically formatted; re-run with --fetch")
        digest = hashlib.sha256(body).hexdigest()
        if digest != meta["sha256"]:
            problems.append(
                f"{filename}: sha256 {digest} does not match SOURCES.json "
                f"({meta['sha256']}); re-run with --fetch"
            )
    return problems


def check_chart_refs(sources):
    """The chart must reference only known URNs, and nothing over the network."""
    problems = []
    known = {meta["urn"] for meta in sources["schemas"].values()}
    for path in (SCHEMA_JSON, SCHEMA_YAML):
        text = path.read_text()
        rel = path.relative_to(ROOT).as_posix()
        for url in REMOTE_REF.findall(text):
            problems.append(f"{rel}: remote $ref reintroduced: {url}")
        for urn in set(re.findall(r'"(urn:[^"#]*)"', text)):
            if urn not in known:
                problems.append(f"{rel}: unknown urn {urn}")
        for urn in set(re.findall(r'"(urn:[^"]*#[^"]*)"', text)):
            problems.append(
                f"{rel}: {urn} has a JSON-pointer fragment; Helm's permissive "
                "fallback cannot resolve it when the URN is unregistered"
            )
    return problems


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fetch", action="store_true", help="re-download from upstream")
    parser.add_argument(
        "--check", action="store_true", help="check only; this is also the default"
    )
    args = parser.parse_args()

    sources = load_sources()
    if args.fetch:
        if args.check:
            sys.exit("--fetch and --check are mutually exclusive")
        fetch(sources)
        sources = load_sources()

    problems = verify(sources) + check_chart_refs(sources)
    if problems:
        sys.exit("\n".join(problems))
    print(f"{len(sources['schemas'])} vendored schemas up to date; no remote $refs in the chart")


if __name__ == "__main__":
    main()
