package smoke_test

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSmokeSkipsUpDevWhenDependenciesAreAlreadyReachable(t *testing.T) {
	skipIfSmokePortsUnavailable(t)

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
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("smoke script failed: %v\n%s", err, out)
	}
}

func TestSmokeExercisesArtifactBuildAndPull(t *testing.T) {
	skipIfSmokePortsUnavailable(t)

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

	cmd := exec.Command("bash", "scripts/dev/smoke.sh")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"API_BASE_URL=http://127.0.0.1:8080",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("smoke script failed: %v\n%s", err, out)
	}
}

func fakeCurlScript() string {
	return `#!/usr/bin/env bash
url=""
for arg in "$@"; do
  if [[ "$arg" == http://* || "$arg" == https://* ]]; then
    url="$arg"
  fi
done
case "$url" in
  */healthz|*/readyz)
    exit 0
    ;;
  */v1/datasets)
    printf '{"dataset_id":1}\n'
    ;;
  */scan)
    printf '{"added_items":2}\n'
    ;;
  */items)
    printf '{"items":[{"object_key":"train/a.jpg"}]}\n'
    ;;
  */objects/presign)
    printf '{"url":"http://signed.local/object"}\n'
    ;;
  */jobs/zero-shot)
    printf '{"job_id":1}\n'
    ;;
  */v1/artifacts/packages)
    printf '{"artifact_id":7,"status":"pending"}\n'
    ;;
  */v1/artifacts/7)
    printf '{"id":7,"status":"ready","format":"yolo","version":"v-smoke-1"}\n'
    ;;
  *)
    echo "unexpected curl url: $url" >&2
    exit 97
    ;;
esac
`
}

func fakeGoScript() string {
	return `#!/usr/bin/env bash
if [[ "$1" == "run" && "$2" == "./cmd/s3-bootstrap" ]]; then
  exit 0
fi
if [[ "$1" == "run" && "$2" == "./cmd/dev-seed-artifact-smoke" ]]; then
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
printf 'fake-image-a' > "pulled-${version}/train/images/a.jpg"
printf '0 0.5 0.5 0.2 0.2\n' > "pulled-${version}/train/labels/a.txt"
printf '{"artifact_id":7,"snapshot":"%s"}\n' "$version" > verify-report.json
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
