-- news content model: platform-neutral articles (PRD NB-03) and per-title
-- Switch BCAT datasets. Empty audience array = "all".
CREATE TABLE articles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         TEXT NOT NULL UNIQUE,
    platforms    TEXT[] NOT NULL DEFAULT '{}',
    title_ids    TEXT[] NOT NULL DEFAULT '{}',
    regions      TEXT[] NOT NULL DEFAULT '{}', -- ISO 3166-1 alpha-2
    languages    TEXT[] NOT NULL DEFAULT '{}', -- BCP 47; also the locale contract
    published_at TIMESTAMPTZ, -- NULL = draft, never served
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX articles_audience_idx ON articles USING gin (platforms, title_ids, regions);
CREATE INDEX articles_published_idx ON articles (published_at DESC);

CREATE TABLE article_locales (
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    language   TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL,
    PRIMARY KEY (article_id, language)
);

CREATE TABLE article_images (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id UUID NOT NULL REFERENCES articles(id) ON DELETE CASCADE,
    language   TEXT NOT NULL DEFAULT '', -- '' = all languages
    sort       INT NOT NULL DEFAULT 0,
    object_key TEXT NOT NULL,
    file_name  TEXT NOT NULL,
    size       BIGINT NOT NULL,
    sha256     TEXT NOT NULL,
    md5        TEXT NOT NULL
);

CREATE INDEX article_images_article_idx ON article_images(article_id);
CREATE INDEX article_images_key_idx ON article_images(object_key);

-- Switch-only by nature (PRD §1): per-title BCAT datasets.
CREATE TABLE datasets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title_id    TEXT NOT NULL,
    valid_from  TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_until TIMESTAMPTZ,
    sealed      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX datasets_served_idx ON datasets(title_id, sealed, valid_from DESC);

CREATE TABLE dataset_files (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dataset_id UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    path       TEXT NOT NULL,
    object_key TEXT NOT NULL,
    size       BIGINT NOT NULL,
    sha256     TEXT NOT NULL,
    UNIQUE (dataset_id, path)
);
