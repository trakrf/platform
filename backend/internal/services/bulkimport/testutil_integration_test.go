//go:build integration
// +build integration

package bulkimport

import (
	"bytes"
	"mime/multipart"
	"testing"
)

func createTestCSV(t *testing.T, content string) (multipart.File, *multipart.FileHeader) {
	t.Helper()

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	part, err := writer.CreateFormFile("file", "test.csv")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}

	writer.Close()

	reader := multipart.NewReader(&b, writer.Boundary())
	form, err := reader.ReadForm(10 * 1024 * 1024)
	if err != nil {
		t.Fatal(err)
	}

	files := form.File["file"]
	if len(files) == 0 {
		t.Fatal("no files in form")
	}

	file, err := files[0].Open()
	if err != nil {
		t.Fatal(err)
	}

	return file, files[0]
}
