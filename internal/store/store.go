// Package store is the PostgreSQL implementation of the news content
// model (content.Reader + content.Writer). Objects (images, dataset
// files) live in the injected objects.Store.
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openpak/news/internal/content"
	"github.com/openpak/news/internal/objects"
	"github.com/openpak/news/internal/store/migrations"
)

type Store struct {
	pool *pgxpool.Pool
	objs objects.Store
}

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 8
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Migrate applies embedded migrations in order, one tx each.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := migrationNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		var applied bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func New(pool *pgxpool.Pool, objs objects.Store) *Store { return &Store{pool: pool, objs: objs} }

// emptyNotNIL keeps Go nil slices out of NOT NULL array columns: nil ("all")
// must land as '{}', not NULL.
func emptyNotNIL(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func sanitized(a content.Audience) content.Audience {
	return content.Audience{
		Platforms: emptyNotNIL(a.Platforms),
		TitleIDs:  emptyNotNIL(a.TitleIDs),
		Regions:   emptyNotNIL(a.Regions),
		Languages: emptyNotNIL(a.Languages),
	}
}

// ---------- Reader ----------

// Articles returns published articles visible to the audience query.
// Visibility per dimension: article targeting empty ("all") always
// matches; otherwise the query value must be a member.
func (s *Store) Articles(ctx context.Context, q content.AudienceQuery) ([]content.Article, time.Time, error) {
	where := `WHERE published_at IS NOT NULL AND published_at <= now()
		AND (expires_at IS NULL OR expires_at > now())`
	args := []any{}
	add := func(cond string, vals ...any) {
		for _, v := range vals {
			args = append(args, v)
			cond = strings.Replace(cond, "?", "$"+fmt.Sprint(len(args)), 1)
		}
		where += ` AND ` + cond
	}
	if q.Platform != "" {
		add(`(platforms = '{}' OR ? = ANY(platforms))`, strings.ToLower(q.Platform))
	}
	if q.TitleID != "" {
		add(`(title_ids = '{}' OR ? = ANY(title_ids))`, q.TitleID)
	}
	if len(q.Regions) > 0 {
		upper := make([]string, len(q.Regions))
		for i, r := range q.Regions {
			upper[i] = strings.ToUpper(r)
		}
		add(`(regions = '{}' OR regions && ?)`, upper)
	}
	if q.Language != "" {
		add(`(languages = '{}' OR ? = ANY(languages))`, q.Language)
	}
	rows, err := s.pool.Query(ctx, `SELECT id, slug, platforms, title_ids, regions, languages,
		published_at, expires_at, created_at, updated_at FROM articles `+where+`
		ORDER BY published_at DESC, created_at DESC`, args...)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()
	var out []content.Article
	var maxMod time.Time
	for rows.Next() {
		var a content.Article
		var published, expires *time.Time
		if err := rows.Scan(&a.ID, &a.Slug, &a.Audience.Platforms, &a.Audience.TitleIDs,
			&a.Audience.Regions, &a.Audience.Languages, &published, &expires, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, time.Time{}, err
		}
		a.PublishedAt, a.ExpiresAt = published, expires
		if a.UpdatedAt.After(maxMod) {
			maxMod = a.UpdatedAt
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, err
	}
	for i := range out {
		if err := s.loadLocalesImages(ctx, &out[i]); err != nil {
			return nil, time.Time{}, err
		}
	}
	return out, maxMod, nil
}

func (s *Store) Titles(ctx context.Context) ([]string, time.Time, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT title_id, created_at FROM datasets
		WHERE sealed AND valid_from <= now()
		  AND (valid_until IS NULL OR valid_until > now())`)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()
	var titles []string
	var maxMod time.Time
	for rows.Next() {
		var tid string
		var created time.Time
		if err := rows.Scan(&tid, &created); err != nil {
			return nil, time.Time{}, err
		}
		if created.After(maxMod) {
			maxMod = created
		}
		titles = append(titles, tid)
	}
	return titles, maxMod, rows.Err()
}

func (s *Store) CurrentDataset(ctx context.Context, titleID string) (*content.Dataset, time.Time, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, title_id, valid_from, valid_until, created_at FROM datasets
		WHERE title_id = $1 AND sealed AND valid_from <= now()
		  AND (valid_until IS NULL OR valid_until > now())
		ORDER BY valid_from DESC, created_at DESC LIMIT 1`, titleID)
	d := &content.Dataset{}
	var validUntil *time.Time
	var created time.Time
	if err := row.Scan(&d.ID, &d.TitleID, &d.ValidFrom, &validUntil, &created); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, time.Time{}, content.ErrNotFound
		}
		return nil, time.Time{}, err
	}
	d.ValidUntil = validUntil
	d.Sealed = true
	files, err := s.datasetFiles(ctx, d.ID)
	if err != nil {
		return nil, time.Time{}, err
	}
	d.Files = files
	return d, created, nil
}

