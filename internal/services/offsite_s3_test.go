package services

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"arsippro/internal/config"
)

type listBucketResult struct {
	XMLName     xml.Name `xml:"ListBucketResult"`
	Name        string   `xml:"Name"`
	Prefix      string   `xml:"Prefix"`
	KeyCount    int      `xml:"KeyCount"`
	MaxKeys     int      `xml:"MaxKeys"`
	IsTruncated bool     `xml:"IsTruncated"`
}

// mockS3 is a minimal S3-compatible endpoint sufficient for the operations the
// offsite backup performs: bucket existence, bucket location, list and PUT.
func mockS3(t *testing.T, puts *int, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		bucket := strings.SplitN(path, "/", 2)[0]
		q := r.URL.Query()

		switch {
		case r.Method == http.MethodHead && (path == bucket || path == bucket+"/"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && q.Get("location") != "":
			w.Header().Set("Content-Type", "application/xml")
			io.WriteString(w, xml.Header+`<LocationConstraint></LocationConstraint>`)
		case r.Method == http.MethodGet && q.Get("list-type") == "2":
			w.Header().Set("Content-Type", "application/xml")
			_ = xml.NewEncoder(w).Encode(listBucketResult{Name: bucket, KeyCount: 0, MaxKeys: 1})
		case r.Method == http.MethodGet && (path == bucket || path == bucket+"/"):
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut:
			mu.Lock()
			*puts++
			mu.Unlock()
			w.Header().Set("ETag", `"d41d8cd98f00b204e9800998ecf8427e"`)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestS3MirrorAgainstMock(t *testing.T) {
	var puts int
	var mu sync.Mutex
	srv := mockS3(t, &puts, &mu)
	defer srv.Close()

	// Run in a temp working dir so config.UploadDir() resolves under it.
	oldWd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	up := filepath.Join("storage", "app", "uploads", "sub")
	if err := os.MkdirAll(up, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(up, "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.OffsiteConfig{
		S3Endpoint:  srv.URL,
		S3Region:    "us-east-1",
		S3Bucket:    "simarc-backup",
		S3AccessKey: "test",
		S3SecretKey: "test",
		S3Prefix:    "simarc",
		S3PathStyle: true,
	}

	if err := TestS3(cfg); err != nil {
		t.Fatalf("TestS3: %v", err)
	}

	count, bytes, err := uploadFilesToS3(cfg)
	if err != nil {
		t.Fatalf("uploadFilesToS3: %v", err)
	}
	if count != 1 || bytes != 5 {
		t.Fatalf("got count=%d bytes=%d, want 1 and 5", count, bytes)
	}
	if puts == 0 {
		t.Fatal("expected at least one PUT request")
	}
}

func TestS3NotReadyRejected(t *testing.T) {
	if _, err := newS3Client(config.OffsiteConfig{}); err == nil {
		t.Fatal("expected an error for incomplete settings")
	}
}
