package installscript

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const scriptPath = "../../site/static/install.sh"

// release is a fake GitHub release server. It answers the two URL shapes the
// script uses — latest/download/<asset> and download/<tag>/<asset> — for every
// platform, with a SHA256SUMS listing each archive the way the real release
// workflow writes it.
type release struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
}

type releaseOptions struct {
	tamper   bool // serve archives whose bytes differ from the listed hash
	unlisted bool // omit archive lines from SHA256SUMS
	broken   bool // ship a snoop that cannot start
	// noexec ships a snoop that cannot start from anywhere under this directory,
	// as a binary can't on a filesystem mounted noexec.
	noexec string
}

var platforms = []string{"linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64"}

func newRelease(t *testing.T, version string, opts releaseOptions) *release {
	t.Helper()
	files := map[string][]byte{}
	var sums strings.Builder
	for _, p := range platforms {
		archive := stubArchive(t, version, opts)
		sum := sha256.Sum256(archive)
		for _, name := range []string{"snoop_" + p + ".tar.gz", "snoop_" + version + "_" + p + ".tar.gz"} {
			if !opts.unlisted {
				fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(sum[:]), name)
			}
			served := archive
			if opts.tamper {
				served = append(append([]byte{}, archive...), 0)
			}
			files[name] = served
		}
	}
	files["SHA256SUMS"] = []byte(sums.String())

	r := &release{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.requests = append(r.requests, req.URL.Path)
		r.mu.Unlock()
		name := ""
		switch {
		case strings.HasPrefix(req.URL.Path, "/latest/download/"):
			name = strings.TrimPrefix(req.URL.Path, "/latest/download/")
		case strings.HasPrefix(req.URL.Path, "/download/"+version+"/"):
			name = strings.TrimPrefix(req.URL.Path, "/download/"+version+"/")
		}
		body, ok := files[name]
		if !ok {
			http.NotFound(w, req)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *release) paths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

// stubSnoop stands in for the snoop binary: it prints what snoop --version
// prints.
const stubSnoop = `#!/bin/sh
echo 'snoop VERSION'
`

// stubArchive builds snoop/snoop and snoop/LICENSE, gzipped, in the layout
// scripts/build_binaries.sh produces. A broken archive's snoop exits 1, and a
// noexec one's fails as exec does on a noexec mount when run from under it.
func stubArchive(t *testing.T, version string, opts releaseOptions) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name string, mode int64, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	body := strings.ReplaceAll(stubSnoop, "VERSION", version)
	if opts.broken {
		body = "#!/bin/sh\necho 'snoop: cannot start' >&2\nexit 1\n"
	}
	if opts.noexec != "" {
		body = strings.Replace(body, "#!/bin/sh\n", "#!/bin/sh\n"+
			`case "$(pwd -P)/" in '`+opts.noexec+`'/*) echo "./snoop: Permission denied" >&2; exit 126 ;; esac`+"\n", 1)
	}
	add("snoop/snoop", 0o755, body)
	add("snoop/LICENSE", 0o644, "MIT\n")
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// declaredTools is every external command the script may call. A run's PATH
// holds only these, so a script that quietly starts depending on another tool
// fails here rather than on a machine without it. gzip is on the list because
// GNU tar execs it for -z.
var declaredTools = []string{"curl", "wget", "tar", "gzip", "uname", "mktemp", "mkdir", "cp", "chmod", "mv", "rm",
	"env", "sha256sum", "shasum", "sysctl"}

// toolbox returns a directory of symlinks to the declared tools that exist on
// this machine, minus any named in omit.
func toolbox(t *testing.T, omit ...string) string {
	t.Helper()
	dir := t.TempDir()
	skip := map[string]bool{}
	for _, name := range omit {
		skip[name] = true
	}
	for _, name := range declaredTools {
		if skip[name] {
			continue
		}
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if err := os.Symlink(path, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fakeCommand writes an executable sh script called name into dir.
func fakeCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

type result struct {
	stdout, stderr string
	code           int
	home           string
}

// runScript runs script under /bin/sh with a clean environment: HOME is a
// fresh temp dir, PATH is exactly pathDirs, and env adds to both.
func runScript(t *testing.T, script string, r *release, env map[string]string, pathDirs ...string) result {
	t.Helper()
	return runScriptIn(t, "", script, r, env, pathDirs...)
}

// runScriptIn is runScript from the working directory cwd.
func runScriptIn(t *testing.T, cwd, script string, r *release, env map[string]string, pathDirs ...string) result {
	t.Helper()
	home := t.TempDir()
	abs, err := filepath.Abs(script)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", abs)
	cmd.Dir = cwd
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + strings.Join(pathDirs, string(os.PathListSeparator)),
		"SNOOP_INSTALL_BASE_URL=" + r.URL,
	}
	// A shell exports PWD, and pwd reports the directory as it was reached
	// (/var/..., not /private/var/... on macOS), so the harness does too.
	if cwd != "" {
		cmd.Env = append(cmd.Env, "PWD="+cwd)
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+strings.ReplaceAll(v, "$HOME", home))
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", script, err)
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), code: code, home: home}
}

// hostPlatform is the snoop_<os>_<arch> suffix the script should pick on this
// machine when uname is not faked.
func hostPlatform(t *testing.T) string {
	t.Helper()
	switch runtime.GOOS + "_" + runtime.GOARCH {
	case "linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64":
		return runtime.GOOS + "_" + runtime.GOARCH
	}
	t.Skipf("no snoop build for %s/%s", runtime.GOOS, runtime.GOARCH)
	return ""
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s exists, want nothing installed", path)
	}
}

