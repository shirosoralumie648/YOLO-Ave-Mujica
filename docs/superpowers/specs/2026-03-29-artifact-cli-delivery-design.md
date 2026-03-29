# Artifact And CLI Delivery Design

- Date: 2026-03-29
- Scope: Complete the artifact packaging and CLI delivery path for the MVP branch
- Status: Drafted from approved design discussion
- Owner: Platform team

## 1. Objective

Turn the current artifact metadata placeholder flow into a real training-package delivery path that exports canonical annotations as a YOLO training bundle, persists artifact metadata, stores generated outputs behind a storage adapter, and allows `platform-cli pull` to download, unpack, and verify the artifact end to end.

This design covers one focused sub-project only:

1. Export canonical annotations for a dataset snapshot into a real YOLO training package.
2. Persist artifact metadata and lifecycle state instead of relying on in-memory runtime state.
3. Support storage through an S3-shaped abstraction with a local filesystem adapter for development.
4. Deliver artifacts through `platform-cli pull` using a real package download path and checksum verification.

It does not attempt to solve candidate review persistence or worker-side model execution in the same iteration.

## 2. Scope And Constraints

### 2.1 In Scope

1. `POST /v1/artifacts/packages` creates a real artifact for `format=yolo`.
2. Export source data comes from canonical `annotations`, `categories`, `dataset_items`, and `dataset_snapshots`.
3. Effective annotations are computed using snapshot interval semantics:
   - `created_at_snapshot_id <= target_snapshot_id`
   - and (`deleted_at_snapshot_id is null` or `deleted_at_snapshot_id > target_snapshot_id`)
4. The package contains real image files, YOLO label files, `data.yaml`, and `manifest.json`.
5. The build output includes both:
   - a materialized directory tree
   - a `package.yolo.tar.gz` archive
6. Artifact metadata is stored durably and exposed through existing artifact APIs.
7. CLI pull resolves a ready artifact, downloads the archive, unpacks it, verifies files against the manifest, and writes `verify-report.json`.

### 2.2 Out Of Scope

1. COCO export or any non-YOLO format.
2. Candidate-label export from `annotation_candidates`.
3. Automatic train/val/test split generation.
4. Full workerization of packaging as a background compute pipeline.
5. Broad review or annotation-production changes outside what is needed to read canonical annotations.

### 2.3 Working Constraints

1. Existing in-memory helpers may remain for focused tests, but runtime behavior must no longer depend on in-memory artifact state.
2. The external artifact contract must remain stable enough to support a later migration from filesystem-backed storage to MinIO/S3-backed storage.
3. The MVP branch must continue to run locally with PostgreSQL, Redis, and MinIO-compatible dependencies, but development cannot depend on a fully implemented object-storage artifact path on day one.
4. The implementation must preserve current route shape and improve behavior underneath it.

## 3. Chosen Approach

### 3.1 Recommended Approach

Use a minimal-intrusion export path with five focused layers:

1. `ExportQuery` reads canonical snapshot data from PostgreSQL.
2. `ArtifactBuilder` creates a real YOLO package directory and archive from the query result.
3. `ArtifactStorage` writes generated outputs through a storage abstraction.
4. `ArtifactRepository` persists artifact metadata and lifecycle status.
5. `CLI Pull` resolves, downloads, unpacks, and verifies the exported archive.

This approach is intentionally narrower than also solving candidate review or generalized worker execution. It makes the artifact path real without widening the current sub-project beyond control.

### 3.2 Why This Approach

1. It turns the most user-visible gap into a working feature without entangling multiple unfinished subsystems.
2. It keeps the export query logic separate from package generation and storage concerns, which reduces future migration cost.
3. It preserves the intended architecture shape: control plane coordinates, storage abstraction hides runtime backend choice, CLI depends on stable HTTP contracts.

## 4. Architecture

### 4.1 Export Query Layer

Add a focused query path under the Artifacts module that produces a normalized export model from:

1. `datasets`
2. `dataset_items`
3. `dataset_snapshots`
4. `annotations`
5. `categories`

