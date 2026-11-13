package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

type SelectedFile struct {
	// The struct descibes files which must
	FullPath     string
	SourcePath   string
	TeamID       *int64
	Hash         string
	IndexVersion string
}

func GetFilePaths(knowledgeDir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(knowledgeDir, func(path string, entry fs.DirEntry, err error) error {

		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		if entry.Type().IsRegular() &&
			strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

func FileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func FileSelection(knowledgeDir string, indexversion string, ctx context.Context, db *pgx.Conn) ([]SelectedFile, error) {
	// var result map[string]DocumentMetadata
	filesMetadata, err := LoadDocumentMetadata(ctx, db)
	if err != nil {
		return []SelectedFile{}, err
	}
	paths, err := GetFilePaths(knowledgeDir)
	if err != nil {
		return []SelectedFile{}, err
	}
	var selectedFiles []SelectedFile
	for _, path := range paths {
		hash, err := FileHash(path)
		if err != nil {
			return []SelectedFile{}, err
		}
		relativePath, err := filepath.Rel(knowledgeDir, path)
		if err != nil {
			return nil, err
		}
		sourcePath := filepath.ToSlash(relativePath)
		folder, _, found := strings.Cut(sourcePath, "/")
		if !found {
			return nil, fmt.Errorf("%s: expected a file under common/ or team-*/", sourcePath)
		}

		var teamID *int64
		switch {
		case folder == "common":
		case strings.HasPrefix(folder, "team-") && folder != "team-":
			teamCode := strings.TrimPrefix(folder, "team-")
			id, err := GetTeamId(ctx, db, teamCode)
			if err != nil {
				return nil, fmt.Errorf("%s: find team %q: %w", sourcePath, teamCode, err)
			}
			teamID = &id
		default:
			return nil, fmt.Errorf("%s: unknown knowledge folder %q", sourcePath, folder)
		}

		metadata := filesMetadata[sourcePath]
		if metadata.ContentHash != hash || metadata.IndexingVersion != indexversion {
			builder := SelectedFile{
				FullPath:     path,
				SourcePath:   sourcePath,
				TeamID:       teamID,
				Hash:         hash,
				IndexVersion: indexversion,
			}
			selectedFiles = append(selectedFiles, builder)
		}

	}
	return selectedFiles, nil
}

func ReadFileText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
