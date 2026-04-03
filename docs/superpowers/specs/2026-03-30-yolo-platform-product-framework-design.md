# YOLO Platform Product Framework Design

- Date: 2026-03-30
- Scope: Long-term product framework beyond the current MVP control plane
- Status: Drafted from approved product-design discussion
- Owner: Platform team

## 1. Document Positioning And Scope

### 1.1 Document Role

This document is the formal product framework spec for the long-term YOLO Platform.

It defines:

1. What the product is and is not.
2. Which problems the product must solve for visual AI teams.
3. The core platform modules, resource model, state machines, and deployment shapes.
4. The phased capability boundaries for V1, V1.5, and V2.
5. The technical landing guidance that should shape implementation inside this repository.

This document is the "mother spec" for the product.

It is intended to precede:

1. A separate user-journey and interaction spec.
2. Detailed implementation plans.
3. Module-level technical ADRs.

### 1.2 Relationship To Existing Repository Docs

This document does not replace the current MVP control-plane design.

The repository currently contains:

1. A current-state MVP design focused on Data Hub, Jobs, Review, and Artifacts.
2. A completion design for finishing the in-flight MVP implementation.
3. README files that describe the long-term product direction at a higher level.

This framework spec sits above those documents:

1. The MVP design remains the implementation reference for the currently shipped control-plane slice.
2. This framework spec defines the full product target the repository is growing toward.
3. Future implementation plans should trace back to this framework spec, not directly to README prose.

### 1.3 Problem Statement

The product exists to solve recurring problems for researchers, ML engineers, and annotation teams working on computer vision workflows:

1. Dataset versions are hard to track, leading to unreproducible experiments.
2. Pure manual annotation is too slow and too expensive.
3. Training, evaluation, and delivery are disconnected from the dataset and annotation process.
4. Teams rely on scripts, spreadsheets, chat, and naming conventions to coordinate production work.
5. Existing tools often solve only one slice: either annotation, or training, or dataset browsing, but not the production loop end to end.

### 1.4 Product Definition

The product is a visual AI engineering platform centered on:

1. Dataset governance.
2. Collaborative annotation.
3. AI-assisted labeling.
4. Review and publish control.
5. Training and evaluation lineage.
6. Artifact delivery.
7. CLI and SDK based remote workflows.

It is not intended to be:

1. A generic admin dashboard.
2. A pure annotation-only tool.
3. A thin model management UI disconnected from data.
4. A point solution that works only in SaaS or only in self-hosted mode.

### 1.5 Supported Problem Domain

The platform should primarily target 2D image and video workflows first, including:

1. Object detection.
2. Instance segmentation.
3. Classification.
4. Pose estimation.
5. Oriented bounding boxes (OBB).

Point cloud should be treated as a later extension domain through the plugin system.

### 1.6 Deployment Target

The product must support:

1. Hosted SaaS.
2. Dedicated single-tenant hosted deployment.
3. Self-hosted on-premise deployment.
4. Air-gapped on-premise deployment.

The business semantics must remain the same across all deployment shapes.

## 2. Product Goals, Principles, And Non-Goals

### 2.1 Product Goals

The long-term product should enable a team to:

1. Register and govern large-scale datasets without physically duplicating raw objects.
2. Create immutable, reproducible dataset snapshots.
3. Run collaborative annotation and structured review workflows online.
4. Use AI-assisted candidates without allowing models to bypass human review.
5. Launch, observe, and compare training runs against trusted dataset versions.
6. Deliver reproducible artifacts through Web, CLI, SDK, and API.
7. Close the loop by feeding review and evaluation feedback back into data and training decisions.

### 2.2 Core Product Principles

1. Metadata first.
2. Snapshot-bound reproducibility.
3. Publish by gate, not by convenience.
4. Human-reviewed canonical truth.
5. Traceability over hidden shortcuts.
6. Task-first UX rather than module-first navigation.
7. Capability-based extension rather than hard-coded verticalization.

### 2.3 Product Non-Goals For The First Product Line

The first product line is not expected to deliver:

1. A fully mature public plugin marketplace in V1.
2. Native point-cloud production workflows in V1.
3. A full business operations backend for quotas, billing, or tenant monetization in V1.
4. Exhaustive deployment benchmark automation for every edge runtime in V1.
5. A microservices-first architecture in V1.

## 3. Product Boundaries And Design Method

### 3.1 Two-Spec System

The product documentation should be split into two major specs:

1. Product framework spec.
2. User journey and interaction experience spec.

This document is the product framework spec.

The future user-journey spec should derive from this document and expand:

1. Entry flows.
2. Role-based workflows.
3. Page-level interaction logic.
4. Information hierarchy and visual behavior.

### 3.2 Design Bias

The platform should be designed as a production system, not a toy toolbox.

That means:

1. Every key action must have an owner, a version context, and an audit trail.
2. Every formal dataset, evaluation, and artifact must be lineage-bound.
3. Every workflow should be modeled as a state machine where responsibility is explicit.
4. Every extension point should respect the platform kernel instead of bypassing it.

### 3.3 Product Kernel

The platform kernel should always answer four questions:

1. Who did what under which scope?
2. Which versioned resource did it act upon?
3. Did the action enter a formal published state?
4. Can the full chain be replayed and audited later?

Everything else, including modality-specific rendering or backend connectors, should build on this kernel.

## 4. Core Product Modules

### 4.1 Module Overview

The platform should be organized into the following core modules:

1. Platform Foundation.
2. Data Hub.
3. Annotation Workspace.
4. AI Assist And Learning Loop.
5. Review And QA.
6. Training And Evaluation Hub.
7. Delivery And CLI.
8. Plugin Runtime And Marketplace.

### 4.2 Platform Foundation

Platform Foundation is responsible for:

1. Identity and access control.
2. Organizations, projects, scopes, and capabilities.
3. Audit logging.
4. Configuration and deployment profiles.
5. Job orchestration and event dispatch.
6. Plugin registration, isolation, and capability declaration.

It is the hard-constraint layer. No product module should bypass it.

### 4.3 Data Hub

Data Hub is responsible for:

