package smoke_test

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestSmokeSkipsUpDevWhenDependenciesAreAlreadyReachable(t *testing.T) {
	fakeBin := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBin, "docker"), "#!/usr/bin/env bash\nexit 0\n")
	writeExecutable(t, filepath.Join(fakeBin, "make"), `#!/usr/bin/env bash
if [[ "$1" == "up-dev" ]]; then
  echo "unexpected up-dev" >&2
  exit 99
fi
if [[ "$1" == "migrate-up" ]]; then
  exit 0
fi
echo "unexpected make target: $*" >&2
exit 98
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), fakeCurlScript())
	writeExecutable(t, filepath.Join(fakeBin, "go"), fakeGoScript())

	cmd := exec.Command("bash", "scripts/dev/smoke.sh")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"API_BASE_URL=http://127.0.0.1:8080",
		"SMOKE_SKIP_PORT_CHECK=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("smoke script failed: %v\n%s", err, out)
	}
}

func TestMakeUpDevStopsWhenDockerComposeFails(t *testing.T) {
	fakeBin := t.TempDir()
	dockerLog := filepath.Join(t.TempDir(), "docker.log")
	goLog := filepath.Join(t.TempDir(), "go.log")

	writeExecutable(t, filepath.Join(fakeBin, "docker"), "#!/usr/bin/env bash\nprintf 'docker %s\\n' \"$*\" >> \"$DOCKER_LOG\"\nexit 42\n")
	writeExecutable(t, filepath.Join(fakeBin, "go"), "#!/usr/bin/env bash\nprintf 'go %s\\n' \"$*\" >> \"$GO_LOG\"\nexit 0\n")

	cmd := exec.Command("make", "up-dev")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DOCKER_LOG="+dockerLog,
		"GO_LOG="+goLog,
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected make up-dev to fail when docker compose fails, output=%s", out)
	}

	if goCalls, readErr := os.ReadFile(goLog); readErr == nil && len(strings.TrimSpace(string(goCalls))) > 0 {
		t.Fatalf("expected s3-bootstrap to be skipped when docker compose fails, got go calls:\n%s", string(goCalls))
	}
}

func TestSmokeExercisesImportExportResolveAndPull(t *testing.T) {
	fakeBin := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBin, "docker"), "#!/usr/bin/env bash\nexit 0\n")
	writeExecutable(t, filepath.Join(fakeBin, "make"), `#!/usr/bin/env bash
if [[ "$1" == "migrate-up" ]]; then
  exit 0
fi
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), fakeCurlScript())
	writeExecutable(t, filepath.Join(fakeBin, "go"), fakeGoScript())

	callLog := filepath.Join(t.TempDir(), "calls.log")
	cmd := exec.Command("bash", "scripts/dev/smoke.sh")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"API_BASE_URL=http://127.0.0.1:8080",
		"CALL_LOG="+callLog,
		"SMOKE_SKIP_PORT_CHECK=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("smoke script failed: %v\n%s", err, out)
	}

	callBytes, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	callText := string(callBytes)
	for _, fragment := range []string{
		"/v1/projects/1/tasks",
		"/v1/tasks/1/workspace",
		"/v1/tasks/1/workspace/draft",
		"/v1/tasks/1/workspace/submit",
		"/v1/datasets/1/snapshots",
		"go run ./cmd/dev-seed-artifact-smoke --dataset-id 1 --category-only",
		"/v1/jobs/1",
		"/v1/jobs/1/events",
		"/v1/review/candidates",
		"/v1/jobs/video-extract",
		"/v1/jobs/2",
		"/v1/jobs/2/events",
		"/v1/snapshots/1/import",
		"/v1/jobs/3",
		"/v1/jobs/3/events",
		"/v1/publish/candidates?project_id=1",
		"/v1/publish/batches",
		"/v1/publish/batches/71",
		"/v1/publish/batches/71/feedback",
		"/v1/publish/batches/71/review-approve",
		"/v1/publish/batches/71/owner-approve",
		"/v1/publish/batches/71/workspace",
		"/v1/publish/records/91",
		"go run ./cmd/dev-seed-artifact-smoke --dataset-id 1",
		"/v1/snapshots/1/export",
		"/v1/artifacts/resolve?dataset=smoke-dataset&format=yolo&version=v-smoke-1",
		"/metrics",
		"platform-cli pull --dataset smoke-dataset --format yolo --version v-smoke-1",
	} {
		if !strings.Contains(callText, fragment) {
			t.Fatalf("expected smoke script to call %s, got log:\n%s", fragment, callText)
		}
	}
	if got := strings.Count(callText, "/v1/tasks/1/workspace/submit"); got < 2 {
		t.Fatalf("expected smoke script to exercise duplicate workspace submit path, got %d submit calls in log:\n%s", got, callText)
	}
	if got := strings.Count(callText, "/v1/snapshots/1/export"); got < 2 {
		t.Fatalf("expected smoke script to exercise both coco and yolo export requests, got %d export calls in log:\n%s", got, callText)
	}
	if !strings.Contains(callText, `"format":"coco"`) {
		t.Fatalf("expected smoke script to exercise coco export path, got log:\n%s", callText)
	}
	if !strings.Contains(callText, `"format":"yolo"`) {
		t.Fatalf("expected smoke script to retain yolo export path, got log:\n%s", callText)
	}
}