func TestInstallsNewestIntoLocalBin(t *testing.T) {
	p := hostPlatform(t)
	r := newRelease(t, "v0.7.0", releaseOptions{})
	res := runScript(t, scriptPath, r, nil, toolbox(t))
	if res.code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
	bin := filepath.Join(res.home, ".local", "bin", "snoop")
	out, err := exec.Command(bin, "--version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "snoop v0.7.0" {
		t.Fatalf("installed %s --version = %q, %v", bin, out, err)
	}
	if want := "✓ Installed snoop v0.7.0 to " + bin; !strings.Contains(res.stdout, want) {
		t.Errorf("stdout = %q, want %q", res.stdout, want)
	}
	for _, want := range []string{"/latest/download/snoop_" + p + ".tar.gz", "/latest/download/SHA256SUMS"} {
		if !contains(r.paths(), want) {
			t.Errorf("requests = %v, want %s", r.paths(), want)
		}
	}
}

func TestPinnedVersion(t *testing.T) {
	p := hostPlatform(t)
	for _, spelled := range []string{"v0.7.0", "0.7.0"} {
		t.Run(spelled, func(t *testing.T) {
			r := newRelease(t, "v0.7.0", releaseOptions{})
			res := runScript(t, scriptPath, r, map[string]string{"SNOOP_VERSION": spelled}, toolbox(t))
			if res.code != 0 {
				t.Fatalf("exit %d, stderr: %s", res.code, res.stderr)
			}
			for _, want := range []string{"/download/v0.7.0/snoop_v0.7.0_" + p + ".tar.gz", "/download/v0.7.0/SHA256SUMS"} {
				if !contains(r.paths(), want) {
					t.Errorf("requests = %v, want %s", r.paths(), want)
				}
			}
			for _, got := range r.paths() {
				if strings.HasPrefix(got, "/latest/") {
					t.Errorf("pinned run requested %s", got)
				}
			}
		})
	}
}

func TestUnknownVersionNamesTheURL(t *testing.T) {
	hostPlatform(t)
	r := newRelease(t, "v0.7.0", releaseOptions{})
	res := runScript(t, scriptPath, r, map[string]string{"SNOOP_VERSION": "v9.9.9"}, toolbox(t))
	if res.code == 0 || !strings.Contains(res.stderr, r.URL+"/download/v9.9.9/") {
		t.Fatalf("exit %d, stderr %q: want a failure naming the URL", res.code, res.stderr)
	}
	mustNotExist(t, filepath.Join(res.home, ".local", "bin", "snoop"))
}

func TestHonorsInstallDir(t *testing.T) {
	hostPlatform(t)
	r := newRelease(t, "v0.7.0", releaseOptions{})
	dir := filepath.Join(t.TempDir(), "not", "yet", "there")
	res := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir}, toolbox(t))
	if res.code != 0 {
		t.Fatalf("exit %d, stderr: %s", res.code, res.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "snoop")); err != nil {
		t.Errorf("want snoop in %s: %v", dir, err)
	}
	mustNotExist(t, filepath.Join(res.home, ".local", "bin", "snoop"))
}