1. Dataset registration rather than raw object migration.
2. Asset indexing over S3-compatible object storage.
3. Snapshot creation and diff.
4. Import and export codecs for formats such as COCO and YOLO.
5. Ontology management and version binding.
6. Derived asset lineage, such as extracted video frames.
7. Sensor calibration and multi-modal alignment metadata.

Snapshot implementation should use copy-on-write or virtual snapshot semantics rather than physical duplication.

Import and export must support streaming to avoid OOM behavior on large datasets.

### 4.4 Annotation Workspace

Annotation Workspace is responsible for:

1. Online manual annotation.
2. Team task assignment and collaborative work.
3. Video timeline operations, keyframing, interpolation, and track continuity.
4. Rendering AI-generated candidates as reviewable overlays.
5. Workspace session state and local productivity features such as shortcuts.

2D image and video labeling are built-in platform capabilities, not plugin-only features.

The module should be based on a unified Canvas Runtime rather than ad hoc modality-specific pages.

### 4.5 AI Assist And Learning Loop

AI Assist And Learning Loop is responsible for:

1. Generating candidate annotations from assisted models.
2. Supporting zero-shot and prompt-driven entry points over time.
3. Capturing confidence, source metadata, and suggestion provenance.
4. Receiving structured review and evaluation feedback through a feedback bus.
5. Driving hard-example mining, negative sampling, or training-strategy adjustments downstream.

This module has a forward path and a reverse path:

1. Forward assist to produce candidates.
2. Reverse feedback from reject and QA outcomes into the learning loop.

### 4.6 Review And QA

Review And QA is responsible for:

1. Structured review queues.
2. Accept, reject, rework, and escalation flows.
3. Quality-control checks.
4. Publish gating.
5. Machine-readable reason codes and feedback generation.

Review And QA is the only formal publish gate for canonical data.

### 4.7 Training And Evaluation Hub

Training And Evaluation Hub is responsible for:

1. Training run registration and lifecycle tracking.
2. Backend connector integration for remote or managed training.
3. Logs, curves, checkpoints, and environment context ingestion.
4. Evaluation run management.
5. Benchmark-aligned comparison across checkpoints.
6. Model promotion and recommendation decisions.

Evaluation is a first-class subsystem, not just a side output of training.

### 4.8 Delivery And CLI

Delivery And CLI is responsible for:

1. Exporting reproducible data payloads.
2. CLI-first data pull and artifact pull workflows.
3. Manifest-based integrity verification.
4. Uploading logs, curves, checkpoints, evaluation results, and artifacts back to the platform.
5. Supporting remote command generation and terminal-first workflows.

### 4.9 Plugin Runtime And Marketplace

Plugin Runtime And Marketplace is responsible for:

1. Runtime isolation for frontend and backend extensions.
2. Plugin capability declaration and compatibility contracts.
3. Extending codecs, renderers, connectors, and model adapters.
4. Supporting future domain expansion, including point cloud.

The product should follow a microkernel mindset:

1. The kernel owns resource flow, permissions, versions, and metadata.
2. Plugins extend modality-specific rendering, schema, compute, and integration behavior.

### 4.10 Multi-Modal Alignment In Data Hub

Although point cloud is not a V1 core workflow, the product must reserve multi-modal alignment structures in Data Hub, including:

1. `sensor_rigs`.
2. `sensors`.
3. `coordinate_frames`.
4. `sensor_calibrations`.
5. `capture_groups`.
6. `calibration_version` binding in training, labeling, and evaluation.

This is required to support later 2D-3D synchronization without redesigning the kernel.

## 5. Identity, Roles, Capabilities, And Authorization

### 5.1 Authorization Model

The platform should use a combined model:

1. RBAC for default role bundles.
2. Capability-based authorization for action granularity.
3. ABAC for tag or label-sensitive resource restrictions.
4. Scope-aware authorization over deployment, organization, project, and resource levels.

### 5.2 Subject Types

The platform should distinguish the following subject types:

1. Human user.
2. Guest or contractor.
3. Service account.
4. Worker identity.
5. Plugin identity.

### 5.3 Scope Levels

Authorization must support four scope levels:

1. Deployment.
2. Organization.
3. Project.
4. Resource.

This enables both multi-tenant SaaS and self-hosted deployments without changing product semantics.

### 5.4 Business Roles

Default business roles should include:

1. Platform Admin.
2. Organization Admin.
3. Project Owner.
4. Data Manager.
5. Annotator.
6. Reviewer.
7. ML Engineer.
8. Observer.

These should be implemented as opinionated capability bundles rather than hard-coded special cases.

### 5.5 Service Accounts And Temporary Access

The model must support:

1. Service accounts for automation and workers.
2. Time-limited guest or contractor access.
3. Restricted run-bound permissions for training scripts and upload clients.

Service accounts must not be allowed to perform UI login.

### 5.6 Feedback And Publish Capabilities

Capabilities should include explicit rights such as:

1. `feedback.write`.
2. `feedback.consume`.
3. `dataset.create_snapshot`.
4. `annotation.edit`.
5. `review.accept`.
6. `review.reject`.
7. `snapshot.publish`.
8. `artifact.promote`.
9. `project.invite_guest`.

### 5.7 Authorization Principles

The following principles are mandatory:

1. Audit by design for publish, delete, review, and promotion actions.
2. Fail-closed default behavior when capability or scope mapping is absent.
3. ABAC constraints for sensitive dataset tags or labels.
4. Service accounts constrained to the minimum resource set required by their bound job or run.

## 6. Unified Resource Model And Lineage

### 6.1 Core Resource Set

The platform should maintain a unified lineage model across the following resources:

1. Deployment.
2. Organization.
3. Project.
4. Dataset.
5. Asset.
6. CaptureGroup.
7. CalibrationBundle.
8. Snapshot.
9. Annotation.
10. Task.
11. ReviewRecord.
12. FeedbackRecord.
13. Job.
14. TrainingRun.
15. Checkpoint.
16. MetricSeries.
17. EvaluationReport.
18. Artifact.
19. PluginPackage.

### 6.2 Asset And Derived Asset

Asset should support derivation tracking through `parent_asset_id`.

This is required for:

1. Video frame extraction.
2. Resolution conversion.
3. Preprocessing outputs.
4. Coordinate remapping across derived views.

