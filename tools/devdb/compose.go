package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// composeFile is the narrow subset of docker-compose.yml's schema devdb
// understands: one or more services, each a plain container spec. It does
// not aim to support the full Compose spec (build, depends_on, networks,
// …) — just enough to describe glidepath's single dev-only Postgres
// service, which is all this project needs.
type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Image       string            `yaml:"image"`
	Environment map[string]string `yaml:"environment"`
	Ports       []string          `yaml:"ports"`
	Volumes     []string          `yaml:"volumes"`
}

// loadService parses path and returns the named service.
func loadService(path, name string) (composeService, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return composeService{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var f composeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return composeService{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	svc, ok := f.Services[name]
	if !ok {
		return composeService{}, fmt.Errorf("%s has no service named %q", path, name)
	}
	if svc.Image == "" {
		return composeService{}, fmt.Errorf("%s: service %q has no image", path, name)
	}
	return svc, nil
}