Responsibilities:

1. Validate that the requested dataset and snapshot exist and match.
2. Load dataset item object keys for the target dataset.
3. Load effective canonical annotations for the target snapshot using interval filtering.
4. Load category names in a stable order for YOLO class index generation.
5. Return a normalized export payload suitable for deterministic package generation.

This layer must not build files, fetch object bytes, or persist artifact rows.

### 4.2 Artifact Builder Layer

The builder converts the normalized export model into a train-ready YOLO package.

Responsibilities:

1. Create a stable workspace directory for one export build.
2. Download real image bytes for every exported dataset item through the configured source adapter.
3. Emit one label file per image using YOLO text format.
4. Emit an empty label file when an image has no effective annotations in the target snapshot.
5. Generate `data.yaml`.
6. Generate `manifest.json` with per-file checksum metadata.
7. Produce `package.yolo.tar.gz`.

Directory shape for the first version:

```text
<package-root>/
  images/
    <original or normalized file names>
  labels/
    <matching stems>.txt
  data.yaml
  manifest.json
```

No automatic train/val split is introduced in this iteration. `data.yaml` should point at the generated `images/` directory in a stable, internally consistent way for the extracted package.

### 4.3 Artifact Storage Layer

Introduce a storage abstraction with S3-style intent and a filesystem development adapter.

The interface should support:

1. Writing the package archive.
2. Writing the manifest.
3. Optionally writing or retaining the materialized directory tree.
4. Returning stable URIs or locator strings for stored outputs.
5. Opening a stored package for CLI download.

Runtime expectations:

1. The interface shape should be compatible with object storage usage.
2. The first concrete implementation may store outputs on the local filesystem for development.
3. Production-style URI fields in artifact metadata should remain explicit rather than leaking local-only path assumptions into handlers or CLI logic.

### 4.4 Artifact Metadata Repository

Replace the current runtime-only in-memory artifact repository path with a PostgreSQL-backed runtime implementation.

Artifact metadata must include at minimum:

1. `dataset_id`
2. `snapshot_id`
3. `format`
4. `version`
5. `uri`
6. `manifest_uri`
7. `checksum`
8. `status`
9. `created_at`

Lifecycle states for this iteration:

1. `pending`
2. `ready`
3. `failed`

`resolve` must only return artifacts in `ready` state.

### 4.5 CLI Pull Pipeline

`platform-cli pull` should become a real delivery path.

Flow:

1. Resolve artifact by `format` and `version`.
2. Download the package archive from a dedicated artifact download endpoint or presigned URL path.
3. Unpack the archive into `pulled-<version>/`.
4. Read `manifest.json` from the extracted package.
5. Verify extracted files against manifest checksums.
6. Write `verify-report.json` with verification summary and environment context.

The CLI should not construct artifact storage paths by itself. It must depend on server-provided metadata and download URLs.

## 5. HTTP And Data Flow

### 5.1 Package Creation

`POST /v1/artifacts/packages`

1. Validate request fields.
2. Create artifact metadata row in `pending`.
3. Run export build for the target dataset snapshot.
4. Store directory tree, manifest, and package archive through `ArtifactStorage`.
5. Update artifact metadata with final URIs, checksum, and `ready` status.
6. Return `artifact_id` and a stable response shape compatible with later background execution.

This iteration may execute the build synchronously inside the request path, while still preserving an async-shaped response contract for future workerization.

### 5.2 Artifact Resolve

`GET /v1/artifacts/resolve?format=yolo&version=vX`

1. Lookup artifact metadata by `format + version`.
2. Only return metadata for artifacts in `ready`.
3. Return enough information for the CLI to obtain the archive download path.

### 5.3 Artifact Download

The server must expose a concrete way for CLI to fetch the package archive.

Acceptable first-iteration choices:

1. A direct API download endpoint that streams the archive from storage.
2. A presigned-style artifact URL generated from the storage adapter.

The chosen method must work with the filesystem adapter in development and remain compatible with future MinIO/S3-backed delivery.