### 6.3 CaptureGroup And CalibrationBundle

`CaptureGroup` represents the logical atomic unit of synchronized multi-sensor capture.

`CalibrationBundle` must be:

1. Versioned.
2. Bindable to downstream workflows.
3. Marked with `is_valid`.

If a calibration bundle becomes invalid, tasks, publish actions, training, and evaluation that depend on it must be blocked or marked untrusted.

### 6.4 Snapshot Semantics

Snapshots must:

1. Use virtual or copy-on-write semantics.
2. Be immutable once sealed.
3. Carry `content_hash` or `merkle_root`.
4. Serve as the only formal version input for training, evaluation, and artifact lineage.

No training or evaluation result without snapshot binding should be treated as formally trusted.

### 6.5 Annotation Model Split

The annotation model should be split into:

1. `AnnotationCore`.
2. `AnnotationGeometry`.
3. `AnnotationAttributes`.
4. `AnnotationState`.

This allows the kernel to reason about resource ownership and lifecycle without hard-coding geometry specifics.

`geometry` should be stored in a flexible format such as `JSONB`, validated against platform or plugin-provided schema.

### 6.6 Ontology Binding

Annotations and downstream runs must bind to an explicit `ontology_version`.

This is necessary because class definitions evolve over long-running projects.

### 6.7 Review And Feedback Separation

The system should separate:

1. `ReviewRecord` for human workflow and compliance.
2. `FeedbackRecord` for machine-consumable learning and routing.

FeedbackRecord should include fields such as:

1. `reason_code`.
2. `severity`.
3. `influence_weight`.
4. `uncertainty_score`.
5. `loss_contribution`.
6. `actionable`.

### 6.8 Published Data Immutability

Once an annotation is published through a snapshot, it becomes immutable.

Any correction must:

1. Produce a new working record.
2. Flow through review.
3. Enter a later snapshot.

### 6.9 Lineage Chain

The canonical lineage chain should support tracing from deployed or downloadable output back to source data:

`Artifact -> EvaluationReport -> Checkpoint -> TrainingRun -> Snapshot -> Dataset -> Asset -> CaptureGroup -> CalibrationBundle`

This lineage must be queryable and visible in the product surface.

## 7. System Architecture And Delivery Shapes

### 7.1 Layered Architecture

The system should be expressed through three main technical layers:

1. Metadata Control Plane.
2. Execution Plane.
3. Storage And Compute Substrate.

### 7.2 Access Surfaces

Users and systems should interact through four access surfaces:

1. Web Console.
2. CLI.
3. SDK.
4. API.

These surfaces must share the same product semantics even when the user experience differs.

### 7.3 Control Plane

The control plane is responsible for:

1. Metadata APIs.
2. IAM and authorization.
3. Audit.
4. Snapshot and lineage control.
5. Job registration and orchestration.
6. Review and publish gating.
7. Artifact registry and promotion state.

### 7.4 Execution Plane

The execution plane is responsible for:

1. AI assist jobs.
2. Cleaning jobs.
3. Video processing jobs.
4. Import and export codecs.
5. Training connectors.
6. Evaluation runners.
7. Plugin sidecars.

Workers must declare their capabilities, resource profile, and supported operations.

### 7.5 Storage And Compute Substrate

The substrate layer may vary by deployment, but must be abstracted behind stable contracts:

1. PostgreSQL or compatible relational metadata storage.
2. Redis or alternative queue coordination.
3. S3-compatible object storage.
4. Training backends such as local runners, SSH-style execution, Slurm, or Kubernetes.

### 7.6 Data Flows

The architecture must formalize at least five major flows:

1. Data ingestion flow.
2. Annotation flow.
3. AI feedback flow.
4. Training flow.
5. Delivery flow.

Each flow should support `origin_context_id` or equivalent metadata to connect actions and outcomes across modules.

### 7.7 Media Out Of Band

API Server must not proxy large media by default.

Media access should happen through:

1. Short-lived pre-signed URLs.
2. Private endpoints in internal deployments.
3. A controlled Storage Gateway Proxy when direct access is not possible.

### 7.8 Plugin Isolation

Plugins must be isolated:

1. Frontend plugins via iframe or worker-based sandbox boundaries.
2. Backend plugins via sidecar, sub-process, or remote gRPC boundaries.

No plugin should be allowed to corrupt control-plane state or bypass authorization.

### 7.9 Architecture Laws

The following architecture laws are mandatory:

1. Metadata First.
2. Stateless API.
3. Traceability.
4. Media Out Of Band.
5. Plugin Isolation.
6. Capability Degradation rather than semantic divergence.

## 8. Capability Layers, Matrix, And Module Interactions

### 8.1 Capability Layers

The platform should define four capability layers:

1. L0: Foundation capabilities.
2. L1: Built-in core business capabilities.
3. L2: Enhancement and closed-loop productivity capabilities.
4. L3: Pluggable extension capabilities.

#### L0: Foundation

1. IAM.
2. Audit.
3. Scopes and capabilities.
4. Resource model and lineage.
5. Job orchestration and eventing.
6. Storage access control.
7. Plugin registration and isolation.

#### L1: Built-In Core Business

1. 2D image and video Data Hub.
2. Online annotation workspace.
3. Review and QA.
4. AI assist basics.
5. Training and evaluation basics.
6. CLI, SDK, API access surfaces.

#### L2: Enhancement And Closed Loop

1. Active learning.
2. Hard-example mining.
3. Negative sampling.
4. Multi-checkpoint evaluation comparison.
5. Quality scanning and cleaning.
6. Command generation and remote training integration.

#### L3: Pluggable Extensions

1. Point cloud workflows.
2. Custom codecs.
3. Custom model adapters.
4. Specialized training connectors.
5. Plugin marketplace and private registries.

### 8.2 Capability Matrix

#### Platform Foundation

Built-in responsibilities:

1. IAM.
2. Audit.
3. Job orchestration.
4. Events.
5. Plugin runtime.

Pluggable responsibilities:

1. SSO provider integrations.
2. Audit sinks.
3. Optional storage gateway proxy.

Key outputs:

1. Unified identity.
2. Unified tasking.
3. Unified policy enforcement.

