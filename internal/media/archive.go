package media

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
)

// WriteArchive streams the files at paths into a zip archive, one entry per
// file named after its base name. Entries are stored, not deflated: mp4 and jpg
// are already compressed, so deflating would spend CPU on an old laptop for
// almost no saving in size.
func WriteArchive(writer io.Writer, paths []string) error {
	archive := zip.NewWriter(writer)

	for _, path := range paths {
		if err := addArchiveEntry(archive, path); err != nil {
			return err
		}
	}

	return archive.Close()
}

func addArchiveEntry(archive *zip.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.Base(path)
	header.Method = zip.Store

	entry, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(entry, file)

	return err
}
