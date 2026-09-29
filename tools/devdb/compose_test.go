package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCompose(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test compose file: %v", err)
	}
	return path
}

func TestLoadService(t *testing.T) {
	path := writeCompose(t, `
services:
  db:
    image: postgres:17
    environment:
      POSTGRES_USER: glidepath
      POSTGRES_PASSWORD: glidepath
      POSTGRES_DB: glidepath
    ports:
      - "127.0.0.1:5432:5432"
    volumes:
      - db-data:/var/lib/postgresql/data

volumes:
  db-data:
`)

	svc, err := loadService(path, "db")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.Image != "postgres:17" {
		t.Errorf("Image = %q, want postgres:17", svc.Image)
	}
	if svc.Environment["POSTGRES_USER"] != "glidepath" {
		t.Errorf("POSTGRES_USER = %q, want glidepath", svc.Environment["POSTGRES_USER"])
	}
	if len(svc.Ports) != 1 || svc.Ports[0] != "127.0.0.1:5432:5432" {
		t.Errorf("Ports = %v", svc.Ports)
	}
	if len(svc.Volumes) != 1 || svc.Volumes[0] != "db-data:/var/lib/postgresql/data" {
		t.Errorf("Volumes = %v", svc.Volumes)
	}
}

func TestLoadService_UnknownService(t *testing.T) {
	path := writeCompose(t, "services:\n  db:\n    image: postgres:17\n")
	if _, err := loadService(path, "nope"); err == nil {
		t.Fatal("expected an error for an unknown service name")
	}
}

func TestLoadService_MissingImage(t *testing.T) {
	path := writeCompose(t, "services:\n  db:\n    ports: []\n")
	if _, err := loadService(path, "db"); err == nil {
		t.Fatal("expected an error for a service with no image")
	}
}

func TestLoadService_FileNotFound(t *testing.T) {
	if _, err := loadService("/nonexistent/docker-compose.yml", "db"); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
