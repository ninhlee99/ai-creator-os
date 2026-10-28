package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func multipartUploadRequest(t *testing.T, field string, files ...[2]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range files {
		part, err := writer.CreateFormFile(field, file[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(file[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestSaveUploadsRemovesEarlierFilesWhenLaterFileIsInvalid(t *testing.T) {
	dir := t.TempDir()
	req := multipartUploadRequest(t, "photos", [2]string{"first.jpg", "image"}, [2]string{"bad.txt", "not image"})
	if _, err := saveUploads(req, "photos", dir, 10); err == nil {
		t.Fatal("saveUploads accepted an unsupported file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial upload left files behind: %v", entries)
	}
}

func TestSaveMotionClipsRemovesEarlierFilesWhenLaterClipIsInvalid(t *testing.T) {
	dir := t.TempDir()
	req := multipartUploadRequest(t, "clips", [2]string{"first.mp4", "video"}, [2]string{"bad.txt", "not video"})
	if _, err := saveMotionClips(req, "clips", dir, 30); err == nil {
		t.Fatal("saveMotionClips accepted an unsupported file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("partial clip upload left files behind: %v", entries)
	}
}

func TestSaveMotionClipsKeepsFilesOnSuccess(t *testing.T) {
	dir := t.TempDir()
	req := multipartUploadRequest(t, "clips", [2]string{"first.mp4", "video"})
	paths, err := saveMotionClips(req, "clips", dir, 30)
	if err != nil || len(paths) != 1 {
		t.Fatalf("saveMotionClips = (%v, %v), want one saved clip", paths, err)
	}
	if _, err := os.Stat(paths[0]); err != nil {
		t.Fatalf("saved clip missing after successful upload: %v", err)
	}
}
