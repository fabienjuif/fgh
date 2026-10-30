//go:build integration

package forgejotest_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fabienjuif/fgh/forgejo"
	"github.com/fabienjuif/fgh/internal/forgejotest"
)

func TestAttachmentsUpload(t *testing.T) {
	in := forgejotest.Start(t)
	owner, repo, _ := strings.Cut(in.Repo, "/")

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	want := buf.Bytes()

	ctx := context.Background()
	client := forgejo.New(in.URL, in.Token)
	att, err := client.Attachments.Upload(ctx, owner, repo, int64(in.BranchPR), "shot.png", bytes.NewReader(want))
	require.NoError(t, err)
	require.NotZero(t, att.ID)
	require.Equal(t, "shot.png", att.Name)
	require.NotEmpty(t, att.BrowserDownloadURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, att.BrowserDownloadURL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "token "+in.Token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, want, got)
}