func TestSmokeRecoversFromMissingFutureMigrationVersion(t *testing.T) {
	fakeBin := t.TempDir()
	writeExecutable(t, filepath.Join(fakeBin, "docker"), "#!/usr/bin/env bash\nexit 0\n")
	latestVersion := latestMigrationVersion(t)

	makeState := filepath.Join(t.TempDir(), "migrate-up.failed")
	writeExecutable(t, filepath.Join(fakeBin, "make"), `#!/usr/bin/env bash
if [[ "$1" == "migrate-up" ]]; then
  if [[ ! -f "$FAKE_MAKE_STATE" ]]; then
    printf '2026/03/30 no migration found for version 3: read down for version 3 .: file does not exist\n' >&2
    touch "$FAKE_MAKE_STATE"
    exit 1
  fi
  exit 0
fi
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), fakeCurlScript())
	writeExecutable(t, filepath.Join(fakeBin, "go"), `#!/usr/bin/env bash
if [[ -n "$CALL_LOG" ]]; then
  printf 'go %s\n' "$*" >> "$CALL_LOG"
fi
if [[ "$1" == "run" && "$2" == "./cmd/s3-bootstrap" ]]; then
  exit 0
fi
if [[ "$1" == "run" && "$2" == "./cmd/migrate" ]]; then
  if [[ "$3" == "-command" && "$4" == "force" && "$5" == "-force-version" && "$6" == "$FAKE_LATEST_MIGRATION_VERSION" ]]; then
    exit 0
  fi
  echo "unexpected migrate command: $*" >&2
  exit 95
fi
if [[ "$1" == "run" && "$2" == "./cmd/dev-seed-artifact-smoke" ]]; then
  if [[ "$3" == "--dataset-id" && "$5" == "--category-only" ]]; then
    printf '{"dataset_id":1,"category_seeded":true}\n'
    exit 0
  fi
  printf '{"dataset_id":1,"snapshot_id":1,"version":"v1"}\n'
  exit 0
fi
if [[ "$1" == "build" ]]; then
  out=""
  prev=""
  for arg in "$@"; do
    if [[ "$prev" == "-o" ]]; then
      out="$arg"
      break
    fi
    prev="$arg"
  done
  cat > "$out" <<'EOF'
#!/usr/bin/env bash
if [[ -n "$CALL_LOG" ]]; then
  printf 'platform-cli %s\n' "$*" >> "$CALL_LOG"
fi
version=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "--version" ]]; then
    version="$arg"
  fi
  prev="$arg"
done
mkdir -p "pulled-${version}/train/images" "pulled-${version}/train/labels"
printf 'train: ./train/images\nval: ./train/images\nnames:\n  - person\n' > "pulled-${version}/data.yaml"
printf '{"version":"%s","entries":[{"path":"train/images/a.jpg","checksum":"sha256:abc"},{"path":"train/labels/a.txt","checksum":"sha256:def"},{"path":"data.yaml","checksum":"sha256:ghi"}]}\n' "$version" > "pulled-${version}/manifest.json"
printf 'fake-image-a' > "pulled-${version}/train/images/a.jpg"
printf '0 0.5 0.5 0.2 0.2\n' > "pulled-${version}/train/labels/a.txt"
printf '{"artifact_id":5,"snapshot":"%s"}\n' "$version" > verify-report.json
EOF
  chmod +x "$out"
  exit 0