## 6. Export Semantics

### 6.1 Annotation Selection

Only canonical annotations are exported.

For the target snapshot, include an annotation when:

1. It belongs to the requested dataset.
2. Its `created_at_snapshot_id` is less than or equal to the target snapshot.
3. Its `deleted_at_snapshot_id` is null or greater than the target snapshot.
4. Its category resolves to a known canonical category row.

Candidate labels are explicitly excluded from this iteration.

### 6.2 Category Ordering

The export must produce deterministic YOLO class indexes.

Recommended rule:

1. Order categories by ascending category ID for the dataset's project.
2. Emit category names in that order into `data.yaml`.
3. Map annotation category IDs to zero-based positions in that ordered list.

### 6.3 Label File Semantics

For each exported image:

1. Create `labels/<stem>.txt`.
2. If the image has effective annotations, write one YOLO line per annotation.
3. If the image has no effective annotations, create an empty file.

### 6.4 Image File Semantics

The package must contain real image bytes copied from the dataset object source.

The first iteration may:

1. Preserve source filenames where practical.
2. Normalize destination names only as needed to avoid collisions.

If collisions are possible, the chosen naming rule must be deterministic and reflected in both manifest and label mapping.

## 7. Error Handling

Failures must be explicit and diagnosable.

### 7.1 API Errors

1. Validation failures return `400`.
2. Dataset or snapshot not found returns `404`.
3. Artifact not ready or not found on resolve/pull returns `404` or a clear non-success status, depending on endpoint semantics.
4. Build/storage failures return `500` and must record artifact state as `failed` when an artifact row has already been created.

### 7.2 Build Failure Sources

At minimum, errors should distinguish:

1. export query failure
2. source image fetch failure
3. YOLO label generation failure
4. manifest generation failure
5. archive generation failure
6. storage write failure
7. metadata persistence failure

### 7.3 CLI Failures

`platform-cli pull` must fail by default when:

1. artifact resolution fails
2. archive download fails
3. archive extraction fails
4. manifest is missing or malformed
5. checksum verification fails

`--allow-partial` may permit completion when extracted files exist but one or more checksum validations fail. It must not suppress artifact-resolution, download, or extraction failures.

## 8. Testing And Acceptance

### 8.1 Automated Tests

Add or update tests for:

1. export query correctness for effective annotations at a target snapshot
2. deterministic category ordering
3. YOLO label generation
4. empty label file generation for unannotated images
5. `data.yaml` generation
6. `manifest.json` checksum coverage
7. archive generation and extraction
8. artifact metadata persistence and resolve behavior
9. CLI download, unpack, and verify flow

### 8.2 Integration Coverage

Local integration should prove:

1. canonical annotation rows can produce a real artifact
2. the artifact reaches `ready`
3. CLI can resolve and download the artifact
4. the extracted output contains real images, real labels, `data.yaml`, and `manifest.json`
5. `verify-report.json` reflects the actual verification outcome

### 8.3 Local Smoke Extension

Extend the local smoke path to cover the artifact main path:

1. prepare minimal canonical annotation data for a dataset snapshot
2. create an artifact package
3. resolve the artifact
4. pull the artifact with CLI
5. assert extracted package contents exist locally

### 8.4 Definition Of Done

This sub-project is done when:

1. artifact creation produces a real YOLO training package from canonical annotations
2. artifact metadata is persisted and resolvable in runtime
3. CLI pull downloads and extracts the real package
4. checksums are verified against `manifest.json`
5. local smoke covers the end-to-end artifact delivery path
6. existing tests and new tests pass together

## 9. Implementation Order

Recommended execution order:

1. Introduce export query and snapshot-effective annotation tests.
2. Introduce deterministic YOLO package builder tests.
3. Add runtime artifact repository persistence.
4. Add storage abstraction and filesystem adapter.
5. Wire package creation to build and persist real artifacts.
6. Add real package download support for CLI.
7. Update CLI pull to download, extract, and verify real packages.
8. Extend smoke coverage for the artifact path.
