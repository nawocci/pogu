package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	DefaultDataDir = "data"
	DefaultListen  = "127.0.0.1:8080"
	DefaultSocket  = "pogu.sock"
)

type Config struct {
	DataDir       string `json:"data_dir"`
	Listen        string `json:"listen"`
	ControlSock   string `json:"control_socket"`
	MasterKeyFile string `json:"master_key_file"`
	DatabaseFile  string `json:"database_file"`
}

func Default(dataDir string) Config {
	if dataDir == "" {
		dataDir = DefaultDataDir
	}
	return Config{
		DataDir:       dataDir,
		Listen:        DefaultListen,
		ControlSock:   filepath.Join(dataDir, DefaultSocket),
		MasterKeyFile: filepath.Join(dataDir, "master.key"),
		DatabaseFile:  filepath.Join(dataDir, "pogu.db"),
	}
}

func pathOf(dataDir string) string {
	if dataDir == "" {
		dataDir = DefaultDataDir
	}
	return filepath.Join(dataDir, "config.json")
}

func Load(dataDir string) (Config, error) {
	cfg := Default(dataDir)
	raw, err := os.ReadFile(pathOf(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return Config{}, err
	}
	var file Config
	if err := json.Unmarshal(raw, &file); err != nil {
		return Config{}, err
	}
	dir := file.DataDir
	if dir == "" {
		dir = cfg.DataDir
	}
	def := Default(dir)
	def.Listen = file.Listen
	if def.Listen == "" {
		def.Listen = DefaultListen
	}
	if file.ControlSock != "" {
		def.ControlSock = file.ControlSock
	}
	if file.MasterKeyFile != "" {
		def.MasterKeyFile = file.MasterKeyFile
	}
	if file.DatabaseFile != "" {
		def.DatabaseFile = file.DatabaseFile
	}
	return def, nil
}

func (c Config) Save() error {
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := filepath.Join(c.DataDir, "config.json.tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, pathOf(c.DataDir))
}