fi
echo "unexpected go command: $*" >&2
exit 96
`)

	callLog := filepath.Join(t.TempDir(), "calls.log")
	cmd := exec.Command("bash", "scripts/dev/smoke.sh")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"API_BASE_URL=http://127.0.0.1:8080",
		"CALL_LOG="+callLog,
		"FAKE_MAKE_STATE="+makeState,
		"FAKE_LATEST_MIGRATION_VERSION="+latestVersion,
		"SMOKE_SKIP_PORT_CHECK=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("smoke script failed: %v\n%s", err, out)
	}

	callBytes, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	if !strings.Contains(string(callBytes), "go run ./cmd/migrate -command force -force-version "+latestVersion) {
		t.Fatalf("expected smoke recovery to force current migration version, got log:\n%s", string(callBytes))
	}
}

func fakeCurlScript() string {
	return `#!/usr/bin/env bash
url=""
data=""
write_out=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "-d" || "$prev" == "--data" || "$prev" == "--data-raw" ]]; then
    data="$arg"
  fi
  if [[ "$prev" == "-w" || "$prev" == "--write-out" ]]; then
    write_out="$arg"
  fi
  if [[ "$arg" == http://* || "$arg" == https://* ]]; then
    url="$arg"
  fi
  prev="$arg"
done
if [[ -n "$CALL_LOG" && -n "$url" ]]; then
  if [[ -n "$data" ]]; then
    printf '%s DATA=%s\n' "$url" "$data" >> "$CALL_LOG"
  else
    printf '%s\n' "$url" >> "$CALL_LOG"
  fi
fi
status="200"
body=""
case "$url" in
  */healthz|*/readyz)
    body=""
    ;;
  */metrics)
    body=$'# TYPE yolo_http_requests_total counter\nyolo_http_requests_total{method="GET",route="/healthz",status_class="2xx"} 1\n# TYPE yolo_job_creations_total counter\nyolo_job_creations_total{job_type="zero-shot"} 1\n# TYPE yolo_queue_depth gauge\nyolo_queue_depth{lane="jobs:cpu"} 0\n# TYPE yolo_review_backlog gauge\nyolo_review_backlog 1\n'
    ;;
  */v1/datasets)
    body='{"dataset_id":1}'
    ;;
  */v1/datasets/1/snapshots)
    body='{"id":1,"dataset_id":1,"version":"v1"}'
    ;;
  */v1/projects/1/tasks)
    body='{"id":1,"project_id":1,"snapshot_id":1,"title":"Annotate smoke image","kind":"annotation","status":"in_progress","priority":"high","assignee":"annotator-1","asset_object_key":"train/a.jpg","media_kind":"image","ontology_version":"v1"}'
    ;;
  */v1/tasks/1/workspace/draft)
    body='{"task":{"id":1,"status":"in_progress","kind":"annotation","asset_object_key":"train/a.jpg","media_kind":"image"},"asset":{"dataset_id":1,"dataset_name":"smoke-dataset","snapshot_id":1,"snapshot_version":"v1","object_key":"train/a.jpg"},"draft":{"id":21,"task_id":1,"state":"draft","revision":1,"body":{"objects":[{"id":"box-1","label":"person"}]}}}'
    ;;
  */v1/tasks/1/workspace/submit)
    body='{"task":{"id":1,"status":"submitted","kind":"annotation","asset_object_key":"train/a.jpg","media_kind":"image"},"asset":{"dataset_id":1,"dataset_name":"smoke-dataset","snapshot_id":1,"snapshot_version":"v1","object_key":"train/a.jpg"},"draft":{"id":21,"task_id":1,"state":"submitted","revision":2,"body":{"objects":[{"id":"box-1","label":"person"}]}}}'
    ;;
  */v1/tasks/1/workspace)
    body='{"task":{"id":1,"status":"in_progress","kind":"annotation","asset_object_key":"train/a.jpg","media_kind":"image"},"asset":{"dataset_id":1,"dataset_name":"smoke-dataset","snapshot_id":1,"snapshot_version":"v1","object_key":"train/a.jpg"},"draft":{"task_id":1,"state":"draft","revision":0,"body":{}}}'
    ;;
  */v1/review/candidates)
    body='{"items":[{"id":301,"object_key":"train/a.jpg","status":"queued_for_review","source":{"job_id":1,"model_name":"grounding_dino_fake","is_pseudo":true}}]}'
    ;;
  */v1/snapshots/1/import)
    body='{"job_id":3,"status":"queued","dataset_id":1,"snapshot_id":1}'
    ;;
  */v1/jobs/1)
    body='{"id":1,"status":"succeeded","dataset_id":1,"snapshot_id":1,"result_type":"annotation_candidates","result_count":2}'
    ;;
  */v1/jobs/1/events)
    body='{"items":[{"event_type":"progress","detail_json":{"total_items":2,"succeeded_items":2,"failed_items":0}},{"event_type":"review_candidates_materialized","detail_json":{"result_type":"annotation_candidates","result_count":2}}]}'
    ;;
  */v1/jobs/2)
    body='{"id":2,"status":"succeeded","dataset_id":1,"result_type":"video_frames","result_count":7,"result_ref":{"result_type":"video_frames","result_count":7,"frames":[{"frame_index":0,"timestamp_ms":0,"object_key":"clips/a/frame-0000.jpg"},{"frame_index":6,"timestamp_ms":3000,"object_key":"clips/a/frame-0006.jpg"}]}}'
    ;;
  */v1/jobs/2/events)
    body='{"items":[{"event_type":"progress","detail_json":{"total_items":7,"succeeded_items":7,"failed_items":0}},{"event_type":"video_frames_materialized","detail_json":{"result_type":"video_frames","result_count":7}}]}'
    ;;
  */v1/jobs/3)
    body='{"id":3,"status":"succeeded","dataset_id":1,"snapshot_id":1}'
    ;;
  */v1/jobs/3/events)
    body='{"items":[{"event_type":"progress","detail_json":{"total_items":1,"succeeded_items":1,"failed_items":0}}]}'
    ;;
  */v1/publish/candidates*project_id=1)
    body='{"items":[]}'
    ;;
  */v1/publish/batches/71/feedback)
    body='{"id":2,"scope":"batch","stage":"review","action":"comment","reason_code":"smoke_ready","severity":"low","influence_weight":1,"comment":"smoke batch feedback"}'
    ;;
  */v1/publish/batches/71/review-approve)
    body='{"ok":true}'
    ;;
  */v1/publish/batches/71/owner-approve)
    body='{"publish_record_id":91}'
    ;;
  */v1/publish/batches/71/workspace)
    body='{"batch":{"id":71,"snapshot_id":1,"status":"published"},"items":[{"item_id":801,"candidate_id":401,"task_id":51,"overlay":{"boxes":[{"label":"car"}]},"diff":{"added":1,"updated":0,"removed":0},"feedback":[]}],"history":[{"stage":"review","actor":"reviewer-1","action":"approve"}]}'
    ;;
  */v1/publish/records/91)
    body='{"id":91,"publish_batch_id":71,"status":"published"}'
    ;;
  */v1/publish/batches/71)
    body='{"id":71,"project_id":1,"snapshot_id":1,"status":"draft","items":[{"id":801,"candidate_id":401,"task_id":51,"dataset_id":1,"snapshot_id":1,"item_payload":{"overlay":{"boxes":[{"label":"car"}]},"diff":{"added":1,"updated":0,"removed":0}}}],"feedback":[]}'
    ;;
  */v1/publish/batches)
    body='{"id":71,"project_id":1,"snapshot_id":1,"status":"draft","items":[{"id":801,"candidate_id":401,"task_id":51,"dataset_id":1,"snapshot_id":1,"item_payload":{"overlay":{"boxes":[{"label":"car"}]},"diff":{"added":1,"updated":0,"removed":0}}}],"feedback":[]}'
    ;;
  */scan)
    body='{"added_items":2}'
    ;;
  */items)
    body='{"items":[{"id":11,"object_key":"train/a.jpg"},{"id":12,"object_key":"train/b.jpg"}]}'
    ;;
  */v1/snapshots/1/export)
    if [[ "$data" == *'"format":"coco"'* ]]; then
      body='{"job_id":4,"artifact_id":4,"status":"pending"}'
    else
      body='{"job_id":5,"artifact_id":5,"status":"pending"}'
    fi
    ;;
  */v1/artifacts/4)
    body='{"id":4,"format":"coco","version":"v-smoke-1-coco","status":"ready"}'
    ;;
  */v1/artifacts/5)
    body='{"id":5,"format":"yolo","version":"v-smoke-1","status":"ready"}'
    ;;
  */v1/artifacts/resolve*dataset=smoke-dataset*format=yolo*version=v-smoke-1)
    body='{"id":5,"format":"yolo","version":"v-smoke-1","download_url":"http://127.0.0.1:8080/v1/artifacts/5/download"}'
    ;;
  */objects/presign)
    body='{"url":"http://signed.local/object"}'
    ;;
  */jobs/zero-shot)
    body='{"job_id":1,"status":"queued"}'
    ;;
  */jobs/video-extract)
    body='{"job_id":2,"status":"queued"}'
    ;;
  *)
    echo "unexpected curl url: $url" >&2
    exit 97
    ;;