func TestUnwritableInstallDir(t *testing.T) {
	hostPlatform(t)
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	r := newRelease(t, "v0.7.0", releaseOptions{})
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	res := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir}, toolbox(t))
	if res.code == 0 || !strings.Contains(res.stderr, "SNOOP_INSTALL_DIR") {
		t.Fatalf("exit %d, stderr %q: want a refusal suggesting SNOOP_INSTALL_DIR", res.code, res.stderr)
	}
	mustNotExist(t, filepath.Join(dir, "snoop"))
}

func TestReplacesAnExistingInstall(t *testing.T) {
	hostPlatform(t)
	dir := t.TempDir()
	fakeCommand(t, dir, "snoop", "echo 'snoop v0.6.0'")
	r := newRelease(t, "v0.7.0", releaseOptions{})
	res := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir}, toolbox(t))
	if res.code != 0 {
		t.Fatalf("exit %d, stderr: %s", res.code, res.stderr)
	}
	out, _ := exec.Command(filepath.Join(dir, "snoop"), "--version").Output()
	if strings.TrimSpace(string(out)) != "snoop v0.7.0" {
		t.Errorf("after reinstall snoop --version = %q, want snoop v0.7.0", out)
	}
}

// Overwriting a running executable in place fails on Linux with "text file
// busy"; the script installs through a temp file and a rename, which does not.
// A shell-script stub can't show this — the kernel only guards mapped
// binaries — so this compiles a real one and keeps it running.
func TestReplacesARunningSnoop(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("text file busy is a Linux behavior")
	}
	hostPlatform(t)
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"),
		[]byte("package main\n\nimport \"time\"\n\nfunc main() { time.Sleep(time.Minute) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module sleeper\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(dir, "snoop"), ".")
	build.Dir = src
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the stand-in snoop: %v\n%s", err, out)
	}
	running := exec.Command(filepath.Join(dir, "snoop"))
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { running.Process.Kill(); running.Wait() })

	r := newRelease(t, "v0.7.0", releaseOptions{})
	res := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir}, toolbox(t))
	if res.code != 0 {
		t.Fatalf("install over a running snoop: exit %d, stderr: %s", res.code, res.stderr)
	}
	out, _ := exec.Command(filepath.Join(dir, "snoop"), "--version").Output()
	if strings.TrimSpace(string(out)) != "snoop v0.7.0" {
		t.Errorf("after install snoop --version = %q, want snoop v0.7.0", out)
	}
}

// A snoop that cannot start must not replace one that can. The check that the
// new binary runs happens before it is moved into place, so a damaged download
// or a misdetected architecture leaves the working install alone.
func TestRefusesABinaryThatDoesNotRun(t *testing.T) {
	hostPlatform(t)
	dir := t.TempDir()
	fakeCommand(t, dir, "snoop", "echo 'snoop v0.6.0'")
	r := newRelease(t, "v0.7.0", releaseOptions{broken: true})
	res := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir}, toolbox(t))
	if res.code == 0 || !strings.Contains(res.stderr, "cannot start") {
		t.Fatalf("exit %d, stderr %q: want a refusal carrying snoop's own error", res.code, res.stderr)
	}
	out, _ := exec.Command(filepath.Join(dir, "snoop"), "--version").Output()
	if strings.TrimSpace(string(out)) != "snoop v0.6.0" {
		t.Errorf("existing snoop now reports %q, want the working v0.6.0 left in place", out)
	}
	assertNothingStaged(t, dir)
}

// The check that the binary runs is made on the copy staged in the install
// directory, not in the download's temporary one: on a host that mounts /tmp
// noexec, nothing can run from there, and every install would be refused as a
// snoop that "did not run here".
func TestChecksTheBinaryWhereItWillRun(t *testing.T) {
	hostPlatform(t)
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	r := newRelease(t, "v0.7.0", releaseOptions{noexec: tmp})
	res := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir, "TMPDIR": tmp}, toolbox(t))
	if res.code != 0 {
		t.Fatalf("exit %d, stderr %q: want an install, with only the temporary directory noexec",
			res.code, res.stderr)
	}
	out, _ := exec.Command(filepath.Join(dir, "snoop"), "--version").Output()
	if strings.TrimSpace(string(out)) != "snoop v0.7.0" {
		t.Errorf("after install snoop --version = %q, want snoop v0.7.0", out)
	}
	assertNothingStaged(t, dir)
}

