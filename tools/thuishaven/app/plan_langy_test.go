package app

import (
	"strings"
	"testing"
)

func TestLangyContainerShell(t *testing.T) {
	t.Run("given the sandboxed tier (UID sandbox on)", func(t *testing.T) {
		sh := langyContainerShell(langyContainerOpts{
			Slug: "happy-tiger", Port: 49624, Secret: "sekret",
		})
		t.Run("publishes the manager port to host loopback", func(t *testing.T) {
			if !strings.Contains(sh, "'-p' '127.0.0.1:49624:49624'") {
				t.Fatalf("missing loopback publish in: %s", sh)
			}
		})
		t.Run("sets PORT, ENVIRONMENT and the internal secret", func(t *testing.T) {
			for _, want := range []string{"'PORT=49624'", "'ENVIRONMENT=local'", "'LANGY_INTERNAL_SECRET=sekret'"} {
				if !strings.Contains(sh, want) {
					t.Fatalf("missing %s in: %s", want, sh)
				}
			}
		})
		t.Run("does NOT disable the UID sandbox", func(t *testing.T) {
			if strings.Contains(sh, "LANGY_UNSAFE_DEV_DISABLE_ISOLATION") {
				t.Fatalf("sandboxed tier must keep the UID sandbox on: %s", sh)
			}
		})
		t.Run("force-removes a stale container then execs docker run", func(t *testing.T) {
			if !strings.Contains(sh, "docker rm -f 'langyagent-happy-tiger'") {
				t.Fatalf("missing stale-container cleanup: %s", sh)
			}
			if !strings.Contains(sh, "exec 'docker' 'run' '--rm'") {
				t.Fatalf("must exec docker run so SIGTERM reaches it: %s", sh)
			}
		})
		t.Run("never pulls from a registry", func(t *testing.T) {
			if !strings.Contains(sh, "'--pull' 'never'") {
				t.Fatalf("must pin --pull never: %s", sh)
			}
		})
		t.Run("runs the default local image", func(t *testing.T) {
			if !strings.Contains(sh, "'"+langyImage+"'") {
				t.Fatalf("missing image %s in: %s", langyImage, sh)
			}
		})
	})

	t.Run("given the container-unsafe tier", func(t *testing.T) {
		sh := langyContainerShell(langyContainerOpts{
			Slug: "brave-otter", Port: 5000, Secret: "s", DisableUIDSandbox: true,
		})
		t.Run("disables the UID sandbox inside the container", func(t *testing.T) {
			if !strings.Contains(sh, "'LANGY_UNSAFE_DEV_DISABLE_ISOLATION=true'") {
				t.Fatalf("container-unsafe tier must disable the UID sandbox: %s", sh)
			}
		})
	})
}

func TestLangyImageEnsureShell(t *testing.T) {
	t.Run("given no force rebuild", func(t *testing.T) {
		sh := langyImageEnsureShell("langyagent:dev", false)
		t.Run("builds only when the image is absent", func(t *testing.T) {
			if !strings.Contains(sh, "docker image inspect 'langyagent:dev' >/dev/null 2>&1 ||") {
				t.Fatalf("expected presence-gated build, got: %s", sh)
			}
			if !strings.Contains(sh, "docker build -f Dockerfile.langyagent -t 'langyagent:dev' .") {
				t.Fatalf("missing build command: %s", sh)
			}
		})
	})
	t.Run("given a forced rebuild", func(t *testing.T) {
		sh := langyImageEnsureShell("langyagent:dev", true)
		t.Run("always rebuilds, unconditionally", func(t *testing.T) {
			if strings.Contains(sh, "docker image inspect") {
				t.Fatalf("forced rebuild must not gate on presence: %s", sh)
			}
			if !strings.HasPrefix(sh, "docker build -f Dockerfile.langyagent") {
				t.Fatalf("expected an unconditional build: %s", sh)
			}
		})
	})
}