esac
if [[ -n "$body" ]]; then
  printf '%s\n' "$body"
fi
if [[ -n "$write_out" ]]; then
  rendered="${write_out//\%\{http_code\}/$status}"
  printf '%b' "$rendered"
fi
`
}

func fakeGoScript() string {
	return `#!/usr/bin/env bash
if [[ -n "$CALL_LOG" ]]; then
  printf 'go %s\n' "$*" >> "$CALL_LOG"
fi
if [[ "$1" == "run" && "$2" == "./cmd/s3-bootstrap" ]]; then
  exit 0
fi
if [[ "$1" == "run" && "$2" == "./cmd/dev-seed-artifact-smoke" ]]; then
  if [[ "$3" == "--dataset-id" && "$5" == "--category-only" ]]; then
    printf '{"dataset_id":1,"category_seeded":true}\n'
    exit 0
  fi
  printf '{"dataset_id":1,"snapshot_id":1,"version":"v1"}\n'
  exit 0
fi
if [[ "$1" == "build" ]]; then
  out=""
  prev=""
  for arg in "$@"; do
    if [[ "$prev" == "-o" ]]; then
      out="$arg"
      break
    fi
    prev="$arg"
  done
  cat > "$out" <<'EOF'
#!/usr/bin/env bash
if [[ -n "$CALL_LOG" ]]; then
  printf 'platform-cli %s\n' "$*" >> "$CALL_LOG"
