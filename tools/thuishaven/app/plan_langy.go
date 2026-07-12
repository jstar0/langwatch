package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/langwatch/langwatch/tools/thuishaven/domain"
)

// langyImage is the tag haven builds and runs the langyagent worker under in its
// container tiers. Local-only (never pushed/pulled) — built from Dockerfile.langyagent.
const langyImage = "langyagent:dev"

// langyChild builds the supervised child that runs the langyagent worker, picking
// the launch mechanism from the stack's tier (see domain.LangyTier):
//
//   - host-unsafe tier: a bare `go run` on the host (via `make service`), full host
//     access, the per-worker UID sandbox off (it needs root, which the developer's
//     own user is not). This is the fast-iteration tier.
//   - sandboxed / container-unsafe tiers: a `docker run` in colima. The sandboxed
//     tier keeps the UID sandbox on (production-like); the container-unsafe tier
//     turns it off but the VM still isolates the host. langyDockerHost is colima's
//     docker socket, resolved by Up before it plans; empty means the container
//     could not be prepared and this path is not reached.
func (o *Orchestrator) langyChild(st domain.Stack, opts PlanOptions, base []string, port int, langyDockerHost string) Child {
	if st.LangyTier.RunsInContainer() {
		return Child{
			Name: "langyagent", Dir: opts.RepoRoot, Color: palette[6],
			Shell: langyContainerShell(langyContainerOpts{
				Slug:              st.Slug,
				Port:              port,
				Secret:            domain.DefaultLangyInternalSecret,
				DisableUIDSandbox: st.LangyTier.DisablesUIDSandbox(),
			}),
			// Only DOCKER_HOST — the container is a clean environment; its langyagent
			// config rides the `docker run -e` flags, not the host overlay.
			Env: []string{"DOCKER_HOST=" + langyDockerHost},
		}
	}
	// Host tier: langyagent (the cmd/service mono-binary) takes its listen port from
	// PORT, not SERVER_ADDR (see services/langyagent/config.go) — PORT always wins.
	// Its sessions/workspace roots default to the in-container /workspace, which is
	// read-only on a dev host; point them at writable per-slug dirs under haven's
	// home and create them so the manager boots (session spawn still needs an
	// `opencode` binary on PATH, but the service itself comes up). The UID sandbox
	// is disabled — on the host the worker runs as the developer's own unprivileged
	// user, where the setuid + chown the sandbox needs would fail with EPERM.
	laRoot := filepath.Join(o.cfg.Home, "langyagent", st.Slug)
	_ = os.MkdirAll(filepath.Join(laRoot, "sessions"), 0o755)
	_ = os.MkdirAll(filepath.Join(laRoot, "workspace"), 0o755)
	return Child{
		Name: "langyagent", Dir: opts.RepoRoot, Color: palette[6],
		Shell: goServiceShell(opts.RepoRoot, "langyagent", opts.ShouldGoWatch),
		Env: append(append([]string{}, base...),
			fmt.Sprintf("PORT=%d", port),
			"SESSIONS_ROOT="+filepath.Join(laRoot, "sessions"),
			"LANGY_WORKSPACE_ROOT="+filepath.Join(laRoot, "workspace"),
			"LANGY_UNSAFE_DEV_DISABLE_ISOLATION=true",
		),
	}
}

// langyContainerOpts are the inputs to the `docker run` command for a
// containerized langyagent worker (the sandboxed / container-unsafe tiers).
type langyContainerOpts struct {
	Slug   string // per-worktree, names the container so restarts replace cleanly
	Port   int    // the manager's HTTP port, published to host loopback and set as PORT
	Secret string // the shared control-plane ↔ manager bearer secret
	Image  string // the image tag (defaults to langyImage when empty)
	// DisableUIDSandbox sets LANGY_UNSAFE_DEV_DISABLE_ISOLATION inside the container
	// — true only for the container-unsafe tier. The sandboxed tier leaves it unset
	// so the worker uses the ADR-033 per-worker UID sandbox (the container runs as
	// root, so setuid+chown work), exactly as production does.
	DisableUIDSandbox bool
}

func (o langyContainerOpts) image() string {
	if o.Image != "" {
		return o.Image
	}
	return langyImage
}

// containerName is the stable per-slug name so a restart replaces the previous
// container rather than colliding with it.
func (o langyContainerOpts) containerName() string {
	return "langyagent-" + o.Slug
}

// langyContainerShell builds the supervised child's shell for a containerized
// worker. It runs against colima's docker socket (DOCKER_HOST is set on the child
// env, not here). The container reaches the host control plane + gateway via the
// host.docker.internal overrides the overlay already injected; the host reaches
// the manager via the published loopback port. A stale container from a prior
// crash is force-removed first, then `exec docker run` makes the container the
// child's own process so the supervisor's SIGTERM stops it directly (and --rm
// cleans it up).
func langyContainerShell(o langyContainerOpts) string {
	run := []string{
		"docker", "run", "--rm",
		"--name", o.containerName(),
		// Never touch a registry — the image is built locally into this colima
		// daemon. A missing image is a hard error, not a silent pull of something else.
		"--pull", "never",
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", o.Port, o.Port),
		"-e", fmt.Sprintf("PORT=%d", o.Port),
		"-e", "ENVIRONMENT=local",
		"-e", "LANGY_INTERNAL_SECRET=" + o.Secret,
	}
	if o.DisableUIDSandbox {
		run = append(run, "-e", "LANGY_UNSAFE_DEV_DISABLE_ISOLATION=true")
	}
	run = append(run, o.image())

	quoted := make([]string, len(run))
	for i, a := range run {
		quoted[i] = shQuote(a)
	}
	return fmt.Sprintf("docker rm -f %s >/dev/null 2>&1; exec %s",
		shQuote(o.containerName()), strings.Join(quoted, " "))
}

// langyImageEnsureShell checks the image is present and builds it only when it is
// not, so a normal `up` pays nothing after the first (minutes-long) build. When
// forceRebuild is set (HAVEN_LANGY_REBUILD=1) it always rebuilds — the escape
// hatch for picking up langyagent source changes, since the presence check alone
// would keep running stale bytes. Runs from the repo root (the build context).
func langyImageEnsureShell(image string, forceRebuild bool) string {
	build := fmt.Sprintf("docker build -f Dockerfile.langyagent -t %s .", shQuote(image))
	if forceRebuild {
		return build
	}
	return fmt.Sprintf("docker image inspect %s >/dev/null 2>&1 || %s", shQuote(image), build)
}

// shQuote single-quotes a shell argument, escaping embedded single quotes. Every
// value here is a controlled constant today, but quoting keeps a future value with
// a shell metacharacter from breaking out of the command.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
