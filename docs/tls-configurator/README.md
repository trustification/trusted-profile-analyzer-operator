# TLS Configurator — documentation

Documentation for the TLS configurator that ships inside the operator image
(`cmd/tls-configurator`, `pkg/tlsconfigurator/`). See the
"TLS Configurator & Post-Quantum Cryptography (PQC)" section of the root
`CLAUDE.md` for how the module is wired into the Helm chart.

## Start here

- **[FINAL_PROJECT_STATUS.md](FINAL_PROJECT_STATUS.md)** — current state of the
  component: package layout, CLI actions and flags, the runtime reconcile flow,
  PQC, packaging, RBAC, test inventory and open items. Verified against the tree
  on 2026-09-28.

## Historical documents

These were written for the standalone `tls-openshift-configurator` repository,
before the component was merged into this operator. They are kept for the
rationale they record, but their paths, versions, build commands and deployment
model are **out of date** — `FINAL_PROJECT_STATUS.md` has a table summarising
exactly what changed in the merge.

| Document | Subject |
|---|---|
| [PROJECT_SUMMARY.md](PROJECT_SUMMARY.md) | Original technical overview |
| [IMPROVEMENTS_ANALYSIS.md](IMPROVEMENTS_ANALYSIS.md) | Gap analysis against PR #316 |
| [PR316_IMPLEMENTATION_SUMMARY.md](PR316_IMPLEMENTATION_SUMMARY.md) | What PR #316 changed and why |
| [UPDATES_FROM_PR316.md](UPDATES_FROM_PR316.md) | PR #316 changelog |
| [VERSION_CHECK_FEATURE.md](VERSION_CHECK_FEATURE.md) | OpenShift 4.22+ gate design |
| [README_VERSION_CHECK_ADDITION.md](README_VERSION_CHECK_ADDITION.md) | README snippet for the version gate |
| [README_UPDATE.md](README_UPDATE.md) | Usage examples |
| [LIBRARY_USAGE.md](LIBRARY_USAGE.md) | Consuming the packages as a Go library |
| [VERIFICATION_REPORT.md](VERIFICATION_REPORT.md) | Build verification of the standalone repo |