fi
version=""
prev=""
for arg in "$@"; do
  if [[ "$prev" == "--version" ]]; then
    version="$arg"
  fi
  prev="$arg"
done
mkdir -p "pulled-${version}/train/images" "pulled-${version}/train/labels"
printf 'train: ./train/images\nval: ./train/images\nnames:\n  - person\n' > "pulled-${version}/data.yaml"
printf '{"version":"%s","entries":[{"path":"train/images/a.jpg","checksum":"sha256:abc"},{"path":"train/labels/a.txt","checksum":"sha256:def"},{"path":"data.yaml","checksum":"sha256:ghi"}]}\n' "$version" > "pulled-${version}/manifest.json"
printf 'fake-image-a' > "pulled-${version}/train/images/a.jpg"
printf '0 0.5 0.5 0.2 0.2\n' > "pulled-${version}/train/labels/a.txt"
printf '{"artifact_id":5,"snapshot":"%s"}\n' "$version" > verify-report.json
EOF
  chmod +x "$out"
  exit 0
fi
echo "unexpected go command: $*" >&2
exit 96
`
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func latestMigrationVersion(t *testing.T) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot(t), "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	latest := 0
	for _, path := range paths {
		base := filepath.Base(path)
		underscore := strings.IndexByte(base, '_')
		if underscore <= 0 {
			continue
		}
		version, err := strconv.Atoi(base[:underscore])
		if err != nil {
			t.Fatalf("parse migration version from %s: %v", base, err)
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		t.Fatal("no migration versions found")
	}
	return strconv.Itoa(latest)
}

func skipIfSmokePortsUnavailable(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("bash smoke test is not supported on windows")
	}

	for _, addr := range []string{"127.0.0.1:5432", "127.0.0.1:6379", "127.0.0.1:9000"} {
		if !portReachable(addr) {
			t.Skipf("required dependency port %s is not reachable", addr)
		}
	}
}

func portReachable(addr string) bool {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
