package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/mediaasset"
)

func TestCoverQueriesAndMatching(t *testing.T) {
	for _, tc := range []struct {
		name string
		want []string
	}{
		{"[source] abc_00123_C_1080p.mp4", []string{"ABC-00123"}},
		{"FC2_PPV_1234567.mp4", []string{"FC2-PPV-1234567"}},
		{"Example.Movie.1080p.x265.mkv", []string{"Example Movie"}},
		{"x.mp4", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := coverQueries(&catalog.Video{FileName: tc.name})
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("queries=%v want=%v", got, tc.want)
			}
		})
	}
	match := coverMatch{ID: "id", Provider: "provider", Number: "ABC-123", Title: "Example Movie"}
	if !matchesCover("ABC-00123", match) || !matchesCover("Example.Movie.mp4", match) {
		t.Fatal("exact matches rejected")
	}
	if matchesCover("ABC-124", match) || matchesCover("Example", match) {
		t.Fatal("unrelated result accepted")
	}
}

func TestNetworkCoverWorkerSuccessAndFallback(t *testing.T) {
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 16, 9)), nil); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "no-match", "wrong-number", "invalid-image", "http-error", "not-found", "malformed", "timeout", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			imageRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("missing authentication")
				}
				if r.URL.Path == "/prefix/v1/movies/search" {
					if r.URL.Query().Get("q") != "ABC-123" {
						t.Error("incorrect filename query")
					}
					switch mode {
					case "not-found":
						w.WriteHeader(404)
						return
					case "http-error":
						w.WriteHeader(503)
						return
					case "malformed":
						_, _ = w.Write([]byte("invalid"))
						return
					case "timeout", "canceled":
						<-r.Context().Done()
						return
					}
					matches := []coverMatch{{ID: "movie", Provider: "provider", Number: "ABC-123"}}
					if mode == "no-match" {
						matches = nil
					}
					if mode == "wrong-number" {
						matches[0].Number = "ABC-124"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": matches})
					return
				}
				if r.URL.Path != "/prefix/v1/images/backdrop/provider/movie" {
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				imageRequests++
				if mode == "invalid-image" {
					_, _ = w.Write([]byte("<html>not an image</html>"))
					return
				}
				_, _ = w.Write(jpegData.Bytes())
			}))
			defer server.Close()
			cat, video := seedPreviewTestVideo(t, "network-cover-"+mode)
			video.FileName = "ABC-123.mp4"
			if err := cat.UpsertVideo(context.Background(), video); err != nil {
				t.Fatal(err)
			}
			gen := &fakeThumbGenerator{probeDuration: 42}
			drive := &previewFakeDrive{}
			worker := NewThumbWorker(gen, cat, drive)
			dir := t.TempDir()
			provider := NewMetaTubeCover(server.URL+"/prefix", "secret", time.Second, dir)
			if mode == "timeout" {
				provider.timeout = 20 * time.Millisecond
			}
			worker.Covers = provider
			ctx := context.Background()
			if mode == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 30*time.Millisecond)
				defer cancel()
			}
			worker.process(ctx, video)
			got, err := cat.GetVideo(context.Background(), video.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "canceled" {
				if got.ThumbnailURL != "" || gen.thumbnailVideoID != "" || drive.streamFileID != "" {
					t.Fatal("cancellation triggered extraction")
				}
				return
			}
			if got.ThumbnailURL != "/p/thumb/"+video.ID || got.PreviewStatus != "pending" {
				t.Fatalf("unexpected metadata: %+v", got)
			}
			if mode == "success" {
				if gen.thumbnailVideoID != "" || gen.probeCalls != 0 || drive.streamFileID != "" {
					t.Fatal("network success accessed video stream")
				}
				data, err := os.ReadFile(mediaasset.ThumbnailPath(dir, video.ID))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
					t.Fatal(err)
				}
				worker.process(context.Background(), video)
				if imageRequests != 1 {
					t.Fatal("existing cover fetched again")
				}
			} else {
				if gen.thumbnailVideoID != video.ID || got.DurationSeconds != 42 {
					t.Fatal("original frame extraction did not run")
				}
				if mode == "wrong-number" && imageRequests != 0 {
					t.Fatal("downloaded mismatched cover")
				}
				if _, err := os.Stat(mediaasset.ThumbnailPath(dir, video.ID)); !os.IsNotExist(err) {
					t.Fatal("failed cover left an asset")
				}
			}
		})
	}
}

func TestNetworkCoverRejectsRedirectAndOversizedResponse(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, target.URL, http.StatusFound)
			return
		}
		_, _ = w.Write(make([]byte, 100))
	}))
	defer server.Close()
	provider := NewMetaTubeCover(server.URL, "secret", time.Second, t.TempDir())
	if _, err := provider.get(context.Background(), "/redirect", nil, 1000); err == nil {
		t.Fatal("redirect accepted")
	}
	if targetCalls != 0 {
		t.Fatal("followed redirect with token")
	}
	if _, err := provider.get(context.Background(), "/large", nil, 10); err == nil {
		t.Fatal("oversized response accepted")
	}
}
