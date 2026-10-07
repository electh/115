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

func TestCoverQueries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		video catalog.Video
		want  []string
	}{
		{"source prefix", catalog.Video{FileName: "hhd800.com@STARS-847", Title: "edited title"}, []string{"hhd800.com@STARS-847"}},
		{"release tags", catalog.Video{FileName: "[source] abc_00123_C_1080p.mp4"}, []string{"[source] abc_00123_C_1080p.mp4"}},
		{"title punctuation", catalog.Video{FileName: "Example.Movie.1080p.x265.mkv"}, []string{"Example.Movie.1080p.x265.mkv"}},
		{"URL encoding", catalog.Video{FileName: "来源@STARS-847 & extras+#.mp4"}, []string{"来源@STARS-847 & extras+#.mp4"}},
		{"short name", catalog.Video{FileName: "x.mp4"}, []string{"x.mp4"}},
		{"missing filename", catalog.Video{Title: "STARS-847 edited title"}, []string{"STARS-847 edited title"}},
		{"blank filename", catalog.Video{FileName: "  ", Title: "fallback title"}, []string{"fallback title"}},
		{"empty names", catalog.Video{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := coverQueries(&tc.video)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("queries=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestNetworkCoverWorkerSuccessAndFallback(t *testing.T) {
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 16, 9)), nil); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "source-prefix", "filename-encoding", "no-match", "wrong-number", "multiple-results", "invalid-first-result", "invalid-image", "http-error", "not-found", "malformed", "timeout", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			imageRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("missing authentication")
				}
				if r.URL.Path == "/prefix/v1/movies/search" {
					wantQuery := "ABC-123.mp4"
					if mode == "source-prefix" {
						wantQuery = "hhd800.com@STARS-847"
					}
					if mode == "filename-encoding" {
						wantQuery = "来源@STARS-847 & extras+#.mp4"
					}
					if r.URL.Query().Get("q") != wantQuery {
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
					matches := []map[string]string{{"id": "movie", "provider": "provider", "number": "ABC-123"}}
					if mode == "no-match" {
						matches = nil
					}
					if mode == "wrong-number" {
						matches[0]["number"] = "ABC-124"
					}
					if mode == "multiple-results" || mode == "invalid-image" || mode == "invalid-first-result" {
						matches = append(matches, map[string]string{"id": "second", "provider": "provider", "number": "ABC-123"})
					}
					if mode == "invalid-first-result" {
						delete(matches[0], "id")
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
			if mode == "source-prefix" {
				video.FileName = "hhd800.com@STARS-847"
			}
			if mode == "filename-encoding" {
				video.FileName = "来源@STARS-847 & extras+#.mp4"
			}
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
			if mode == "success" || mode == "source-prefix" || mode == "filename-encoding" || mode == "wrong-number" || mode == "multiple-results" {
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
				if mode == "invalid-first-result" && imageRequests != 0 {
					t.Fatal("downloaded a later result after invalid first result")
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
