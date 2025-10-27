package bulkimport

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	csvutil "github.com/trakrf/platform/backend/internal/util/csv"
)

const (
	MaxFileSize = 5 * 1024 * 1024
	MaxRows     = 1000
)

var allowedMIMETypes = map[string]bool{
	"text/csv":                 true,
	"application/vnd.ms-excel": true,
	"application/csv":          true,
	"text/plain":               true,
}

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) ValidateFile(file multipart.File, header *multipart.FileHeader) error {
	if header.Size > MaxFileSize {
		return fmt.Errorf("file too large: %d bytes (max %d bytes / 5MB)", header.Size, MaxFileSize)
	}

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".csv") {
		return fmt.Errorf("invalid file extension: must be .csv")
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		buffer := make([]byte, 512)
		_, err := file.Read(buffer)
		if err != nil {
			return fmt.Errorf("failed to read file for type detection: %w", err)
		}
		contentType = http.DetectContentType(buffer)
		file.Seek(0, 0)
	}

	if !allowedMIMETypes[contentType] {
		return fmt.Errorf("invalid MIME type: %s (expected text/csv or application/vnd.ms-excel)", contentType)
	}

	return nil
}

func (v *Validator) ParseAndValidateCSV(file multipart.File) ([][]string, []string, error) {
	csvContent, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV file: %w", err)
	}

	csvReader := csv.NewReader(bytes.NewReader(csvContent))
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("invalid CSV format: %w", err)
	}

	if len(records) < 1 {
		return nil, nil, fmt.Errorf("CSV file is empty")
	}

	headers := records[0]
	if err := csvutil.ValidateCSVHeaders(headers); err != nil {
		return nil, nil, fmt.Errorf("invalid CSV headers: %w", err)
	}

	totalRows := len(records) - 1
	if totalRows == 0 {
		return nil, nil, fmt.Errorf("CSV has headers but no data rows")
	}

	if totalRows > MaxRows {
		return nil, nil, fmt.Errorf("too many rows: %d (max %d)", totalRows, MaxRows)
	}

	return records, headers, nil
}
