package smoke_test

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSmokeSkipsUpDevWhenDependenciesAreAlreadyReachable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash smoke test is not supported on windows")
	}

	for _, addr := range []string{"127.0.0.1:5432", "127.0.0.1:6379", "127.0.0.1:9000"} {
		if !portReachable(addr) {
			t.Skipf("required dependency port %s is not reachable", addr)
		}
	}

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
	writeExecutable(t, filepath.Join(fakeBin, "platform-cli"), `#!/usr/bin/env bash
if [[ -n "$CALL_LOG" ]]; then
  printf 'platform-cli %s\n' "$*" >> "$CALL_LOG"
fi
mkdir -p "pulled-v1/labels"
printf '{"artifact_id":5,"snapshot":"v1","total_files":1,"failed_files":0}' > "verify-report.json"
printf '{"version":"v1","entries":[{"path":"labels/0001.txt","checksum":"abc123"}]}' > "pulled-v1/manifest.json"
printf '0 0.5 0.5 0.2 0.2\n' > "pulled-v1/labels/0001.txt"
exit 0
`)
	writeExecutable(t, filepath.Join(fakeBin, "curl"), `#!/usr/bin/env bash
url=""
for arg in "$@"; do
  if [[ "$arg" == http://* || "$arg" == https://* ]]; then
    url="$arg"
  fi
done
if [[ -n "$CALL_LOG" && -n "$url" ]]; then
  printf '%s\n' "$url" >> "$CALL_LOG"
fi
case "$url" in
  */healthz|*/readyz)
    exit 0
    ;;
  */v1/datasets)
    printf '{"dataset_id":1}\n'
    ;;
  */v1/datasets/1/snapshots)
    printf '{"id":1,"dataset_id":1,"version":"v1"}\n'
    ;;
  */v1/snapshots/1/import)
    printf '{"job_id":3,"status":"queued","dataset_id":1,"snapshot_id":1}\n'
    ;;
  */v1/jobs/3)
    printf '{"id":3,"status":"succeeded","dataset_id":1,"snapshot_id":1}\n'
    ;;
  */scan)
    printf '{"added_items":2}\n'
    ;;
  */items)
    printf '{"items":[{"object_key":"train/a.jpg"}]}\n'
    ;;
  */v1/snapshots/1/export)
    printf '{"job_id":2,"artifact_id":5,"status":"queued"}\n'
    ;;
  */v1/artifacts/5)
    printf '{"id":5,"format":"yolo","version":"v1","status":"ready","entries":[]}\n'
    ;;
  */v1/artifacts/resolve?dataset=smoke-dataset&format=yolo&version=v1)
    printf '{"id":5,"format":"yolo","version":"v1"}\n'
    ;;
  */objects/presign)
    printf '{"url":"http://signed.local/object"}\n'
    ;;
  */jobs/zero-shot)
    printf '{"job_id":1}\n'
    ;;
  *)
    echo "unexpected curl url: $url" >&2
    exit 97
    ;;
esac
`)

	callLog := filepath.Join(t.TempDir(), "calls.log")
	cmd := exec.Command("bash", "scripts/dev/smoke.sh")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"API_BASE_URL=http://127.0.0.1:8080",
		"CALL_LOG="+callLog,
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
		"/v1/datasets/1/snapshots",
		"/v1/snapshots/1/import",
		"/v1/jobs/3",
		"/v1/snapshots/1/export",
		"/v1/artifacts/resolve?dataset=smoke-dataset&format=yolo&version=v1",
		"platform-cli pull --dataset smoke-dataset --format yolo --version v1",
	} {
		if !strings.Contains(callText, fragment) {
			t.Fatalf("expected smoke script to call %s, got log:\n%s", fragment, callText)
		}
	}
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

func portReachable(addr string) bool {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