// assertNothingStaged checks dir holds no copy left staged for the rename.
func assertNothingStaged(t *testing.T, dir string) {
	t.Helper()
	staged, err := filepath.Glob(filepath.Join(dir, ".snoop.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != 0 {
		t.Errorf("%s holds %q, want no staged copy left behind", dir, staged)
	}
}

// A relative SNOOP_INSTALL_DIR is resolved against where the script ran, and the
// PATH hint names the absolute directory: a relative PATH entry would run
// whatever sits in ./bin of the current directory. A trailing slash on a
// directory already on PATH is still recognized as on PATH.
func TestResolvesTheInstallDir(t *testing.T) {
	hostPlatform(t)
	r := newRelease(t, "v0.7.0", releaseOptions{})
	cwd := t.TempDir()
	res := runScriptIn(t, cwd, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": "bin"}, toolbox(t))
	if res.code != 0 {
		t.Fatalf("exit %d, stderr: %s", res.code, res.stderr)
	}
	abs := filepath.Join(cwd, "bin")
	if _, err := os.Stat(filepath.Join(abs, "snoop")); err != nil {
		t.Errorf("want snoop in %s: %v", abs, err)
	}
	if want := `export PATH="` + abs + `:$PATH"`; !strings.Contains(res.stdout, want) {
		t.Errorf("stdout = %q, want the absolute hint %q", res.stdout, want)
	}
	if !strings.Contains(res.stdout, "Installed snoop v0.7.0 to "+abs+"/snoop") {
		t.Errorf("stdout = %q, want the absolute install path", res.stdout)
	}

	dir := t.TempDir()
	on := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir + "/"}, toolbox(t), dir)
	if on.code != 0 || strings.Contains(on.stdout, "not on your PATH") {
		t.Errorf("trailing slash, dir on PATH: exit %d, stdout %q, want no hint", on.code, on.stdout)
	}
}

// env(1) reads any argument containing "=" as a NAME=value assignment, so a
// path like that can't be handed to it as the command to run. An install
// directory is the user's to name.
func TestInstallDirWithAnEqualsSign(t *testing.T) {
	hostPlatform(t)
	r := newRelease(t, "v0.7.0", releaseOptions{})
	dir := filepath.Join(t.TempDir(), "a=b")
	res := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir}, toolbox(t))
	if res.code != 0 || !strings.Contains(res.stdout, "✓ Installed snoop v0.7.0 to "+dir+"/snoop") {
		t.Fatalf("exit %d, stdout %q, stderr %q: want a successful install into %s", res.code, res.stdout, res.stderr, dir)
	}
}

func TestRefusesTamperedArchive(t *testing.T) {
	hostPlatform(t)
	r := newRelease(t, "v0.7.0", releaseOptions{tamper: true})
	res := runScript(t, scriptPath, r, nil, toolbox(t))
	if res.code == 0 || !strings.Contains(res.stderr, "checksum mismatch") {
		t.Fatalf("exit %d, stderr %q: want a checksum refusal", res.code, res.stderr)
	}
	mustNotExist(t, filepath.Join(res.home, ".local", "bin", "snoop"))
}

func TestRefusesUnlistedArchive(t *testing.T) {
	hostPlatform(t)
	r := newRelease(t, "v0.7.0", releaseOptions{unlisted: true})
	res := runScript(t, scriptPath, r, nil, toolbox(t))
	if res.code == 0 || !strings.Contains(res.stderr, "not listed") {
		t.Fatalf("exit %d, stderr %q: want a refusal for an unlisted archive", res.code, res.stderr)
	}
	mustNotExist(t, filepath.Join(res.home, ".local", "bin", "snoop"))
}

func TestUnsupportedPlatform(t *testing.T) {
	for _, tc := range []struct{ kernel, machine string }{{"FreeBSD", "x86_64"}, {"Linux", "riscv64"}} {
		t.Run(tc.kernel+"/"+tc.machine, func(t *testing.T) {
			fakes := t.TempDir()
			fakeCommand(t, fakes, "uname", fmt.Sprintf(`case "$1" in -s) echo %s;; -m) echo %s;; esac`, tc.kernel, tc.machine))
			r := newRelease(t, "v0.7.0", releaseOptions{})
			res := runScript(t, scriptPath, r, nil, fakes, toolbox(t, "uname"))
			want := "no snoop build for " + tc.kernel + "/" + tc.machine
			if res.code == 0 || !strings.Contains(res.stderr, want) || !strings.Contains(res.stderr, "go install github.com/jzills/snoop/cmd/snoop@latest") {
				t.Fatalf("exit %d, stderr %q: want %q and the go install alternative", res.code, res.stderr, want)
			}
			if len(r.paths()) != 0 {
				t.Errorf("requests = %v, want none for an unsupported platform", r.paths())
			}
		})
	}
}

