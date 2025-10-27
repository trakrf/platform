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
	MaxFileSize = 5 * 1024 * 1024 // 5MB
	MaxRows     = 1000            // Maximum rows per CSV
)

// Allowed MIME types for CSV files
var allowedMIMETypes = map[string]bool{
	"text/csv":                 true,
	"application/vnd.ms-excel": true,
	"application/csv":          true,
	"text/plain":               true, // Some systems send CSV as text/plain
}

// Validator handles file and CSV validation logic
type Validator struct{}

// NewValidator creates a new Validator instance
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateFile validates file size, extension, and MIME type
func (v *Validator) ValidateFile(file multipart.File, header *multipart.FileHeader) error {
	// Validate size
	if header.Size > MaxFileSize {
		return fmt.Errorf("file too large: %d bytes (max %d bytes / 5MB)", header.Size, MaxFileSize)
	}

	// Validate extension
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".csv") {
		return fmt.Errorf("invalid file extension: must be .csv")
	}

	// Validate MIME type
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		// Try to detect from file
		buffer := make([]byte, 512)
		_, err := file.Read(buffer)
		if err != nil {
			return fmt.Errorf("failed to read file for type detection: %w", err)
		}
		contentType = http.DetectContentType(buffer)
		// Reset file pointer
		file.Seek(0, 0)
	}

	if !allowedMIMETypes[contentType] {
		return fmt.Errorf("invalid MIME type: %s (expected text/csv or application/vnd.ms-excel)", contentType)
	}

	return nil
}

// ParseAndValidateCSV reads, parses, and validates CSV content
func (v *Validator) ParseAndValidateCSV(file multipart.File) ([][]string, []string, error) {
	// Read all CSV content into memory (safe due to 5MB limit)
	csvContent, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CSV file: %w", err)
	}

	// Parse CSV
	csvReader := csv.NewReader(bytes.NewReader(csvContent))
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("invalid CSV format: %w", err)
	}

	// Validate minimum content
	if len(records) < 1 {
		return nil, nil, fmt.Errorf("CSV file is empty")
	}

	// Validate headers (using csvutil.ValidateCSVHeaders from util/csv package)
	headers := records[0]
	if err := csvutil.ValidateCSVHeaders(headers); err != nil {
		return nil, nil, fmt.Errorf("invalid CSV headers: %w", err)
	}

	// Count data rows (exclude header)
	totalRows := len(records) - 1
	if totalRows == 0 {
		return nil, nil, fmt.Errorf("CSV has headers but no data rows")
	}

	// Validate row limit
	if totalRows > MaxRows {
		return nil, nil, fmt.Errorf("too many rows: %d (max %d)", totalRows, MaxRows)
	}

	return records, headers, nil
}
