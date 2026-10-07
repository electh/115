package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/video-site/backend/internal/applog"
	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/mediaasset"
	"github.com/video-site/backend/internal/persistence"
)

// CoverProvider saves a validated network cover at the managed thumbnail path.
// An empty path means no match; errors also fall back to local extraction.
type CoverProvider interface {
	FetchCover(context.Context, *catalog.Video) (string, error)
}

type MetaTubeCover struct {
	baseURL  string
	token    string
	timeout  time.Duration
	localDir string
	client   *http.Client
}

func NewMetaTubeCover(baseURL, token string, timeout time.Duration, localDir string) *MetaTubeCover {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &MetaTubeCover{
		baseURL: strings.TrimRight(baseURL, "/"), token: token, timeout: timeout, localDir: localDir,
		// Requests stay on the configured service, including its image proxy.
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

type coverMatch struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

var errCoverNotFound = errors.New("MetaTube cover not found")

// Send names unchanged: MetaTube owns filename cleanup and number extraction.
func coverQueries(video *catalog.Video) []string {
	if strings.TrimSpace(video.FileName) != "" {
		return []string{video.FileName}
	}
	if strings.TrimSpace(video.Title) != "" {
		return []string{video.Title}
	}
	return nil
}

func (m *MetaTubeCover) get(ctx context.Context, path string, query url.Values, limit int64) ([]byte, error) {
	u := m.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errors.New("invalid MetaTube request")
	}
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		// Do not log service URLs or query strings, which may contain private names.
		return nil, errors.New("MetaTube request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errCoverNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MetaTube HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errors.New("read MetaTube response failed")
	}
	if int64(len(body)) > limit {
		return nil, errors.New("MetaTube response too large")
	}
	return body, nil
}

func (m *MetaTubeCover) FetchCover(ctx context.Context, video *catalog.Video) (string, error) {
	if video == nil || m.baseURL == "" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	for _, query := range coverQueries(video) {
		body, err := m.get(ctx, "/v1/movies/search", url.Values{"q": {query}}, 1<<20)
		if errors.Is(err, errCoverNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}
		var response struct {
			Data  []coverMatch    `json:"data"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return "", errors.New("invalid MetaTube search response")
		}
		if len(response.Error) > 0 && string(response.Error) != "null" {
			return "", errors.New("MetaTube search error")
		}
		if len(response.Data) > 0 {
			match := response.Data[0]
			if match.ID == "" || match.Provider == "" {
				return "", errors.New("invalid MetaTube first result")
			}
			// MetaTube's image proxy applies provider headers and returns JPEG,
			// avoiding hotlink failures and sending credentials to third parties.
			path := "/v1/images/backdrop/" + url.PathEscape(match.Provider) + "/" + url.PathEscape(match.ID)
			data, err := m.get(ctx, path, nil, 12<<20)
			if err != nil {
				return "", err
			}
			// Use only the first result; image failures fall back to extraction.
			return m.saveCover(ctx, video.ID, data)
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", nil
}

func (m *MetaTubeCover) saveCover(ctx context.Context, videoID string, data []byte) (string, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 20_000_000 {
		return "", errors.New("invalid or oversized cover image")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	dst := mediaasset.ThumbnailPath(m.localDir, videoID)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(dst), ".network-cover-*.jpg")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	encodeErr := jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	closeErr := f.Close()
	if encodeErr != nil {
		return "", encodeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := replaceFile(f.Name(), dst); err != nil {
		return "", err
	}
	return dst, nil
}

func (w *ThumbWorker) tryNetworkCover(ctx context.Context, video *catalog.Video) bool {
	if w.Covers == nil {
		return false
	}
	persistence.RLock()
	defer persistence.RUnlock()
	local, err := w.Covers.FetchCover(ctx, video)
	if err != nil {
		applog.Info(ctx, "Network cover unavailable; use frame extraction", applog.Fields{Component: "thumb", VideoID: video.ID})
		return false
	}
	if local == "" {
		return false
	}
	if err := w.Catalog.UpdateVideoMeta(ctx, video.ID, catalog.VideoMetaPatch{
		ThumbnailURL: "/p/thumb/" + video.ID, ThumbnailStatus: "ready",
	}); err != nil {
		_ = os.Remove(local)
		return false
	}
	applog.Info(ctx, "Network cover ready", applog.Fields{Component: "thumb", VideoID: video.ID})
	return true
}
