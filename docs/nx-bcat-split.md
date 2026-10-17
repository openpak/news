# Console-facing BCAT lives in nx-bcat

Signed News serving, native game BCAT and the topic push moved to
`platforms/switch/infra/nx-bcat` (its README has the cut-over and rollback). News keeps
authoring, storage, the website JSON, the admin API, the channel dataset and Harbor's public
JSON, and offers nx-bcat an internal API:

- `GET /admin/v1/bcat/current` and `GET /admin/v1/bcat/builds/{n}/seed.tar` on the admin
  listener (`internal/newsadmin/bcat.go`);
- `GET /internal/datasets/{title}` and `GET /internal/objects/{key}` on bcatserve
  (`cmd/bcatserve/internal.go`, `NX_INTERNAL_KEY`).

With `NX_BCAT_SYNC_URL` set, `newscdn -data` publishes the unsigned seed and tells nx-bcat to
pull it: no signing key, no console tree and no push from this process. Unset, everything works
as before. `render/nativebcat` and the signing code are copied, not removed, until the cut-over
has run for a while.
