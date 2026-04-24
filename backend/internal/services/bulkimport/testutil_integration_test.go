//go:build integration
// +build integration

package bulkimport

import (
	"bytes"
	"mime/multipart"
	"net/textproto"
	"testing"
)

func createTestCSV(t *testing.T, content string) (multipart.File, *multipart.FileHeader) {
	t.Helper()

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	// CreateFormFile hardcodes Content-Type=application/octet-stream, which the
	// upload validator rejects. Set text/csv explicitly so the helper exercises
	// the validator's happy path.
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="test.csv"`)
	header.Set("Content-Type", "text/csv")
	part, err := writer.CreatePart(header)
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
