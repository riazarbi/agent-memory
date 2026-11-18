package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type ciWorkflow struct {
	Jobs map[string]ciJob `yaml:"jobs"`
}

type ciJob struct {
	If          string            `yaml:"if"`
	Needs       []string          `yaml:"needs"`
	Permissions map[string]string `yaml:"permissions"`
	Steps       []struct {
		Uses string `yaml:"uses"`
		Run  string `yaml:"run"`
	} `yaml:"steps"`
}

// runs is every run script in the job, joined.
func (j ciJob) runs() string {
	var b strings.Builder
	for _, s := range j.Steps {
		b.WriteString(s.Run + "\n")
	}
	return b.String()
}

// Acceptance criterion 5: ci.yml cross-compiles in a build job, and a
// release job — only on a push to main, after every check, with
// contents: write — tags and publishes with the repo's release scripts. No
// Docker.
func TestCIReleaseJobs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf ciWorkflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"lint", "test", "tidy", "vuln", "build", "release"} {
		if _, ok := wf.Jobs[name]; !ok {
			t.Errorf("ci.yml has no %s job", name)
		}
	}

	if !strings.Contains(wf.Jobs["build"].runs(), "scripts/release/build-binaries.sh") {
		t.Error("the build job does not run scripts/release/build-binaries.sh")
	}

	checkReleaseJob(t, wf.Jobs["release"])

	lower := strings.ToLower(string(data))
	for _, banned := range []string{"docker", "ghcr.io"} {
		if strings.Contains(lower, banned) {
			t.Errorf("ci.yml mentions %q; CLI3 publishes no image", banned)
		}
	}
}

// checkReleaseJob: only on a push to main, after every check, allowed to
// write contents, using the repo's release scripts to tag and publish.
func checkReleaseJob(t *testing.T, rel ciJob) {
	t.Helper()
	cond := strings.Join(strings.Fields(rel.If), " ")
	for _, want := range []string{"github.event_name == 'push'", "github.ref == 'refs/heads/main'"} {
		if !strings.Contains(cond, want) {
			t.Errorf("release if: %q lacks %q", rel.If, want)
		}
	}
	if strings.Contains(cond, "||") || strings.Contains(cond, "pull_request") {
		t.Errorf("release if: %q could run off a push to main", rel.If)
	}
	needs := append([]string(nil), rel.Needs...)
	sort.Strings(needs)
	check(t, "release needs", needs, []string{"build", "lint", "test", "tidy", "vuln"})
	check(t, "release permissions", rel.Permissions, map[string]string{"contents": "write"})
	runs := rel.runs()
	for _, want := range []string{"scripts/release/next-tag.sh", "scripts/release/notes.sh", "scripts/release/build-binaries.sh", "gh release create", "checksums.txt", ".tar.gz"} {
		if !strings.Contains(runs, want) {
			t.Errorf("the release job does not use %q", want)
		}
	}
}

// Acceptance criterion 5: build-binaries.sh, run locally into a temp dir,
// produces agent-memory_<tag>_<os>_<arch>.tar.gz for the four targets,
// each holding that platform's binary, and a checksums.txt that matches.
func TestBuildBinaries(t *testing.T) {
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "dist")
	const tag = "20261118.2"
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "release", "build-binaries.sh"), tag, out) //nolint:gosec // the repo's own script
	cmd.Dir = root
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-binaries.sh: %v\n%s", err, b)
	}

	targets := map[string]func(t *testing.T, bin []byte){
		"linux_amd64":  elfMachine(elf.EM_X86_64),
		"linux_arm64":  elfMachine(elf.EM_AARCH64),
		"darwin_amd64": machoCPU(macho.CpuAmd64),
		"darwin_arm64": machoCPU(macho.CpuArm64),
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := make([]string, 0, len(targets)+1)
	want = append(want, "checksums.txt")
	for target := range targets {
		want = append(want, "agent-memory_"+tag+"_"+target+".tar.gz")
	}
	sort.Strings(want)
	check(t, "files", names, want)

	sums := readChecksums(t, filepath.Join(out, "checksums.txt"))
	check(t, "checksums.txt entries", len(sums), len(targets))
	for target, checkBin := range targets {
		name := "agent-memory_" + tag + "_" + target
		archive := filepath.Join(out, name+".tar.gz")
		data, err := os.ReadFile(archive)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		check(t, name+" checksum", sums[name+".tar.gz"], hex.EncodeToString(sum[:]))
		checkBin(t, untar(t, archive, name+"/agent-memory"))
	}
}

func readChecksums(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // a file this test created
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	sums := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			t.Fatalf("checksums.txt line %q, want <sha256> <file>", sc.Text())
		}
		sums[fields[1]] = fields[0]
	}
	return sums
}

// untar returns the named regular file's content from a .tar.gz.
func untar(t *testing.T, archive, name string) []byte {
	t.Helper()
	f, err := os.Open(archive) //nolint:gosec // a file this test created
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("%s holds no %s", filepath.Base(archive), name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == name && h.Typeflag == tar.TypeReg {
			b, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
	}
}

func elfMachine(want elf.Machine) func(*testing.T, []byte) {
	return func(t *testing.T, bin []byte) {
		t.Helper()
		f, err := elf.NewFile(bytes.NewReader(bin))
		if err != nil {
			t.Fatalf("not an ELF binary: %v", err)
		}
		check(t, "ELF machine", f.Machine, want)
	}
}

func machoCPU(want macho.Cpu) func(*testing.T, []byte) {
	return func(t *testing.T, bin []byte) {
		t.Helper()
		f, err := macho.NewFile(bytes.NewReader(bin))
		if err != nil {
			t.Fatalf("not a Mach-O binary: %v", err)
		}
		check(t, "Mach-O CPU", f.Cpu, want)
	}
}
