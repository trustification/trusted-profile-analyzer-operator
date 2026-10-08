# TLS Configurator — documentation

Documentation for the TLS configurator that ships inside the operator image
(`cmd/tls-configurator`, `pkg/tlsconfigurator/`). See the
"TLS Configurator & Post-Quantum Cryptography (PQC)" section of the root
`CLAUDE.md` for how the module is wired into the Helm chart.

## Start here

- **[DEPLOYMENT.md](DEPLOYMENT.md)** — how to configure the module per
  platform: OpenShift 4.22+ (required, with `tlsAdherence` and post-quantum),
  OpenShift < 4.22 (optional), and plain Kubernetes (must stay off, and where
  TLS is configured instead). Includes verification commands and
  troubleshooting. A runnable CRC walkthrough is in
  [`devel/README.md`](../../devel/README.md).
- **[TLS_ADHERENCE.md](TLS_ADHERENCE.md)** — how the component implements Red
  Hat's *TLS Profile Compliance — Implementation Reference*: which upstream
  packages it delegates to, how `tlsAdherence` is read and used, what goes into
  the rollout hash, and how the post-quantum key-exchange group reaches both
  Go services and the router.
- **[FINAL_PROJECT_STATUS.md](FINAL_PROJECT_STATUS.md)** — current state of the
  component: package layout, CLI actions and flags, the runtime reconcile flow,
  PQC, packaging, RBAC, test inventory and open items. Verified against the tree
  on 2026-09-28; the TLS-adherence work postdates it, so prefer
  `TLS_ADHERENCE.md` where the two disagree.

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
