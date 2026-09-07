// newsctl: minimal operator CLI for the news authoring gRPC (NB-04 stopgap
// until the website pages exist; also the owner's acceptance-journey tool).
//
// Examples:
//
//	newsctl -addr :7001 -key $NEWS_OPERATOR_KEY article create -slug hello \
//	  -platforms all -title 0100ed9024eb8000 -region FR -region GB \
//	  -locale en-US -title-text "OpenPak is live" -body-file body.txt
//	newsctl -key ... article publish -id <id> -expires 2027-01-01T00:00:00Z
//	newsctl -key ... image upload -id <id> -file art.jpg -language en-US
//	newsctl -key ... dataset create -title 0100ed9024eb8000 -until 2027-01-01T00:00:00Z
//	newsctl -key ... dataset put -dataset <id> -path event/season.bin -file season.bin
//	newsctl -key ... dataset seal -dataset <id>
//	newsctl -key ... article list -all
//	newsctl -key ... dataset list -title 0100ed9024eb8000
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	newsv1 "github.com/openpak/news/gen/news/v1"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "newsctl:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("newsctl", flag.ExitOnError)
	addr := fs.String("addr", "localhost:7001", "authoring gRPC address")
	key := fs.String("key", os.Getenv("NEWS_OPERATOR_KEY"), "operator key")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		return usage()
	}
	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	client := newsv1.NewNewsClient(conn)
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+*key)

	cmd, sub := fs.Arg(0), fs.Arg(1)
	rest := fs.Args()[2:]
	switch cmd + " " + sub {
	case "article create":
		return articleCreate(ctx, client, rest)
	case "article publish":
		return articlePublish(ctx, client, rest)
	case "article list":
		return articleList(ctx, client, rest)
	case "article preview":
		return articlePreview(ctx, client, rest)
	case "image upload":
		return imageUpload(ctx, client, rest)
	case "dataset create":
		return datasetCreate(ctx, client, rest)
	case "dataset put":
		return datasetPut(ctx, client, rest)
	case "dataset seal":
		return datasetSeal(ctx, client, rest)
	case "dataset list":
		return datasetList(ctx, client, rest)
	}
	return usage()
}

func usage() error {
	return fmt.Errorf(`usage: newsctl -addr <addr> -key <key> <command>
  article create -slug S [-platforms all|switch,...] [-title ID]... [-region CC]... [-locale LANG -title-text T -body-file F | -body-text T]...
  article publish -id ID [-at RFC3339] [-expires RFC3339]
  article list [-all] [-platform P] [-title ID]
  article preview -id ID
  image upload -id ID -file F [-language LANG]
  dataset create -title ID [-from RFC3339] [-until RFC3339]
  dataset put -dataset ID -path REL/PATH -file F
  dataset seal -dataset ID
  dataset list [-title ID]`)
}

