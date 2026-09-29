package main

import (
	"fmt"
	"sort"
	"strings"
)

// containerName derives devdb's container name from the compose service
// name, e.g. "db" -> "glidepath-db", "db-test" -> "glidepath-db-test" — so
// a second service (the integration-test DB) never collides with the main
// dev DB's container/volume regardless of engine.
func containerName(service string) string {
	return "glidepath-" + service
}

// buildRunArgs builds the argument list for "<engine> run -d --name
// <name> ... <image>" from a compose service definition. Environment keys
// are sorted so the generated command is deterministic (useful for tests
// and for anyone reading `devdb`'s log output).
func buildRunArgs(name string, svc composeService) []string {
	args := []string{"run", "-d", "--name", name}

	keys := make([]string, 0, len(svc.Environment))
	for k := range svc.Environment {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", k+"="+svc.Environment[k])
	}

	for _, p := range svc.Ports {
		args = append(args, "-p", p)
	}
	for _, v := range svc.Volumes {
		args = append(args, "-v", v)
	}

	return append(args, svc.Image)
}

// hostAddr parses a compose port entry ("[host-ip:]host-port:container-port")
// into a host:port string reachable from outside the container — what a
// client on the host (goose, the app, …) actually connects to, as opposed
// to `<engine> exec`'s in-container view. Defaults the host IP to
// 127.0.0.1 when omitted.
func hostAddr(port string) (string, error) {
	parts := strings.Split(port, ":")
	switch len(parts) {
	case 3:
		ip := parts[0]
		if ip == "" {
			ip = "127.0.0.1"
		}
		return ip + ":" + parts[1], nil
	case 2:
		return "127.0.0.1:" + parts[0], nil
	default:
		return "", fmt.Errorf("unrecognized port spec %q", port)
	}
}

// volumeNames extracts the named-volume half of each "name:path" entry in
// svc.Volumes, skipping bind mounts (paths starting with "/" or ".").
func volumeNames(svc composeService) []string {
	names := make([]string, 0, len(svc.Volumes))
	for _, v := range svc.Volumes {
		name, _, ok := strings.Cut(v, ":")
		if !ok || name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	return names
}
