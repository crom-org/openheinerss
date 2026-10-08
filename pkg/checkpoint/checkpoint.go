package checkpoint

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/crom-org/openheinerss/pkg/config"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CheckpointInfo struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}
type Manager struct{}

var defaultManager = &Manager{}

func GetManager() *Manager { return defaultManager }

// CreateCheckpoint fotografa arquivos regulares; não usa git stash/checkout.
func (m *Manager) CreateCheckpoint(cwd, sessionID, message string) (*CheckpointInfo, error) {
	dir := filepath.Join(cwd, config.WorkspaceDirName, config.CheckpointsDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	id := "cp_" + hex.EncodeToString(b)
	info := &CheckpointInfo{ID: id, SessionID: sessionID, Message: message, CreatedAt: time.Now().Format(time.RFC3339)}
	root := filepath.Join(dir, id, "files")
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	err := filepath.Walk(cwd, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			// Árvores compartilhadas (/tmp, por exemplo) podem conter diretórios
			// que desaparecem ou não permitem leitura; o snapshot continua útil
			// com os arquivos acessíveis.
			return nil
		}
		rel, _ := filepath.Rel(cwd, path)
		if rel == "." {
			return nil
		}
		if filepath.Clean(cwd) == filepath.Clean(os.TempDir()) {
			return nil
		}
		if rel == config.WorkspaceDirName || strings.HasPrefix(rel, config.WorkspaceDirName+string(os.PathSeparator)) || rel == ".git" || strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) {
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if fi.IsDir() || !fi.Mode().IsRegular() {
			return nil
		}
		dst := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		return copyFile(path, dst, fi.Mode().Perm())
	})
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(info)
	if err := os.WriteFile(filepath.Join(dir, id+".json"), append(data, '\n'), 0644); err != nil {
		return nil, err
	}
	return info, nil
}

// RestoreCheckpoint repõe o snapshot sem executar comandos git nem apagar arquivos novos.
func (m *Manager) RestoreCheckpoint(cwd, id string) error {
	dir := filepath.Join(cwd, config.WorkspaceDirName, config.CheckpointsDirName)
	if _, err := os.Stat(filepath.Join(dir, id+".json")); err != nil {
		return fmt.Errorf("checkpoint '%s' não encontrado", id)
	}
	root := filepath.Join(dir, id, "files")
	return filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		dst := filepath.Join(cwd, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return err
		}
		return copyFile(path, dst, fi.Mode().Perm())
	})
}
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if e := out.Close(); err == nil {
		err = e
	}
	return err
}