func articleCreate(ctx context.Context, c newsv1.NewsClient, args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	slug := fs.String("slug", "", "article slug")
	var platforms, titles, regions multi
	var locales localeList
	var localeFiles multi
	fs.Var(&platforms, "platforms", "comma-separated platforms, or 'all'")
	fs.Var(&titles, "title", "targeted title id (repeatable)")
	fs.Var(&regions, "region", "targeted ISO country (repeatable)")
	fs.Var(&locales, "locale", "LANG=Title (repeatable; body from -body-file/-body-text)")
	fs.Var(&localeFiles, "locale-file", "LANG=Title:body-file (repeatable)")
	bodyFile := fs.String("body-file", "", "body file for -locale")
	bodyText := fs.String("body-text", "", "body text for -locale")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *slug == "" {
		return fmt.Errorf("-slug is required")
	}
	req := &newsv1.CreateArticleRequest{
		Slug: *slug,
		Audience: &newsv1.Audience{
			Platforms: normalizePlatformList(platforms.items),
			TitleIds:  titles.items,
			Regions:   regions.items,
		},
	}
	add := func(spec string, body string) error {
		language, title, found := strings.Cut(spec, "=")
		if !found || title == "" {
			return fmt.Errorf("locale must be LANG=Title: %s", spec)
		}
		req.Locales = append(req.Locales, &newsv1.ArticleLocale{Language: language, Title: title, Body: body})
		return nil
	}
	for _, spec := range locales.items {
		var body string
		if *bodyFile != "" {
			b, err := os.ReadFile(*bodyFile)
			if err != nil {
				return err
			}
			body = string(b)
		} else {
			body = *bodyText
		}
		if err := add(spec, body); err != nil {
			return err
		}
	}
	for _, spec := range localeFiles.items {
		head, file, found := strings.Cut(spec, ":")
		if !found {
			return fmt.Errorf("locale-file must be LANG=Title:FILE: %s", spec)
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if err := add(head, string(b)); err != nil {
			return err
		}
	}
	if len(req.Locales) == 0 {
		return fmt.Errorf("at least one -locale or -locale-file is required")
	}
	a, err := c.CreateArticle(ctx, req)
	if err != nil {
		return err
	}
	fmt.Printf("created %s (%s)\n", a.Id, a.Slug)
	return nil
}

func normalizePlatformList(items []string) []string {
	for _, p := range items {
		if p == "all" {
			return nil
		}
	}
	return items
}

func articlePublish(ctx context.Context, c newsv1.NewsClient, args []string) error {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	id := fs.String("id", "", "")
	at := fs.String("at", "", "")
	expires := fs.String("expires", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	req := &newsv1.PublishArticleRequest{Id: *id}
	if *at != "" {
		t, err := time.Parse(time.RFC3339, *at)
		if err != nil {
			return err
		}
		req.PublishedAt = t.Format(time.RFC3339)
	}
	if *expires != "" {
		t, err := time.Parse(time.RFC3339, *expires)
		if err != nil {
			return err
		}
		req.ExpiresAt = t.Format(time.RFC3339)
	}
	a, err := c.PublishArticle(ctx, req)
	if err != nil {
		return err
	}
	fmt.Printf("published %s at %s\n", a.Id, a.PublishedAt)
	return nil
}

func articleList(ctx context.Context, c newsv1.NewsClient, args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	all := fs.Bool("all", false, "")
	platform := fs.String("platform", "", "")
	title := fs.String("title", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out, err := c.ListArticles(ctx, &newsv1.ListArticlesRequest{
		IncludeUnpublished: *all, Platform: *platform, TitleId: *title,
	})
	if err != nil {
		return err
	}
	for _, a := range out.Articles {
		state := "published " + a.PublishedAt
		if a.PublishedAt == "" {
			state = "draft"
		}
		fmt.Printf("%s  %-20s %-10s %s\n", a.Id, a.Slug, state, formatAudience(a.Audience))
	}
	return nil
}

func formatAudience(a *newsv1.Audience) string {
	f := func(v []string, all string) string {
		if len(v) == 0 {
			return all
		}
		return strings.Join(v, ",")
	}
	return fmt.Sprintf("platforms=%s titles=%s regions=%s languages=%s",
		f(a.Platforms, "all"), f(a.TitleIds, "all"), f(a.Regions, "all"), f(a.Languages, "all"))
}

func articlePreview(ctx context.Context, c newsv1.NewsClient, args []string) error {
	fs := flag.NewFlagSet("preview", flag.ExitOnError)
	id := fs.String("id", "", "")
	slug := fs.String("slug", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out, err := c.RenderPreview(ctx, &newsv1.ArticleRef{Id: *id, Slug: *slug})
	if err != nil {
		return err
	}
	fmt.Print(out.Html)
	return nil
}

func imageUpload(ctx context.Context, c newsv1.NewsClient, args []string) error {
	fs := flag.NewFlagSet("image", flag.ExitOnError)
	id := fs.String("id", "", "")
	file := fs.String("file", "", "")
	language := fs.String("language", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	img, err := c.UploadImage(ctx, &newsv1.UploadImageRequest{
		ArticleId: *id, Language: *language, FileName: *file, Content: data,
	})
	if err != nil {
		return err
	}
	fmt.Printf("uploaded %s (%d bytes) %s\n", img.Url, img.Size, img.Sha256[:12])
	return nil
}

func datasetCreate(ctx context.Context, c newsv1.NewsClient, args []string) error {
	fs := flag.NewFlagSet("dscreate", flag.ExitOnError)
	title := fs.String("title", "", "")
	from := fs.String("from", "", "")
	until := fs.String("until", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	d, err := c.CreateDataset(ctx, &newsv1.CreateDatasetRequest{TitleId: *title, ValidFrom: *from, ValidUntil: *until})
	if err != nil {
		return err
	}
	fmt.Printf("dataset %s for %s (valid until %s)\n", d.Id, d.TitleId, d.ValidUntil)
	return nil
}

func datasetPut(ctx context.Context, c newsv1.NewsClient, args []string) error {
	fs := flag.NewFlagSet("dsput", flag.ExitOnError)
	dataset := fs.String("dataset", "", "")
	path := fs.String("path", "", "")
	file := fs.String("file", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	f, err := c.PutDatasetFile(ctx, &newsv1.PutDatasetFileRequest{DatasetId: *dataset, Path: *path, Content: data})
	if err != nil {
		return err
	}
	fmt.Printf("put %s (%d bytes, sha256 %s…)\n", f.Path, f.Size, short(f.Sha256))
	return nil
}

func datasetSeal(ctx context.Context, c newsv1.NewsClient, args []string) error {
	fs := flag.NewFlagSet("dsseal", flag.ExitOnError)
	dataset := fs.String("dataset", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	d, err := c.SealDataset(ctx, &newsv1.SealDatasetRequest{DatasetId: *dataset})
	if err != nil {
		return err
	}
	fmt.Printf("sealed %s: %d files\n", d.Id, len(d.Files))
	return nil
}

func datasetList(ctx context.Context, c newsv1.NewsClient, args []string) error {
	fs := flag.NewFlagSet("dslist", flag.ExitOnError)
	title := fs.String("title", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out, err := c.ListDatasets(ctx, &newsv1.ListDatasetsRequest{TitleId: *title})
	if err != nil {
		return err
	}
	for _, d := range out.Datasets {
		fmt.Printf("%s  %s  from=%s until=%s sealed=%v files=%d\n",
			d.Id, d.TitleId, d.ValidFrom, d.ValidUntil, d.Sealed, len(d.Files))
	}
	return nil
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

type multi struct{ items []string }

func (m *multi) String() string { return strings.Join(m.items, ",") }
func (m *multi) Set(v string) error {
	m.items = append(m.items, v)
	return nil
}

type localeList struct{ items []string }

func (l *localeList) String() string { return strings.Join(l.items, ",") }
func (l *localeList) Set(v string) error {
	l.items = append(l.items, v)
	return nil
}
