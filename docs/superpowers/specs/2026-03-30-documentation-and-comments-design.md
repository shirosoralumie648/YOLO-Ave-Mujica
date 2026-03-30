# Documentation And Comments Enhancement Design

## Overview

This design covers a documentation and code-comment pass for the current MVP repository. The goal is to make the project easier to approach for both first-time contributors and maintainers without changing runtime behavior.

The work focuses on three layers:

1. User-facing repository entry documentation
2. Developer-facing architecture and local workflow documentation
3. Focused code comments for core entry points and critical module boundaries

The repository already has a usable English `README.md`, a local quickstart, and detailed internal planning docs. What is missing is a cohesive Chinese documentation path and enough inline explanation around the control-plane, artifact delivery flow, CLI pull workflow, and Python worker primitives.

## Goals

- Provide a complete Simplified Chinese documentation path for onboarding and local development
- Keep the English repository entry point usable and better structured
- Add a developer-oriented Chinese architecture document that explains the main runtime flows and module responsibilities
- Improve inline code readability by documenting exported types, constructors, and core orchestration functions in key modules
- Keep the scope pragmatic so the documentation remains maintainable during MVP iteration

## Non-Goals

- No behavior changes to API, CLI, worker logic, or storage flows
- No full-repository comment coverage for every function or test
- No rewrite of existing `docs/superpowers/` specs and plans beyond linking to them where useful
- No large-scale refactor to rearrange packages purely for documentation reasons
- No bilingual duplication of every long-form internal doc

## Current State Summary

The repository currently exposes:

- A Go control plane with `api-server` and `platform-cli`
- Domain modules in `internal/` for data hub, jobs, artifacts, review, versioning, storage, config, queue, and server wiring
- Python worker-side helper modules under `workers/`
- Local developer helpers such as `Makefile`, Docker Compose, migrations, and `scripts/dev/smoke.sh`

Existing docs are useful but still sparse for a Chinese-speaking developer who wants to understand:

- How to boot the stack locally
- How the main modules are wired together
- How artifact packaging and `platform-cli pull` fit into the system
- What worker helper functions are expected to emit or compute

## Proposed Deliverables

### 1. Repository Entry Docs

#### Update `README.md`

Keep `README.md` as the main entry point in English, but restructure it so it works as a navigation document rather than just a compact summary.

Expected improvements:

- Clearer project positioning
- Better repository layout explanation
- More explicit quickstart expectations
- Pointers to the Chinese documentation set
- Pointers to implementation/spec docs only as secondary references

#### Add `README.zh-CN.md`

Add a Simplified Chinese repository overview that mirrors the practical information needed by a developer entering the project for the first time.

Content should include:

- Project purpose and current MVP scope
- Key repository directories
- Local startup steps
- Test and smoke commands
- Main API and CLI capabilities
- Links to the Chinese quickstart and architecture documentation

### 2. Development Docs

#### Update `docs/development/local-quickstart.md`

Keep the English quickstart concise, but make it read more like an operational guide.

Expected improvements:

- Stronger explanation of prerequisites
- Explicit startup sequence
- Better explanation of what smoke verifies
- More precise runtime notes around readiness, storage directory, and local API boot behavior

#### Add `docs/development/local-quickstart.zh-CN.md`

Provide a full Simplified Chinese quickstart covering:

- Dependency requirements
- Environment variable setup
- Local stack startup
- Migration execution
- Test execution
- Smoke verification
- Cleanup
- Common local caveats

#### Add `docs/development/architecture.zh-CN.md`

Provide a Chinese developer-oriented architecture overview that explains the system through flows and responsibilities rather than package listings alone.

The document should cover:

- Overall repository role split: `cmd/`, `internal/`, `workers/`, `scripts/`, `deploy/`
- `api-server` startup and module wiring
- Data Hub request flow
- Job creation, dispatch, and sweeper recovery flow
- Artifact creation, background build, storage, resolve, and download flow
- `platform-cli pull` resolve-download-extract-verify flow
- Python worker helper responsibilities and expected outputs
- Recommended reading order for maintainers

This document should emphasize current MVP behavior and explicitly avoid promising future architecture that is not yet implemented.

### 3. Inline Code Comments

The code-comment pass should focus on high-value explanation, not exhaustive annotation.

#### Go files to document

- `cmd/api-server/main.go`
- `internal/server/http_server.go`
- `internal/datahub/service.go`
- `internal/jobs/model.go`
- `internal/artifacts/service.go`
- `internal/cli/pull.go`

#### Python files to document

- `workers/common/job_client.py`
- `workers/zero_shot/main.py`
- `workers/cleaning/main.py`

#### Commenting strategy

For Go:

- Add doc comments for exported types, exported constructors, and exported functions where missing
- Explain orchestration points, dependency wiring, status semantics, and behavioral preconditions
- Add short inline comments only where a control-flow decision is not immediately obvious

For Python:

- Add concise docstrings to helper functions that emit event payloads or run worker-side rules
- Document payload intent and returned structure where it would otherwise require reading tests

What to avoid:

- Obvious comments that restate code literally
- Commenting every field assignment
- Adding comments to tests unless needed to explain unusual fixtures
- Introducing speculative documentation for features not present in code

## Content Design Principles

The documentation and comments should follow these principles:

- Ground every description in the code that currently exists
- Prefer operator and maintainer usefulness over marketing language
- Keep terminology consistent across README, quickstart, and architecture docs
- Use Chinese docs as first-class material, not partial translations
- Avoid overexplaining simple mechanics when command examples already make the flow clear

## Information Architecture

The final documentation navigation should look like this:

- `README.md`: English entry point and navigation hub
- `README.zh-CN.md`: Chinese entry point and onboarding overview
- `docs/development/local-quickstart.md`: English local runbook
- `docs/development/local-quickstart.zh-CN.md`: Chinese local runbook
- `docs/development/architecture.zh-CN.md`: Chinese architecture and module guide

This keeps the top-level repository approachable while keeping deeper development material under `docs/development/`.

## Validation Criteria

The work is successful when:

- A Chinese-speaking developer can follow the Chinese docs to start dependencies, run migrations, execute tests, and run smoke checks without needing the English docs
- The English `README.md` remains accurate and acts as a useful navigation entry
- The architecture document explains the main runtime flows in a way that matches the actual code
- Core exported Go symbols in the selected files have clear doc comments
- Worker helper functions have concise docstrings that explain emitted payloads or rule behavior
- No code behavior changes are introduced as part of the documentation pass

## Risks And Mitigations

### Risk: Documentation drifts from code

Mitigation:

- Base descriptions on current source files instead of prior planning docs alone
- Prefer documenting implemented flows only
- Run targeted verification after edits

### Risk: Comment noise reduces readability

Mitigation:

- Restrict comments to exported API surface and critical orchestration points
- Avoid mechanical line-by-line narration

### Risk: Duplicate docs become hard to maintain

Mitigation:

- Keep long-form architecture content only in Chinese for this pass
- Keep English docs concise and navigational
- Cross-link instead of duplicating detailed sections everywhere

## Execution Order

1. Update `README.md`
2. Add `README.zh-CN.md`
3. Update `docs/development/local-quickstart.md`
4. Add `docs/development/local-quickstart.zh-CN.md`
5. Add `docs/development/architecture.zh-CN.md`
6. Add focused code comments in selected Go files
7. Add focused docstrings in selected Python files
8. Run formatting and targeted verification

## Acceptance Summary

This design chooses a balanced documentation pass:

- broad enough to cover onboarding, development, and code readability
- narrow enough to stay maintainable for an MVP repository
- explicit enough that implementation can proceed without reopening scope questions