#### Data Hub

Built-in responsibilities:

1. Dataset and asset metadata.
2. Snapshot and diff.
3. Import and export.
4. Ontology and schema references.
5. Calibration and capture-group metadata.

Pluggable responsibilities:

1. Industry codecs.
2. Storage adapters.
3. Future point-cloud indexing helpers.

Key outputs:

1. Reproducible data versions.

#### Annotation Workspace

Built-in responsibilities:

1. 2D and video labeling.
2. Collaboration and task execution.
3. Frame interpolation and track continuity.
4. AI candidate interaction.

Pluggable responsibilities:

1. Point-cloud renderer.
2. Specialized toolbars.
3. Industry-specific interaction logic.

Key outputs:

1. Structured annotation work product.

#### AI Assist And Learning Loop

Built-in responsibilities:

1. Candidate generation.
2. Candidate scoring and provenance.
3. Feedback bus routing.

Pluggable responsibilities:

1. Model adapters.
2. Active-learning strategies.
3. Domain-specific assist engines.

Key outputs:

1. Reviewable candidates.
2. Hard-example and negative-sample signals.

#### Review And QA

Built-in responsibilities:

1. Review queues.
2. Accept/reject/rework.
3. Publish gate.
4. QA rule enforcement.

Pluggable responsibilities:

1. Industry QA rule packs.

Key outputs:

1. Publishable snapshots.
2. Structured machine-consumable feedback.

#### Training And Evaluation Hub

Built-in responsibilities:

1. Training run tracking.
2. Logs and curves.
3. Checkpoint registration.
4. Evaluation reports.
5. Comparison and promotion.

Pluggable responsibilities:

1. Backend connectors.
2. Benchmark integrations.
3. Hardware-specific extensions.

Key outputs:

1. Comparable model outputs.
2. Recommended artifact candidates.

#### Delivery And CLI

Built-in responsibilities:

1. Pull commands.
2. Upload commands.
3. Integrity verification.
4. Artifact distribution.

Pluggable responsibilities:

1. Private registry adapters.
2. Remote execution helpers.

Key outputs:

1. Training entry points.
2. Reproducible delivery outputs.

#### Plugin Runtime And Marketplace

Built-in responsibilities:

1. Plugin installation.
2. Isolation.
3. Capability declaration.

Pluggable responsibilities:

1. The plugins themselves.

Key outputs:

1. Extendable platform behavior.

### 8.3 Write Boundaries

The following write boundaries are mandatory:

1. Data Hub is the only authority for dataset, asset, snapshot, ontology, and calibration metadata.
2. Workspace writes working-state annotations and task progress only.
3. AI Assist writes candidates and feedback only, not canonical annotations.
4. Review And QA is the only module that can formally gate data into published canonical snapshots.
5. Training And Evaluation consumes published snapshots only.
6. Delivery And CLI distributes only registered artifacts and attached outputs.
7. Plugin Runtime may extend behavior but may not bypass permissions, audit, lineage, or publish constraints.

### 8.4 Core Interaction Chains

#### Chain A: Data To Task Readiness

1. Objects enter storage.
2. Data Hub indexes and validates metadata.
3. Assets and derived assets become task-consumable.

#### Chain B: Annotation To Publish

1. Workspace operates on task-bound working data.
2. Human edits and AI candidates remain non-canonical until review.
3. Review decisions control whether data enters a new published snapshot.

#### Chain C: AI Forward Assist And Reverse Learning

1. AI Assist generates candidates from existing data context.
2. Candidates enter review queues.
3. Reject and QA outcomes emit `FeedbackRecord`.
4. Downstream learning systems consume feedback and adjust prioritization or strategy.

`FeedbackRecord` must include `influence_weight` so severe failure classes can drive stronger resampling or loss weighting.

#### Chain D: Training To Evaluation

1. Training consumes published snapshots.
2. Logs, curves, checkpoints, and context flow back into the platform.
3. Evaluation produces comparable reports only when `benchmark_snapshot_id` matches.

Comparisons across different benchmark snapshots must not be ranked directly.

#### Chain E: Delivery To Reuse

1. Delivery packages versioned artifacts.
2. CLI and SDK both download and upload production outputs.
3. Downstream reuse remains traceable through manifest and lineage links.

### 8.5 Mandatory Built-In V1 Scope

V1 must include:

1. 2D image and video Data Hub.
2. 2D and video online annotation and review.
3. AI pre-label and feedback basics.
4. Training run registration and result upload.
5. Evaluation comparison under shared benchmark snapshots.
6. CLI pull and upload workflows.
7. Snapshot, diff, publish, artifact, and audit primitives.

### 8.6 V1.5 And V2 Expansion

V1.5 and V2 may extend:

1. Active-learning sophistication.
2. Plugin ecosystem maturity.
3. Point-cloud workflows.
4. Richer deployment benchmarking and conversion pipelines.

## 9. Core Workflow State Machines

### 9.1 State Machine Principles

1. All formal production outputs must originate from published snapshots.
2. Published canonical objects are immutable.
3. Every state transition must have a responsible actor.
4. Critical transitions must produce audit and event records.
5. Failures become explicit states, not silent data loss.

### 9.2 Dataset And Snapshot State

#### Dataset lifecycle

1. `draft`
2. `active`
3. `archived`
4. `deleted_soft`

#### Snapshot lifecycle

1. `building`
2. `sealed`
3. `published`
4. `superseded`
5. `invalidated`

`invalidated` should be used for severe issues such as schema corruption, ontology mismatch, or calibration invalidation.

### 9.3 Task And Annotation State

#### Task lifecycle

1. `queued`
2. `ready`
3. `in_progress`
4. `submitted`
5. `reviewing`
6. `rework_required`
7. `accepted`
8. `published`
9. `closed`

#### Annotation lifecycle

1. `draft`
2. `submitted`
3. `accepted`
4. `rejected`
5. `published`
6. `superseded`

### 9.4 AI Candidate And Feedback State

#### AI candidate lifecycle

1. `generated`
2. `queued_for_review`
3. `accepted`
4. `rejected`
5. `expired`

#### FeedbackRecord lifecycle

