package main

import (
	"reflect"
	"testing"
)

func TestContainerName(t *testing.T) {
	if got, want := containerName("db"), "glidepath-db"; got != want {
		t.Errorf("containerName(%q) = %q, want %q", "db", got, want)
	}
	if got, want := containerName("db-test"), "glidepath-db-test"; got != want {
		t.Errorf("containerName(%q) = %q, want %q", "db-test", got, want)
	}
}

func TestBuildRunArgs(t *testing.T) {
	svc := composeService{
		Image: "postgres:17",
		Environment: map[string]string{
			"POSTGRES_USER":     "glidepath",
			"POSTGRES_PASSWORD": "glidepath",
			"POSTGRES_DB":       "glidepath",
		},
		Ports:   []string{"127.0.0.1:5432:5432"},
		Volumes: []string{"db-data:/var/lib/postgresql/data"},
	}

	want := []string{
		"run", "-d", "--name", "glidepath-db",
		"-e", "POSTGRES_DB=glidepath",
		"-e", "POSTGRES_PASSWORD=glidepath",
		"-e", "POSTGRES_USER=glidepath",
		"-p", "127.0.0.1:5432:5432",
		"-v", "db-data:/var/lib/postgresql/data",
		"postgres:17",
	}

	got := buildRunArgs("glidepath-db", svc)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildRunArgs() =\n%v\nwant\n%v", got, want)
	}
}

func TestBuildRunArgs_Deterministic(t *testing.T) {
	svc := composeService{
		Image:       "postgres:17",
		Environment: map[string]string{"B": "2", "A": "1", "C": "3"},
	}
	first := buildRunArgs("glidepath-db", svc)
	for range 5 {
		if got := buildRunArgs("glidepath-db", svc); !reflect.DeepEqual(got, first) {
			t.Fatalf("buildRunArgs() is not deterministic: %v vs %v", got, first)
		}
	}
}

func TestHostAddr(t *testing.T) {
	tests := []struct {
		port string
		want string
	}{
		{"127.0.0.1:5432:5432", "127.0.0.1:5432"},
		{"5432:5432", "127.0.0.1:5432"},
		{"0.0.0.0:15432:5432", "0.0.0.0:15432"},
	}
	for _, tt := range tests {
		got, err := hostAddr(tt.port)
		if err != nil {
			t.Fatalf("hostAddr(%q) unexpected error: %v", tt.port, err)
		}
		if got != tt.want {
			t.Errorf("hostAddr(%q) = %q, want %q", tt.port, got, tt.want)
		}
	}
}

func TestHostAddr_Invalid(t *testing.T) {
	if _, err := hostAddr("not-a-port-spec"); err == nil {
		t.Fatal("expected an error for an unrecognized port spec")
	}
}

func TestVolumeNames(t *testing.T) {
	tests := []struct {
		name    string
		volumes []string
		want    []string
	}{
		{"named volume", []string{"db-data:/var/lib/postgresql/data"}, []string{"db-data"}},
		{"bind mount skipped", []string{"/host/path:/container/path"}, nil},
		{"relative bind mount skipped", []string{"./data:/container/path"}, nil},
		{"mixed", []string{"db-data:/a", "/host:/b"}, []string{"db-data"}},
		{"none", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := volumeNames(composeService{Volumes: tt.volumes})
			if len(got) != len(tt.want) {
				t.Fatalf("volumeNames() = %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("volumeNames()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
