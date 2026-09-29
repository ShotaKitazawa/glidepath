// Command devdb starts/stops a Postgres container declared in
// docker-compose.yml, using whichever container engine is available:
// Docker (or an OrbStack/Colima-style Docker-compatible daemon) or Apple's
// native `container` CLI. docker-compose.yml stays the single declarative
// source; devdb only translates the named service into the equivalent
// `run`/`start`/`stop`/`rm` invocations, since Apple's `container` has no
// supported `compose` equivalent (the one third-party plugin that exists,
// container-compose, was flagged by Homebrew as a possible supply-chain
// compromise — not something this project will depend on).
//
// Usage: devdb <up|down|reset> [service]
// service defaults to "db" (the main dev DB); "db-test" is the isolated
// integration-test DB (see mise.toml's test-db-* / test-integration tasks)
// — a different container/volume so test runs never touch dev data.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/jackc/pgx/v5"
)

const composePath = "docker-compose.yml"

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: devdb <up|down|reset> [service]")
		os.Exit(2)
	}

	service := "db"
	if len(os.Args) == 3 {
		service = os.Args[2]
	}

	if err := run(os.Args[1], service); err != nil {
		fmt.Fprintln(os.Stderr, "devdb:", err)
		os.Exit(1)
	}
}

func run(cmd, service string) error {
	engine, err := detectEngine()
	if err != nil {
		return err
	}
	fmt.Printf("devdb: using %s\n", engine)

	svc, err := loadService(composePath, service)
	if err != nil {
		return err
	}
	name := containerName(service)

	switch cmd {
	case "up":
		return up(engine, name, svc)
	case "down":
		return execVisible(engine, "stop", name)
	case "reset":
		// stop/rm/volume rm all error on a target that doesn't exist yet
		// (e.g. the very first reset ever) — best-effort, not fatal, and
		// quiet since "doesn't exist" is the common, expected case here.
		_ = execQuiet(engine, "stop", name)
		_ = execQuiet(engine, "rm", "-f", name)
		for _, volName := range volumeNames(svc) {
			_ = execQuiet(engine, "volume", "rm", volName)
		}
		return up(engine, name, svc)
	default:
		return fmt.Errorf("unknown subcommand %q (want up, down, or reset)", cmd)
	}
}

// detectEngine prefers a running Docker(-compatible) daemon, falling back
// to Apple's container CLI.
func detectEngine() (string, error) {
	if _, err := exec.LookPath("docker"); err == nil {
		if execQuiet("docker", "info") == nil {
			return "docker", nil
		}
	}
	if _, err := exec.LookPath("container"); err == nil {
		return "container", nil
	}
	return "", fmt.Errorf("neither a running docker daemon nor the container CLI was found; install Docker/OrbStack or Apple's `container` (brew install container)")
}

func up(engine, name string, svc composeService) error {
	if execQuiet(engine, "start", name) == nil {
		return waitReady(svc)
	}

	for _, volName := range volumeNames(svc) {
		if err := execVisible(engine, "volume", "create", volName); err != nil {
			return fmt.Errorf("creating volume %s: %w", volName, err)
		}
	}

	if err := execVisible(engine, buildRunArgs(name, svc)...); err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	return waitReady(svc)
}

// waitReady polls from the host, the same vantage point goose/the app
// connect from — not `<engine> exec`'s in-container view, which can report
// ready slightly before the host-side port forward has stabilized (seen in
// practice with Apple's `container`: pg_isready inside the container
// succeeds, but a client connecting from the host still gets "connection
// reset by peer" for a moment after).
func waitReady(svc composeService) error {
	if len(svc.Ports) == 0 {
		return fmt.Errorf("service has no ports to check readiness against")
	}
	addr, err := hostAddr(svc.Ports[0])
	if err != nil {
		return err
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		svc.Environment["POSTGRES_USER"], svc.Environment["POSTGRES_PASSWORD"], addr, svc.Environment["POSTGRES_DB"])

	ctx := context.Background()
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := pingPostgres(ctx, connStr); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		fmt.Println("devdb: database ready")
		return nil
	}
	return fmt.Errorf("database did not become ready within 30s: %w", lastErr)
}

func pingPostgres(ctx context.Context, connStr string) error {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, "SELECT 1")
	return err
}

// execVisible runs engine with args, streaming stdout/stderr.
func execVisible(engine string, args ...string) error {
	cmd := exec.Command(engine, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// execQuiet runs engine with args, discarding output — used for
// existence/readiness checks where failure is an expected, non-noteworthy
// outcome.
func execQuiet(engine string, args ...string) error {
	cmd := exec.Command(engine, args...)
	return cmd.Run()
}