1. `recorded`
2. `classified`
3. `routed`
4. `consumed`
5. `retained`

`FeedbackRecord` should carry at least:

1. `reason_code`
2. `severity`
3. `influence_weight`
4. `uncertainty_score`
5. `loss_contribution`
6. `actionable`
7. `source_snapshot_id`
8. `source_annotation_id` or `source_candidate_id`

### 9.5 Review And Publish State

#### ReviewRecord lifecycle

1. `pending`
2. `in_review`
3. `approved`
4. `rejected`
5. `needs_escalation`
6. `resolved`

Reject operations must require a structured `reason_code`.

#### Publish gate conditions

1. Valid `snapshot_id`.
2. Valid `calibration_version` when applicable.
3. Accepted annotations only.
4. Required QA checks passed.
5. Responsible subject and scope recorded.

### 9.6 Training, Evaluation, And Artifact State

#### TrainingRun lifecycle

1. `created`
2. `scheduled`
3. `running`
4. `succeeded`
5. `failed`
6. `canceled`
7. `superseded`

#### EvaluationReport lifecycle

1. `pending`
2. `running`
3. `completed`
4. `invalid`
5. `superseded`

`completed` requires full benchmark context, including `benchmark_snapshot_id`.

#### Artifact lifecycle

1. `building`
2. `ready`
3. `published`
4. `deprecated`
5. `revoked`

### 9.7 Cross-State Constraints

1. `Annotation.published` requires review approval and publish-gate success.
2. `TrainingRun.created` must reference a published snapshot.
3. `EvaluationReport.completed` must reference a valid checkpoint and benchmark snapshot.
4. `Artifact.published` must trace to a single evaluation report or a clearly documented exemption path.
5. Invalid calibration must block dependent new publish, training, and evaluation actions.
6. `actionable = true` feedback must reach at least one downstream consumer or enter a blocker queue.

## 10. Deployment Shapes, Environment Layers, And Unified Delivery Model

### 10.1 Deployment Goal

The product must support both hosted and private deployment without splitting into multiple products.

Differences must be implemented through:

1. Connectors.
2. Deployment profiles.
3. Infrastructure adapters.
4. Feature degradation rules.

They must not be implemented through divergent business semantics.

### 10.2 Deployment Shapes

Supported shapes:

1. Hosted SaaS.
2. Dedicated hosted single-tenant.
3. Self-hosted on-premise.
4. Air-gapped on-premise.

### 10.3 Environment Layers

The product should distinguish four environment layers:

1. Product Semantic Layer.
2. Control Plane Layer.
3. Execution Plane Layer.
4. Infrastructure Substrate Layer.

The semantic layer must remain constant across deployments.

### 10.4 Unified Delivery Principles

1. Semantic consistency across SaaS and On-Prem.
2. Control plane separated from heavy media traffic.
3. Degrade capability, never resource semantics.
4. Profile-driven deployment, not code forks.

### 10.5 SaaS Shape

SaaS should support:

1. Multi-tenant control plane.
2. Shared execution pools.
3. Tenant isolation through organization and resource boundaries.
4. Shared plugin governance.

### 10.6 On-Prem Shape

On-prem should support:

1. Customer-owned control plane deployment.
2. Enterprise storage and identity integration.
3. Internal training infrastructure connectors.
4. Optional Storage Gateway Proxy.
5. Private or offline plugin distribution.

### 10.7 Object Access Modes

The platform should support four object access modes:

1. Direct object access by pre-signed URL.
2. Private endpoint access.
3. Gateway proxy access.
4. Offline package exchange.

### 10.8 Authentication Integration

SaaS default:

1. Platform-local account/password.
2. Optional OIDC or enterprise SSO.

On-Prem default:

1. Local account support.
2. Enterprise directory support such as LDAP, OIDC, SAML, or similar.

All identity sources must map back into the same subject model.

### 10.9 CLI And SDK Upstream Upload

CLI and SDK must support not only pull but also upload of:

1. Training logs.
2. Curves and metrics.
3. Checkpoints.
4. Evaluation reports.
5. Final artifacts.
6. Environment context.

Uploads without snapshot or benchmark bindings must be marked as temporary results.

### 10.10 Plugin Distribution And Upgrade

The system should support:

1. Official SaaS plugin distribution.
2. Private on-prem plugin registries.
3. Offline signed plugin bundles in air-gapped environments.

Core platform upgrades, plugin upgrades, and model package upgrades must remain separable.

### 10.11 Allowed Capability Degradation

Allowed:

1. No Redis, fall back to DB polling queues.
2. No public object access, use private endpoints or gateway proxy.
3. No public plugin market, use private or offline distribution.
4. No cloud training backend, use local or remote connectors.
5. No SSO, use local accounts.
6. No GPU, degrade AI assist and training acceleration.

Not allowed:

1. No audit.
2. No snapshots.
3. No publish gate.
4. No lineage enforcement.

## 11. Roles, Main Journeys, And Core Scenario Matrix

### 11.1 Role Layers

The product should distinguish:

1. Business roles.
2. System identities.

Business roles define default entry points and workflow responsibilities.

System identities define authorization and runtime behavior.

### 11.2 Business Roles

1. Platform Admin.
2. Organization Admin.
3. Project Owner.
4. Data Manager.
5. Annotator.
6. Reviewer or QA.
7. ML Engineer.
8. Observer.
9. External collaborator or outsourcing partner.
10. Automated actor through Service Account.

### 11.3 Responsibility Summary

#### Platform Admin

1. Deployment configuration.
2. Security and policy.
3. Plugin governance.
4. System health and audit.

#### Organization Admin

1. Org membership.
2. Project visibility.
3. Policy and governance.
4. Capacity and access boundaries.

#### Project Owner

1. Delivery goal alignment.
2. Publish rhythm.
3. Main blocker triage.
4. Promotion approvals.

#### Data Manager

1. Dataset registration.
2. Snapshot creation.
3. Ontology, schema, and calibration setup.
4. Import and export control.

#### Annotator

1. Annotation production.
2. Candidate acceptance or editing.
3. Task submission.

#### Reviewer Or QA

1. Review queue processing.
2. Reject, rework, and reason-code generation.
3. Publish gate participation.
4. Feedback creation.