func (s *Store) datasetFiles(ctx context.Context, datasetID string) ([]content.DatasetFile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT path, object_key, size, sha256 FROM dataset_files
		WHERE dataset_id = $1 ORDER BY path`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []content.DatasetFile
	for rows.Next() {
		var f content.DatasetFile
		if err := rows.Scan(&f.Path, &f.Key, &f.Size, &f.SHA256); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) ImageByKey(ctx context.Context, key string) (*content.ArticleImage, time.Time, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT i.id, i.language, i.object_key, i.file_name, i.size, i.sha256, i.md5, a.updated_at
		FROM article_images i JOIN articles a ON a.id = i.article_id
		WHERE i.object_key = $1`, key)
	img := &content.ArticleImage{}
	var updated time.Time
	if err := row.Scan(&img.ID, &img.Language, &img.Key, &img.FileName, &img.Size, &img.SHA256, &img.MD5, &updated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, time.Time{}, content.ErrNotFound
		}
		return nil, time.Time{}, err
	}
	return img, updated, nil
}

func (s *Store) OpenObject(ctx context.Context, key string) (io.ReadSeekCloser, int64, time.Time, error) {
	rc, info, err := s.objs.Open(ctx, key)
	if err != nil {
		return nil, 0, time.Time{}, err
	}
	return rc, info.Size, info.ModTime, nil
}

// ---------- Writer ----------

func (s *Store) CreateArticle(ctx context.Context, a *content.Article) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	aud := sanitized(a.Audience)
	err = tx.QueryRow(ctx, `
		INSERT INTO articles (slug, platforms, title_ids, regions, languages, published_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at, updated_at`,
		a.Slug, aud.Platforms, aud.TitleIDs, aud.Regions, aud.Languages,
		a.PublishedAt, a.ExpiresAt).
		Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return err
	}
	for _, l := range a.Locales {
		if _, err := tx.Exec(ctx, `INSERT INTO article_locales (article_id, language, title, body)
			VALUES ($1, $2, $3, $4)`, a.ID, l.Language, l.Title, l.Body); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) UpdateArticle(ctx context.Context, id string, mutate func(*content.Article) error) error {
	a, err := s.Article(ctx, id)
	if err != nil {
		return err
	}
	if err := mutate(a); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	aud := sanitized(a.Audience)
	if _, err := tx.Exec(ctx, `UPDATE articles SET platforms = $2, title_ids = $3, regions = $4,
		languages = $5, updated_at = now() WHERE id = $1`,
		a.ID, aud.Platforms, aud.TitleIDs, aud.Regions, aud.Languages); err != nil {
		return err
	}
	for _, l := range a.Locales {
		if _, err := tx.Exec(ctx, `INSERT INTO article_locales (article_id, language, title, body)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (article_id, language) DO UPDATE SET title = EXCLUDED.title, body = EXCLUDED.body`,
			a.ID, l.Language, l.Title, l.Body); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) SetPublication(ctx context.Context, id string, published, expires *time.Time) error {
	ct, err := s.pool.Exec(ctx, `UPDATE articles SET published_at = $2, expires_at = $3, updated_at = now() WHERE id = $1`,
		id, published, expires)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return content.ErrNotFound
	}
	return nil
}

const articleCols = `id, slug, platforms, title_ids, regions, languages, published_at, expires_at, created_at, updated_at`

