# Editable native News sample

Read the [creation guide](../../docs/native-news-authoring-guide.md) before
publishing. These files are an offline example, not a production channel API.

Edit `channel.json` for the channel identity and `article.json` for an article.
Choose a fresh numeric article ID for a new test and keep it stable for updates.
`openpak_news` matches the currently deployed experimental channel. Changing
that ID also requires server route changes; the current CDN hardcodes it.

From the `news` repository, with `uv` and libjpeg-turbo's `cjpeg` installed:

```sh
uv run --with msgpack python examples/native-news/build.py --out /tmp/openpak-news-sample
```

The output directory must not exist. Use `--source /path/to/edited-sample`
to read another pair of JSON files. `--published 2026-09-09T20:00:00Z` makes
timestamp fields reproducible; omit it for fresh samples. The default seven-day
lifetime is converted to UTC Unix seconds. Generated original JPEGs are baseline
JFIF with 4:4:4 sampling: icon 70×70, card 358×201, hero 800×364.

Outputs:

| File | Contents |
| --- | --- |
| `catalog.msgpack` | Array containing one channel record |
| `detail.msgpack` | Channel record with empty `latest_news_urls` |
| `archives.msgpack` | `{na_required: false, data_list: []}` |
| `article.msgpack` | NORMAL article with binary JPEG fields and a body map |
| `article-expired.msgpack` | Same article ID, dates adjusted to an expired interval |
| `icon.jpg`, `card.jpg`, `hero.jpg` | Original image assets embedded where applicable |

The JSON files are authoring inputs. On the wire, use the generated MessagePack;
images are binary values, not JSON file paths or base64 strings. Catalog and
detail use integer Unix timestamps and a boolean `important` field.

The article layout follows the locally proven v0.12 format, but this sample's
OpenPak channel association and server delivery are not hardware-verified.
The archive is empty because its article-entry schema and delivery URL handling
still need implementation. Merely signing `article.msgpack` does not publish it.
An expired replacement is a cleanup candidate, not guaranteed immediate deletion.
The script does not connect to a console, load keys, sign files or deploy anything.
