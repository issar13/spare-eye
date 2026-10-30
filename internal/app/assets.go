package app

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"spareeye/internal/media"
)

const (
	previewRoute  = "/preview.jpg"
	snapshotRoute = "/media/snapshots/"
	videoRoute    = "/media/videos/"
	posterRoute   = "/media/posters/"
)

// AssetHandler serves what cannot be embedded in the binary: the live camera
// frame and the media files on disk.
//
// Wails hands unmatched asset requests here, which is what lets the webview use
// an <img> and a <video> element for both. Sending the frames as base64 over
// the JavaScript bridge instead would copy every frame through JSON.
func (s *Service) AssetHandler() http.Handler {
	router := http.NewServeMux()

	router.HandleFunc(previewRoute, s.servePreview)
	router.HandleFunc(snapshotRoute, s.serveMedia)
	router.HandleFunc(videoRoute, s.serveMedia)
	router.HandleFunc(posterRoute, s.serveMedia)

	return router
}

func (s *Service) servePreview(writer http.ResponseWriter, request *http.Request) {
	frame := s.PreviewJPEG()
	if frame == nil {
		// No content, rather than an error: the UI polls this before the camera
		// is armed and a 404 would fill the console with failures.
		writer.WriteHeader(http.StatusNoContent)
		return
	}

	writer.Header().Set("Content-Type", "image/jpeg")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Write(frame)
}

func (s *Service) serveMedia(writer http.ResponseWriter, request *http.Request) {
	s.mutex.Lock()
	snapshots := s.settings.SnapshotDirectory()
	videos := s.settings.VideoDirectory()
	s.mutex.Unlock()

	directory := videos
	switch {
	case strings.HasPrefix(request.URL.Path, snapshotRoute):
		directory = snapshots
	case strings.HasPrefix(request.URL.Path, posterRoute):
		directory = media.PosterDirectory(videos)
	}

	// Only a bare file name is accepted. The request path reaches here from a
	// webview, and path.Base strips any attempt to climb out of the directory.
	name := path.Base(request.URL.Path)
	if name == "." || name == "/" {
		http.NotFound(writer, request)
		return
	}

	http.ServeFile(writer, request, filepath.Join(directory, name))
}