func articleFromRow(scan func(dest ...any) error) (*content.Article, error) {
	a := &content.Article{}
	var published, expires *time.Time
	if err := scan(&a.ID, &a.Slug, &a.Audience.Platforms, &a.Audience.TitleIDs,
		&a.Audience.Regions, &a.Audience.Languages, &published, &expires, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	a.PublishedAt, a.ExpiresAt = published, expires
	return a, nil
}

func (s *Store) Article(ctx context.Context, id string) (*content.Article, error) {
	a, err := articleFromRow(func(dest ...any) error {
		return s.pool.QueryRow(ctx, `SELECT `+articleCols+` FROM articles WHERE id = $1`, id).Scan(dest...)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, content.ErrNotFound
		}
		return nil, err
	}
	if err := s.loadLocalesImages(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) ArticleBySlug(ctx context.Context, slug string) (*content.Article, error) {
	a, err := articleFromRow(func(dest ...any) error {
		return s.pool.QueryRow(ctx, `SELECT `+articleCols+` FROM articles WHERE slug = $1`, slug).Scan(dest...)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, content.ErrNotFound
		}
		return nil, err
	}
	if err := s.loadLocalesImages(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// ListArticles (Writer side) lists articles for authoring, optionally filtered.
func (s *Store) ListArticles(ctx context.Context, includeUnpublished bool, platform, titleID string) ([]*content.Article, error) {
	q := `SELECT ` + articleCols + ` FROM articles WHERE TRUE`
	args := []any{}
	if !includeUnpublished {
		q += ` AND published_at IS NOT NULL`
	}
	if platform != "" {
		args = append(args, strings.ToLower(platform))
		q += fmt.Sprintf(` AND (platforms = '{}' OR $%d = ANY(platforms))`, len(args))
	}
	if titleID != "" {
		args = append(args, titleID)
		q += fmt.Sprintf(` AND (title_ids = '{}' OR $%d = ANY(title_ids))`, len(args))
	}
	q += ` ORDER BY published_at DESC NULLS LAST, created_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*content.Article
	for rows.Next() {
		a, err := articleFromRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, a := range out {
		if err := s.loadLocalesImages(ctx, a); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) loadLocalesImages(ctx context.Context, a *content.Article) error {
	lrows, err := s.pool.Query(ctx, `SELECT language, title, body FROM article_locales
		WHERE article_id = $1 ORDER BY language`, a.ID)
	if err != nil {
		return err
	}
	a.Locales = nil
	for lrows.Next() {
		var l content.ArticleLocale
		if err := lrows.Scan(&l.Language, &l.Title, &l.Body); err != nil {
			lrows.Close()
			return err
		}
		a.Locales = append(a.Locales, l)
	}
	if err := lrows.Err(); err != nil {
		return err
	}
	lrows.Close()

	irows, err := s.pool.Query(ctx, `SELECT id, language, object_key, file_name, size, sha256, md5
		FROM article_images WHERE article_id = $1 ORDER BY sort, id`, a.ID)
	if err != nil {
		return err
	}
	defer irows.Close()
	for irows.Next() {
		var img content.ArticleImage
		if err := irows.Scan(&img.ID, &img.Language, &img.Key, &img.FileName, &img.Size, &img.SHA256, &img.MD5); err != nil {
			return err
		}
		a.Images = append(a.Images, img)
	}
	return irows.Err()
}

func (s *Store) AttachImage(ctx context.Context, articleID string, img *content.ArticleImage) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO article_images (article_id, language, sort, object_key, file_name, size, sha256, md5)
		VALUES ($1, $2, COALESCE((SELECT MAX(sort) + 1 FROM article_images WHERE article_id = $1), 0), $3, $4, $5, $6, $7)
		RETURNING id`,
		articleID, img.Language, img.Key, img.FileName, img.Size, img.SHA256, img.MD5).Scan(&img.ID)
}

func (s *Store) CreateDataset(ctx context.Context, d *content.Dataset) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO datasets (title_id, valid_from, valid_until)
		VALUES ($1, $2, $3) RETURNING id, created_at`,
		d.TitleID, d.ValidFrom, d.ValidUntil).Scan(&d.ID, &d.ValidFrom)
}

func (s *Store) Dataset(ctx context.Context, id string) (*content.Dataset, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, title_id, valid_from, valid_until, sealed FROM datasets WHERE id = $1`, id)
	d := &content.Dataset{}
	var validUntil *time.Time
	if err := row.Scan(&d.ID, &d.TitleID, &d.ValidFrom, &validUntil, &d.Sealed); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, content.ErrNotFound
		}
		return nil, err
	}
	d.ValidUntil = validUntil
	files, err := s.datasetFiles(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	d.Files = files
	return d, nil
}

func (s *Store) Datasets(ctx context.Context, titleID string) ([]*content.Dataset, error) {
	q := `SELECT id, title_id, valid_from, valid_until, sealed FROM datasets`
	args := []any{}
	if titleID != "" {
		q += ` WHERE title_id = $1`
		args = append(args, titleID)
	}
	q += ` ORDER BY valid_from DESC, created_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*content.Dataset
	for rows.Next() {
		d := &content.Dataset{}
		var validUntil *time.Time
		if err := rows.Scan(&d.ID, &d.TitleID, &d.ValidFrom, &validUntil, &d.Sealed); err != nil {
			return nil, err
		}
		d.ValidUntil = validUntil
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, d := range out {
		if d.Files, err = s.datasetFiles(ctx, d.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) PutDatasetFile(ctx context.Context, datasetID string, f content.DatasetFile) error {
	d, err := s.Dataset(ctx, datasetID)
	if err != nil {
		return err
	}
	if d.Sealed {
		return content.ErrConflict
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO dataset_files (dataset_id, path, object_key, size, sha256)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (dataset_id, path) DO UPDATE
		SET object_key = EXCLUDED.object_key, size = EXCLUDED.size, sha256 = EXCLUDED.sha256`,
		datasetID, f.Path, f.Key, f.Size, f.SHA256)
	return err
}

func (s *Store) SealDataset(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE datasets SET sealed = TRUE WHERE id = $1
		AND EXISTS (SELECT 1 FROM dataset_files WHERE dataset_id = $1)`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		if _, err := s.Dataset(ctx, id); err != nil {
			return err
		}
		return content.ErrValidation
	}
	return nil
}

// TopicID derives a stable public topic id: sha256(articleID + language),
// 16 lowercase hex chars.
func TopicID(articleID, language string) string {
	h := sha256.Sum256([]byte(articleID + "|" + language))
	return hex.EncodeToString(h[:8])
}
