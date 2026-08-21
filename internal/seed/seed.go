package seed

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/utkarsh/claim-identification/internal/model"
	"github.com/utkarsh/claim-identification/internal/store"
)

type Options struct {
	File             string
	MediaDir         string
	PublicBaseURL    string
	RewriteMediaURLs bool
	Overwrite        bool
}

var ErrNoSeedFile = errors.New("seed file not found")

func Load(ctx context.Context, st store.Store, opts Options, log *slog.Logger) (*model.Product, error) {
	raw, err := os.ReadFile(opts.File)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNoSeedFile, opts.File)
		}
		return nil, fmt.Errorf("read seed file: %w", err)
	}

	product, err := model.ParseProductDocument(raw)
	if err != nil {
		return nil, fmt.Errorf("parse seed file %s: %w", opts.File, err)
	}

	if opts.RewriteMediaURLs {
		rewriteMedia(product, opts, log)
	}

	if !opts.Overwrite {
		if existing, err := st.GetProduct(ctx, product.ID); err == nil {
			log.Info("product already stored, skipping seed",
				"productId", existing.ID, "claims", len(existing.Claims))
			return existing, nil
		} else if !errors.Is(err, store.ErrProductNotFound) {
			return nil, fmt.Errorf("check existing product: %w", err)
		}
	}

	if err := st.UpsertProduct(ctx, product); err != nil {
		return nil, fmt.Errorf("store seed product: %w", err)
	}

	log.Info("seeded product", "productId", product.ID, "title", product.Title, "media", len(product.Media))
	return product, nil
}

func rewriteMedia(product *model.Product, opts Options, log *slog.Logger) {
	base := strings.TrimSuffix(opts.PublicBaseURL, "/")

	for i := range product.Media {
		m := &product.Media[i]

		name := path.Base(m.Path)
		if name == "" || name == "." || name == "/" {
			continue
		}

		m.URL = base + "/media/" + name

		if opts.MediaDir != "" {
			if _, err := os.Stat(filepath.Join(opts.MediaDir, name)); err != nil {
				log.Warn("media file missing, URL will 404",
					"file", filepath.Join(opts.MediaDir, name),
					"hint", "run: go run ./cmd/genmedia")
			}
		}
	}
}
