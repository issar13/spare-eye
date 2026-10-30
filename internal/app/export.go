package app

import (
	"fmt"
	"os"
	"slices"

	"spareeye/internal/media"
)

// ExportPaths lists the files an export of one tab contains: "clips",
// "snapshots", or "starred" (protected clips and snapshots together). The
// paths come from the service's own listing, never from the webview, so an
// export cannot be steered at files outside the media folders.
func (s *Service) ExportPaths(collection string) ([]string, error) {
	switch collection {
	case "clips":
		clips, err := s.Clips()
		return recordingPaths(clips), err
	case "snapshots":
		snapshots, err := s.Snapshots()
		return recordingPaths(snapshots), err
	case "starred":
		clips, err := s.Clips()
		if err != nil {
			return nil, err
		}

		snapshots, err := s.Snapshots()
		if err != nil {
			return nil, err
		}

		starred := slices.DeleteFunc(slices.Concat(clips, snapshots), func(view RecordingView) bool {
			return !view.Protected
		})

		return recordingPaths(starred), nil
	}

	return nil, fmt.Errorf("%q is not a collection that can be exported", collection)
}

func recordingPaths(views []RecordingView) []string {
	paths := make([]string, 0, len(views))
	for _, view := range views {
		paths = append(paths, view.Path)
	}

	return paths
}

// WriteExportArchive writes the files at paths to destination as a zip. The
// archive is built beside the destination and renamed into place, so a failure
// part way through never leaves a truncated .zip where the user expects a good
// one.
func (s *Service) WriteExportArchive(paths []string, destination string) error {
	err := writeArchiveAtomically(paths, destination)
	if err != nil {
		s.log.Error("exporting recordings failed", "destination", destination, "error", err)
	}

	return err
}

func writeArchiveAtomically(paths []string, destination string) error {
	partialPath := destination + ".partial"

	partial, err := os.Create(partialPath)
	if err != nil {
		return err
	}

	if err := media.WriteArchive(partial, paths); err != nil {
		partial.Close()
		os.Remove(partialPath)
		return err
	}

	if err := partial.Close(); err != nil {
		os.Remove(partialPath)
		return err
	}

	if err := os.Rename(partialPath, destination); err != nil {
		os.Remove(partialPath)
		return err
	}

	return nil
}
