package workbench

import (
	"encoding/hex"
	"fmt"

	"entity-workbench-go/entitysdk"
)

// FormatHexDump formats raw bytes as 64-character hex lines.
func FormatHexDump(data []byte) []string {
	hexStr := hex.EncodeToString(data)
	var lines []string
	for i := 0; i < len(hexStr); i += 64 {
		end := i + 64
		if end > len(hexStr) {
			end = len(hexStr)
		}
		lines = append(lines, hexStr[i:end])
	}
	return lines
}

// FormatEntitySummary returns a one-line summary of an entity.
func FormatEntitySummary(path, entityType string, dataSize int) string {
	return fmt.Sprintf("%s  %s  %d bytes", path, entityType, dataSize)
}

// EntityHeader is the standard one-entity summary used by detail
// views: path, type, hash, size.
type EntityHeader struct {
	Path string
	Type string
	Hash string
	Size int
}

// HeaderFromResolved builds an EntityHeader from a ResolvedEntity.
func HeaderFromResolved(r entitysdk.ResolvedEntity) EntityHeader {
	return EntityHeader{
		Path: r.Path,
		Type: r.Entity.Type,
		Hash: r.Entity.ContentHash.String(),
		Size: len(r.Entity.Data),
	}
}

// FormatDiagnoseHash returns the hash diagnostic output for an entity.
func FormatDiagnoseHash(r entitysdk.ResolvedEntity) string {
	return r.Entity.DiagnoseHash()
}