func TestRosettaGetsNativeBuild(t *testing.T) {
	for _, tc := range []struct{ sysctl, want string }{{"1", "darwin_arm64"}, {"0", "darwin_amd64"}} {
		t.Run("hw.optional.arm64="+tc.sysctl, func(t *testing.T) {
			fakes := t.TempDir()
			fakeCommand(t, fakes, "uname", `case "$1" in -s) echo Darwin;; -m) echo x86_64;; esac`)
			fakeCommand(t, fakes, "sysctl", "echo "+tc.sysctl)
			r := newRelease(t, "v0.7.0", releaseOptions{})
			res := runScript(t, scriptPath, r, nil, fakes, toolbox(t, "uname", "sysctl"))
			if res.code != 0 {
				t.Fatalf("exit %d, stderr: %s", res.code, res.stderr)
			}
			if want := "/latest/download/snoop_" + tc.want + ".tar.gz"; !contains(r.paths(), want) {
				t.Errorf("requests = %v, want %s", r.paths(), want)
			}
		})
	}
}

func TestPathHint(t *testing.T) {
	hostPlatform(t)
	r := newRelease(t, "v0.7.0", releaseOptions{})
	dir := t.TempDir()
	off := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir}, toolbox(t))
	if want := `export PATH="` + dir + `:$PATH"`; !strings.Contains(off.stdout, want) {
		t.Errorf("install dir off PATH: stdout = %q, want %q", off.stdout, want)
	}
	on := runScript(t, scriptPath, r, map[string]string{"SNOOP_INSTALL_DIR": dir}, toolbox(t), dir)
	if strings.Contains(on.stdout, "not on your PATH") {
		t.Errorf("install dir on PATH: stdout = %q, want no hint", on.stdout)
	}
}

func TestNamesMissingChecksumTool(t *testing.T) {
	r := newRelease(t, "v0.7.0", releaseOptions{})
	res := runScript(t, scriptPath, r, nil, toolbox(t, "sha256sum", "shasum"))
	if res.code == 0 || !strings.Contains(res.stderr, "sha256sum or shasum") {
		t.Fatalf("exit %d, stderr %q: want the missing tools named", res.code, res.stderr)
	}
	if len(r.paths()) != 0 {
		t.Errorf("requests = %v, want none before the tools are present", r.paths())
	}
}

func TestFallsBackToWget(t *testing.T) {
	hostPlatform(t)
	if _, err := exec.LookPath("wget"); err != nil {
		t.Skip("wget not installed")
	}
	r := newRelease(t, "v0.7.0", releaseOptions{})
	res := runScript(t, scriptPath, r, nil, toolbox(t, "curl"))
	if res.code != 0 {
		t.Fatalf("exit %d, stderr: %s", res.code, res.stderr)
	}
}

// A download cut off partway must run nothing: no request, no directory. Every
// prefix of the script short of its last line is run as if it were the whole
// download.
func TestTruncatedScriptRunsNothing(t *testing.T) {
	full, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(full), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if last := strings.TrimSpace(lines[len(lines)-1]); last != `main "$@"` {
		t.Fatalf("last line = %q, want main \"$@\"", last)
	}
	tools := toolbox(t)
	for n := 1; n < len(lines); n++ {
		prefix := filepath.Join(t.TempDir(), "install.sh")
		if err := os.WriteFile(prefix, []byte(strings.Join(lines[:n], "")), 0o644); err != nil {
			t.Fatal(err)
		}
		r := newRelease(t, "v0.7.0", releaseOptions{})
		res := runScript(t, prefix, r, nil, tools)
		if len(r.paths()) != 0 {
			t.Fatalf("first %d lines made requests %v", n, r.paths())
		}
		mustNotExist(t, filepath.Join(res.home, ".local"))
	}
}
