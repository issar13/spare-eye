package media

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteArchiveRoundTripsFileContents(t *testing.T) {
	directory := t.TempDir()
	first := filepath.Join(directory, "first.mp4")
	second := filepath.Join(directory, "second.jpg")
	if err := os.WriteFile(first, []byte("video bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("photo bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buffer bytes.Buffer
	if err := WriteArchive(&buffer, []string{first, second}); err != nil {
		t.Fatalf("WriteArchive returned %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatalf("the archive could not be read back: %v", err)
	}

	contents := map[string]string{}
	for _, entry := range reader.File {
		if entry.Method != zip.Store {
			t.Errorf("%s uses method %d, want stored", entry.Name, entry.Method)
		}
		opened, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(opened)
		opened.Close()
		if err != nil {
			t.Fatal(err)
		}
		contents[entry.Name] = string(body)
	}

	want := map[string]string{"first.mp4": "video bytes", "second.jpg": "photo bytes"}
	if len(contents) != len(want) || contents["first.mp4"] != want["first.mp4"] || contents["second.jpg"] != want["second.jpg"] {
		t.Errorf("archive contents = %v, want %v", contents, want)
	}
}

func TestWriteArchiveWithNoFilesIsAValidEmptyArchive(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteArchive(&buffer, nil); err != nil {
		t.Fatalf("WriteArchive returned %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatalf("an empty archive should still be readable: %v", err)
	}
	if len(reader.File) != 0 {
		t.Errorf("empty archive holds %d entries", len(reader.File))
	}
}

func TestWriteArchiveFailsForAMissingFile(t *testing.T) {
	var buffer bytes.Buffer
	if err := WriteArchive(&buffer, []string{filepath.Join(t.TempDir(), "gone.mp4")}); err == nil {
		t.Error("expected an error for a file that does not exist")
	}
}
