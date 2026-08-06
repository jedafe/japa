package worker

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// PlatformDataExportProcessor defines the interface for platform data export
// background task processing (Section 67). The dataexport.Service implements
// this interface and registers with the worker.
type PlatformDataExportProcessor interface {
	ProcessExport(ctx context.Context, exportID pgtype.UUID) error
}

// PlatformDataImportProcessor defines the interface for platform data import
// background task processing (Section 67). The dataimport.Service implements
// this interface and registers with the worker.
type PlatformDataImportProcessor interface {
	ProcessImport(ctx context.Context, importID pgtype.UUID) error
}

var platformDataExportProcessor PlatformDataExportProcessor
var platformDataImportProcessor PlatformDataImportProcessor

// RegisterPlatformDataExportProcessor registers the data export service.
func RegisterPlatformDataExportProcessor(p PlatformDataExportProcessor) {
	platformDataExportProcessor = p
}

// RegisterPlatformDataImportProcessor registers the data import service.
func RegisterPlatformDataImportProcessor(p PlatformDataImportProcessor) {
	platformDataImportProcessor = p
}