#### ML Engineer

1. AI assist integration.
2. Training and evaluation.
3. Checkpoint comparison.
4. Recommended artifact nomination.

#### Observer

1. Progress and result visibility.
2. Read-only oversight.

#### External Collaborator

1. Restricted task-scoped contribution.
2. Time-bounded access.

#### Service Account

1. Job-bound or run-bound automation.
2. Idempotent upload and execution support.

Service accounts must not be allowed to modify resources outside their bound job or run.

### 11.4 Default Product Entry

The default logged-in entry point should be `Task Overview`.

The homepage should be task-driven rather than BI-driven.

It should answer:

1. What requires attention now?
2. Which blocker is delaying production?
3. Which versioned resources are currently in flight?
4. Which runs failed or require action?

### 11.5 Main Journeys

The framework should define three main product journeys:

#### Journey A: Data To Published Data

1. Ingest or register data.
2. Form snapshot context.
3. Create and execute tasks.
4. Annotate with human and AI assist.
5. Review and publish a trusted snapshot.

#### Journey B: Published Data To Comparable Model

1. Choose a published snapshot.
2. Launch or register training.
3. Upload logs, checkpoints, and evaluation.
4. Compare under a shared benchmark snapshot.
5. Select recommended outputs.

#### Journey C: Result To Learning Loop

1. Review and evaluation errors produce feedback.
2. Feedback routes into active learning or training adjustments.
3. Hard examples and negative samples feed the next cycle.

### 11.6 Core Scenario Matrix

The product must support at least these scenarios:

1. Ingest a new dataset.
2. Create an annotation project.
3. Run collaborative human labeling.
4. Run AI-assisted pre-labeling.
5. Review and publish.
6. Launch training and upload outputs.
7. Compare evaluation results.
8. Promote a recommended model artifact.
9. Feed difficult samples back into the next cycle.
10. Support scoped external collaboration.

### 11.7 V1 Role Closure Priority

V1 should fully close the loop for:

1. Data Manager.
2. Annotator.
3. Reviewer.
4. ML Engineer.
5. Project Owner.

Other roles may exist in V1, but they should not dominate the product design effort.

### 11.8 Additional Journey Constraints

1. `Blockers View` must help Project Owner see bottlenecks in the chain.
2. Review reject actions must require machine-readable reason code input.
3. Model promotion should be a formal scenario: ML Engineer nominates, Project Owner approves.
4. Cross-page actions should inherit context from dataset, snapshot, task, or run whenever possible.

## 12. V1 Information Architecture And Page Skeleton

### 12.1 Page Design Principles

1. Task-first, not module-first.
2. Explicit version visibility.
3. Context inheritance across pages.
4. Separation of list, detail, and workstation responsibilities.
5. High-density but readable tooling UX.

### 12.2 Primary Navigation

V1 should use six primary navigation domains:

1. `Overview`
2. `Data`
3. `Tasks`
4. `Review`
5. `Training`
6. `Artifacts`

Configuration and governance should remain outside the primary task rail where possible.

### 12.3 V1 Page Tree

Overview:

1. `Task Overview`
2. `Project Blockers View`

Data:

1. `Dataset List`
2. `Dataset Detail`
3. `Snapshot Detail`
4. `Snapshot Diff`
5. `Import / Export Runs`

Tasks:

1. `Task List`
2. `My Tasks`
3. `Task Detail`
4. `Annotation Workspace`

Review:

1. `Review Queue`
2. `Review Detail / Review Workspace`
3. `Publish Candidates`

Training:

1. `Training Run List`
2. `Training Run Detail`
3. `Evaluation Compare`
4. `Recommended Model Promotion`

Artifacts:

1. `Artifact Registry`
2. `Artifact Detail`
3. `CLI / SDK Access`

Settings:

1. `Project Settings`
2. `Ontology Management`
3. `Members And Roles`
4. `Service Accounts`
5. `Audit Log`

### 12.4 Task Overview

Task Overview is the default home page and primary operational surface.

It should include:

1. Priority work items by role.
2. Review backlog.
3. Data production state.
4. Recent training and evaluation changes.
5. Artifact recommendation state.
6. `Blockers View`.
7. `Longest Idle Task` to expose forgotten work.

Every blocker item should link directly to a handling page.

### 12.5 Data Domain Pages

#### Dataset List

Should show:

1. Dataset name.
2. Data type.
3. Scale summary.
4. Latest snapshot.
5. Status.
6. Owner.

#### Dataset Detail

Should show:

1. Asset summary.
2. Distribution summary.
3. Related tasks.
4. Related snapshots.
5. Import and export history.
6. Ontology, schema, and calibration summary.

#### Snapshot Detail

Should show:

1. Snapshot metadata.
2. Publish status.
3. Diff summary.
4. Dataset binding.
5. Downstream training, evaluation, and artifact references.

#### Snapshot Diff

Should show:

1. Distribution changes.
2. Annotation deltas.
3. Review and publish impact summary.

### 12.6 Task And Workspace Pages

#### Task List

Should support:

1. Status views.
2. Assignment.
3. Backlog analysis.
4. Rework rate visibility.

#### My Tasks

Should prioritize:

1. Due work.
2. Returned work.
3. High-priority work.

#### Task Detail

Should show:

1. Description.
2. Bound snapshot.
3. Assignment and ownership.
4. State timeline.
5. Audit references.

#### Annotation Workspace

This is one of the most important V1 pages.

It must include:

1. Image or video work surface.
2. Tool palette.
3. Layer and object list.
4. AI candidate panel.
5. Timeline and frame navigation.
6. Task context and version context.
7. Save and submit state.
8. History and conflict hints.

Implementation requirements:

1. Frame list, object list, and timeline must be virtualized.
2. Video operations must remain responsive under large frame counts.
3. `Object Persistence Checker` must flag broken object continuity or ID gaps across frames.

### 12.7 Review Pages

#### Review Queue

Should support:

1. Priority sorting.
2. Risk sorting.
3. Filter by source model, annotator, task, and version.

#### Review Workspace

Should reuse the Canvas Runtime but switch into review mode.

It must expose:

1. Candidate confidence.
2. Source model metadata.
3. Previous modifier.
4. Accept, reject, rework, and escalate actions.

