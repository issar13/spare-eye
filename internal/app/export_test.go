package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"spareeye/internal/media"
)

func TestExportPathsFollowTheCollection(t *testing.T) {
	service := newTestService(t)
	videos := service.settings.VideoDirectory()
	snapshots := service.settings.SnapshotDirectory()

	for _, path := range []string{
		filepath.Join(videos, "clip-plain.mp4"),
		filepath.Join(videos, "clip-starred.mp4"),
		filepath.Join(snapshots, "photo-plain.jpg"),
		filepath.Join(snapshots, "photo-starred.jpg"),
	} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"clip-starred.mp4", "photo-starred.jpg"} {
		if err := media.SetProtected(service.settings.Storage.MediaDirectory, name, true); err != nil {
			t.Fatal(err)
		}
	}

	names := func(collection string) []string {
		paths, err := service.ExportPaths(collection)
		if err != nil {
			t.Fatalf("ExportPaths(%q) returned %v", collection, err)
		}
		baseNames := make([]string, 0, len(paths))
		for _, path := range paths {
			baseNames = append(baseNames, filepath.Base(path))
		}
		slices.Sort(baseNames)
		return baseNames
	}

	cases := map[string][]string{
		"clips":     {"clip-plain.mp4", "clip-starred.mp4"},
		"snapshots": {"photo-plain.jpg", "photo-starred.jpg"},
		"starred":   {"clip-starred.mp4", "photo-starred.jpg"},
	}
	for collection, want := range cases {
		if got := names(collection); !slices.Equal(got, want) {
			t.Errorf("%s exports %v, want %v", collection, got, want)
		}
	}
}

func TestExportPathsRejectsAnUnknownCollection(t *testing.T) {
	if _, err := newTestService(t).ExportPaths("../etc"); err == nil {
		t.Error("expected an error for an unknown collection")
	}
}

func TestWriteExportArchiveLeavesNoPartialFileOnFailure(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "export.zip")

	err := newTestService(t).WriteExportArchive([]string{filepath.Join(t.TempDir(), "gone.mp4")}, destination)
	if err == nil {
		t.Fatal("expected an error for a missing source file")
	}

	for _, leftover := range []string{destination, destination + ".partial"} {
		if _, statErr := os.Stat(leftover); !os.IsNotExist(statErr) {
			t.Errorf("%s should not exist after a failed export", leftover)
		}
	}
}

func TestWriteExportArchiveCreatesAReadableArchive(t *testing.T) {
	source := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(source, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "export.zip")

	if err := newTestService(t).WriteExportArchive([]string{source}, destination); err != nil {
		t.Fatalf("WriteExportArchive returned %v", err)
	}

	reader, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 1 || reader.File[0].Name != "clip.mp4" {
		t.Errorf("archive entries = %v", reader.File)
	}
	if _, err := os.Stat(destination + ".partial"); !os.IsNotExist(err) {
		t.Error("the partial file should have been renamed away")
	}
}
