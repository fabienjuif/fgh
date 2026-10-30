package forgejo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachmentsUpload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/repos/o%20w/r/issues/7/assets", r.URL.EscapedPath())
		assert.Equal(t, "shot 1.png", r.URL.Query().Get("name"))
		assert.Equal(t, "token secret", r.Header.Get("Authorization"))
		assert.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data; boundary="))
		reader, err := r.MultipartReader()
		if !assert.NoError(t, err) {
			return
		}
		part, err := reader.NextPart()
		if !assert.NoError(t, err) {
			return
		}
		assert.Equal(t, "attachment", part.FormName())
		assert.Equal(t, "shot 1.png", part.FileName())
		data, _ := io.ReadAll(part)
		assert.Equal(t, "\x89PNG fake", string(data))
		_, err = reader.NextPart()
		assert.ErrorIs(t, err, io.EOF)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":42,"name":"shot 1.png","size":9,"uuid":"u-1","browser_download_url":"http://f/attachments/u-1"}`)
	}))
	defer server.Close()

	att, err := New(server.URL, "secret").Attachments.Upload(context.Background(), "o w", "r", 7, "shot 1.png", strings.NewReader("\x89PNG fake"))
	require.NoError(t, err)
	require.Equal(t, int64(42), att.ID)
	require.Equal(t, "http://f/attachments/u-1", att.BrowserDownloadURL)
}

func TestAttachmentsUploadErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"secret"}`)
	}))
	defer server.Close()
	client := New(server.URL, "secret")

	_, err := client.Attachments.Upload(context.Background(), "o", "r", 1, "a.exe", strings.NewReader("MZ"))
	var typed *Error
	require.ErrorAs(t, err, &typed)
	require.Equal(t, http.StatusUnprocessableEntity, typed.StatusCode)
	require.NotContains(t, err.Error(), "secret")

	_, err = client.Attachments.Upload(context.Background(), "o", "r", 1, "", strings.NewReader("x"))
	require.Error(t, err)

	_, err = client.Attachments.Upload(context.Background(), "o", "r", 1, "a.png", io.MultiReader(strings.NewReader("x"), errReader{}))
	require.Error(t, err)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
