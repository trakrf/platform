package bulkimport

import (
	"bytes"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestCSV(t *testing.T, content string) (multipart.File, *multipart.FileHeader) {
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	h := make(map[string][]string)
	h["Content-Type"] = []string{"text/csv"}
	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="file"; filename="test.csv"`},
		"Content-Type":        {"text/csv"},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = part.Write([]byte(content))
	if err != nil {
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

func TestValidator_FileTooLarge(t *testing.T) {
	v := NewValidator()

	content := strings.Repeat("a", 6*1024*1024)
	file, header := createTestCSV(t, content)
	defer file.Close()

	err := v.ValidateFile(file, header)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large", "Error should mention file size limit")
}

func TestValidator_InvalidExtension(t *testing.T) {
	v := NewValidator()

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	part, err := writer.CreateFormFile("file", "test.txt")
	require.NoError(t, err)

	_, err = part.Write([]byte("test content"))
	require.NoError(t, err)

	writer.Close()

	reader := multipart.NewReader(&b, writer.Boundary())
	form, err := reader.ReadForm(10 * 1024 * 1024)
	require.NoError(t, err)

	files := form.File["file"]
	require.NotEmpty(t, files)

	file, err := files[0].Open()
	require.NoError(t, err)
	defer file.Close()

	err = v.ValidateFile(file, files[0])
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "extension") || strings.Contains(err.Error(), ".csv"),
		"Error should mention file extension requirement, got: %v", err)
}

func TestValidator_InvalidMIMEType(t *testing.T) {
	v := NewValidator()

	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	part, err := writer.CreateFormFile("file", "test.pdf")
	require.NoError(t, err)

	_, err = part.Write([]byte("%PDF-1.4"))
	require.NoError(t, err)

	writer.Close()

	reader := multipart.NewReader(&b, writer.Boundary())
	form, err := reader.ReadForm(10 * 1024 * 1024)
	require.NoError(t, err)

	files := form.File["file"]
	file, err := files[0].Open()
	require.NoError(t, err)
	defer file.Close()

	err = v.ValidateFile(file, files[0])
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "extension") || strings.Contains(err.Error(), ".csv"),
		"Error should indicate invalid file type for PDF, got: %v", err)
}

func TestValidator_ValidCSVFile(t *testing.T) {
	v := NewValidator()

	content := "identifier,name,type\nTEST-001,Test,device"
	file, header := createTestCSV(t, content)
	defer file.Close()

	err := v.ValidateFile(file, header)
	assert.NoError(t, err, "Valid CSV file should pass validation")
}

func TestValidator_EmptyCSVFile(t *testing.T) {
	v := NewValidator()

	file, header := createTestCSV(t, "")
	defer file.Close()

	err := v.ValidateFile(file, header)
	assert.NoError(t, err, "Empty CSV file passes file validation (content validation happens during parsing)")
}
