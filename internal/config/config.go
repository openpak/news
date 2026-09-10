// Package config loads the news service configuration from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL   string // required
	GRPCAddr      string // authoring gRPC (ports.md: +1)
	HTTPAddr      string // standalone HTTP, read API + renderer (ports.md: +0)
	MetricsAddr   string // metrics/pprof (ports.md: +2)
	OperatorKey   string // required, shared credential for the authoring gRPC
	ObjectsDir    string // filesystem object store root
	PublicBaseURL string // base URL clients fetch images and dataset files from (ports.md 20070)
	MaxImageSize  int64  // 8 MiB
	MaxFileSize   int64  // 64 MiB
}

func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:   os.Getenv("NEWS_DATABASE_URL"),
		GRPCAddr:      getenv("NEWS_GRPC_ADDR", ":20041"),
		HTTPAddr:      getenv("NEWS_HTTP_ADDR", ":20040"),
		MetricsAddr:   getenv("NEWS_METRICS_ADDR", ":20042"),
		OperatorKey:   os.Getenv("NEWS_OPERATOR_KEY"),
		ObjectsDir:    getenv("NEWS_OBJECTS_DIR", "./data/objects"),
		PublicBaseURL: strings.TrimRight(os.Getenv("NEWS_PUBLIC_BASE_URL"), "/"),
		MaxImageSize:  8 << 20,
		MaxFileSize:   64 << 20,
	}
	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "NEWS_DATABASE_URL")
	}
	if c.OperatorKey == "" || len(c.OperatorKey) < 32 {
		missing = append(missing, "NEWS_OPERATOR_KEY (>=32 chars)")
	}
	if c.PublicBaseURL == "" {
		missing = append(missing, "NEWS_PUBLIC_BASE_URL (base URL clients fetch images and dataset files from, e.g. http://localhost:20070)")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing env: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// ValidateImageType reports whether the file name has an accepted image extension.
func ValidateImageType(name string) error {
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 {
		return errors.New("file name has no extension")
	}
	switch strings.ToLower(name[dot:]) {
	case ".jpg", ".jpeg", ".png", ".webp":
		return nil
	}
	return errors.New("unsupported image type (jpeg, png, webp only)")
}