Reject must require:

1. `reason_code`.
2. Optional note.
3. Optional severity.
4. Optional `influence_weight` guidance.

#### Publish Candidates

Should show:

1. Candidate task group.
2. QA status.
3. Expected diff impact.
4. Downstream publish consequences.

### 12.8 Training Pages

#### Training Run List

Should focus on:

1. Status.
2. Failure rate.
3. Bound snapshot.
4. Recent evaluation results.

#### Training Run Detail

Should include:

1. Bound snapshot.
2. Runtime configuration.
3. Log stream.
4. Curves.
5. Checkpoints.
6. Environment context.
7. Failure explanation.

#### Evaluation Compare

This is one of the most important V1 pages.

It must include:

1. `benchmark_snapshot_id`.
2. Compared checkpoints.
3. Metric comparison.
4. Sample-level comparison.
5. Environment and protocol context.

If compared runs do not share the same `benchmark_snapshot_id`, the page must not display a direct ranking.

V1 should offer a "create aligned evaluation" action rather than fake a direct comparison.

#### Recommended Model Promotion

Should support:

1. Nomination by ML Engineer.
2. Approval by Project Owner.
3. Reasoned promotion into a recommended artifact.

### 12.9 Artifact Pages

#### Artifact Registry

Should show:

1. Status.
2. Source lineage.
3. Intended task type.
4. Download and usage activity.

#### Artifact Detail

Should show:

1. Source evaluation and checkpoint.
2. Manifest summary.
3. Download options.
4. CLI and SDK command references.
5. Deprecation or revoke state.

#### CLI / SDK Access

Should provide:

1. Pull commands.
2. Upload guidance.
3. Token and environment setup.
4. Training integration examples.

### 12.10 Context Inheritance And Routing State

Cross-page context inheritance is mandatory.

Important context should be represented in URL params rather than only in invisible global state.

Examples:

1. `/training/runs/45?snapshot_id=snap-009`
2. `/training/evaluations/compare?benchmark_snapshot_id=snap-010&run_ids=44,45`
3. `/workspace/task-1001?asset_id=asset-22&frame=183&object_id=obj-98`

This ensures shareable, reproducible, and reviewable context.

### 12.11 Core Shared UI Components

V1 should define and consistently use shared UI components such as:

1. `Entity Header`
2. `Version Badge`
3. `State Timeline`
4. `Diff Summary Panel`
5. `Lineage Breadcrumb`
6. `Blocker Card`
7. `Reason Code Tag`
8. `Confidence Badge`
9. `Run Status Strip`
10. `Promotion Decision Panel`

The visual system should emphasize semantic clarity rather than generic admin styling.

## 13. Phased Scope: V1, V1.5, And V2

### 13.1 Phasing Principles

1. Every phase must produce a usable closed loop.
2. 2D image and video workflows must be completed before native point-cloud workflows.
3. Productivity and lineage reliability should beat breadth in early phases.

### 13.2 V1 Definition

V1 must let a small or medium visual AI team complete this loop:

1. Register image and video data.
2. Create snapshots and tasks.
3. Perform online annotation and review.
4. Publish trusted snapshots.
5. Launch or register training.
6. Upload logs, curves, checkpoints, and evaluations.
7. Compare checkpoints under one benchmark snapshot.
8. Publish and download recommended artifacts.

#### V1 required built-in capabilities

Platform Foundation:

1. Local account login.
2. Basic RBAC plus capability and scope.
3. Basic audit.
4. Job orchestration.
5. Event basics.
6. Plugin runtime skeleton.

Data Hub:

1. S3-compatible storage integration.
2. Dataset and asset registration.
3. Image and video resources.
4. Virtual snapshots.
5. Snapshot diff.
6. YOLO and COCO import/export.
7. Ontology version basics.
8. Derived asset lineage.
9. Calibration and capture-group structures reserved.

Annotation Workspace:

1. Detection.
2. Instance segmentation.
3. Classification.
4. Pose estimation.
5. OBB.
6. Video timeline basics.
7. Task assignment and submission.
8. AI candidate acceptance and rejection.

Review And QA:

1. Review queue.
2. Accept, reject, rework.
3. Reject with required `reason_code`.
4. Publish gate.
5. Publish change summary.

AI Assist And Learning Loop:

1. Assisted pre-label entry.
2. Zero-shot skeleton entry point.
3. Candidate source metadata.
4. Feedback bus basics.
5. Structured feedback fields such as `reason_code`, `influence_weight`, and `uncertainty_score`.

Training And Evaluation Hub:

1. Training run registration.
2. Remote-command-friendly workflow.
3. CLI or SDK upload of logs, curves, checkpoints, and evaluation.
4. Benchmark snapshot binding.
5. Multi-checkpoint comparison basics.
6. Recommended artifact nomination basics.

Delivery And CLI:

1. Data pull.
2. Artifact pull.
3. Manifest verification.
4. Verify report.
5. Training upstream upload.
6. Command generation support.

Web Frontend:

1. `Vite + React + TypeScript`.
2. `Task Overview` as default entry.
3. `Blockers View`.
4. Dataset, Snapshot, Workspace, Review, Training, Evaluation, and Artifact main surfaces.

### 13.3 V1 Explicit Non-Goals

V1 should not include:

1. Native point-cloud workstation.
2. Full 2D-3D synchronized labeling.
3. Mature public plugin market.
4. Full deployment-conversion matrix such as TensorRT, OpenVINO, or RKNN automation.
5. Advanced active-learning automation.
6. Full business operations backend.

### 13.4 V1 Acceptance Bar

V1 is complete only if:

1. Multiple datasets and snapshots can be managed stably.
2. Published data remains immutable and diff-visible.
3. A real training-to-evaluation-to-artifact chain is usable.
4. CLI can both pull and upload key workflow outputs.
5. The five priority roles can complete their main work.
6. Task Overview exposes tasks, blockers, and failures clearly.

### 13.5 V1.5 Definition

V1.5 should reinforce weak points rather than explode scope.

Likely priorities:

1. Better blocker diagnosis.
2. Stronger active-learning sample recommendation.
3. Stronger QA rules.
4. Richer evaluation comparison reports.
5. More complete service-account lifecycle and boundaries.
6. External collaborator flows.
7. Storage Gateway Proxy productization.
8. More fixed SaaS and On-Prem deployment profiles.

### 13.6 V2 Definition

V2 should focus on platformization and ecosystem maturity:

1. Formal plugin-first expansion model.
2. Point cloud through plugins.
3. 2D/3D synchronization interfaces.
4. More mature deployment, conversion, and benchmark tooling.
5. Richer governance and organization features.

## 14. Non-Functional Requirements, Technical Landing, And Risks

### 14.1 Non-Functional Baseline

The platform must meet the following non-functional goals:

1. Correctness over novelty.
2. Traceability over convenience.
3. Operational clarity over hidden magic.
4. Performance sufficient for high-frequency workstation use.
5. Capability to degrade across environments without semantic drift.

### 14.2 Performance Requirements

Control plane targets:

1. Metadata query `p95 < 300ms`.
2. Lightweight action endpoints `p95 < 500ms`.
3. Access token and presign operations `p95 < 200ms`.

Workspace targets:

1. Image workspace usable in under 2 seconds.
2. Video frame navigation hot path under roughly 120ms target.
3. Frame list, object list, and timeline virtualization required.
4. Async autosave required.

Review targets:

1. Queue-to-next-item transitions under roughly 500ms target.

Evaluation targets:

1. Summary first, details progressively loaded.

### 14.3 Reliability And Recovery

1. Jobs and runs must be idempotent.
2. Workers must support heartbeat, lease, timeout recovery, and retries.
3. Publish actions must be atomic.
4. Upload flows must be retry-safe.
5. Errors must preserve diagnosable context.

### 14.4 Data Integrity And Reproducibility

1. Training, evaluation, and artifacts must bind to snapshots.
2. Horizontal comparison requires shared `benchmark_snapshot_id`.
3. Published annotations are immutable.
4. Snapshot integrity hash is required.
5. Ontology, calibration, and schema versions participate in lineage.
6. Results without required context are temporary, not trusted.

### 14.5 Security And Authorization

1. Default deny.
2. Audit required for publish, delete, review, and promotion.
3. Service account least privilege.
4. Guest TTL and scope constraints.
5. Plugin isolation from canonical writes.
6. Large-object access by presign or controlled gateway rather than API proxy.

### 14.6 Observability

The platform should observe:

1. API latency and error rate.
2. Queue backlog.
3. Worker heartbeat health.
4. Snapshot build duration.
5. Review backlog.
6. `Longest Idle Task`.
7. Training failure rate.
8. Invalid evaluation ratio.
9. Artifact revoke or deprecate counts.
10. Plugin health.

Alerts should point to a fixable object, not a vague symptom.

### 14.7 Frontend Quality Constraints

1. Version context must always be visible on decision pages.
2. Key pages must support deep-linking.
3. Workspace must be keyboard-first.
4. State changes must be explicit.
5. Reject requires structured reason code.
6. `Object Persistence Checker` must expose track continuity issues.
7. Blockers must come with handling links.

### 14.8 Technical Landing Guidance

The repository should evolve toward:

1. Go modular monolith for control plane.
2. Worker and connector-based execution plane.
3. `Vite + React + TypeScript` frontend.
4. Shared schema package for status, reason-code, and manifest contracts.
5. Canvas-based unified runtime for annotation and review workspaces.
6. URL search params as the main shareable context layer.

Suggested domain grouping:

1. `internal/foundation`
2. `internal/datahub`
3. `internal/tasks`
4. `internal/annotations`
5. `internal/review`
6. `internal/feedback`
7. `internal/training`
8. `internal/evaluation`
9. `internal/artifacts`
10. `internal/plugins`

Suggested frontend and SDK grouping:

1. `apps/web`
2. `packages/shared-schema`
3. `packages/sdk-ts`
4. `packages/sdk-py` or equivalent Python SDK directory

### 14.9 Key Technical Decisions To Lock Early

1. Go modular monolith for V1 control plane.
2. `Vite + React + TypeScript` frontend baseline.
3. Canvas Runtime for workspace rendering.
4. Virtual or CoW snapshot semantics.
5. Benchmark-bound evaluation comparison.
6. Structured feedback schema.
7. Strict plugin isolation.
8. Strict run-bound service-account permissions.

### 14.10 Primary Risks

Key risks include:

1. V1 scope explosion.
2. Building a module dashboard instead of a production system.
3. Video workspace performance failure.
4. Weak snapshot and lineage enforcement.
5. Overbuilding training connectors too early.
6. Superficial plugin abstraction that still hard-codes modality.
7. Publish power bypass.
8. Feedback bus becoming unstructured text.
9. Service-account privilege drift.
10. Underestimating on-prem and air-gapped adaptation.

### 14.11 Open Decisions To Confirm Before Detailed Planning

The framework leaves several tactical decisions open for implementation planning:

1. V1 auth baseline: local accounts only, or local plus OIDC.
2. Exact first training connectors: local shell and remote command should be first candidates.
3. V1 video execution strategy: video resource plus derived-frame workflow is recommended over full direct-stream dependence.
4. Recommended artifact approval topology: ML Engineer nomination plus Project Owner approval is recommended.
5. Evaluation execution ownership: platform-runner path plus external upload path is recommended.

### 14.12 Recommended ADR Topics

Suggested ADRs:

1. Why modular monolith for V1.
2. Why Task Overview is the default entry.
3. Why snapshot binding is mandatory.
4. Why evaluation requires shared benchmark snapshots.
5. Why workspace uses a Canvas Runtime.
6. Why URL params are the shareable context layer.
7. Why plugin isolation is mandatory.
8. Why service-account permissions are run-bound.

## 15. Conclusion

This framework defines the product as a visual AI production system rather than a collection of disconnected tools.

It establishes:

1. The product mission.
2. The module structure.
3. The authorization and resource model.
4. The workflow state machines.
5. The deployment shapes.
6. The page skeleton and role journeys.
7. The phased scope.
8. The non-functional and technical constraints.
9. The risks and decisions that must be controlled before implementation.

The next document should be the user-journey and interaction spec derived from this framework.
