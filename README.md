<p align="center"><img src="assets/logo.svg" width="96" alt="Refract"></p>

# Refract

A private, single-user LaTeX workspace with an AI assistant, for your own server. Edit, compile with Tectonic and read the PDF side by side; ask the assistant for changes and review them as diffs; find real papers to cite; keep every version in git.

- **Editor and PDF**: CodeMirror, Tectonic, SyncTeX (⌘J / ⌘-click), problems with "Fix with AI".
- **Assistant**: works on a copy of the project and proposes a diff you accept or reject. Quick actions on any selection (improve, shorten, fix grammar, table, explain, free instruction) shown as inline diffs.
- **References**: finds published papers (OpenAlex) and arXiv preprints that support a passage, and places `\cite` for you to review. Bibliography entries always come from the publisher's or arXiv's metadata, never from a model.
- **History and sync**: every saved version is a git commit; restore any file or version; push and pull to GitHub or any https git remote.
- English and Spanish UI, light and dark themes.

The AI uses your ChatGPT plan ("Sign in with ChatGPT"), so there is no API key to pay for.

## Run it

Refract runs as one container (Go server, built web UI, Tectonic and SyncTeX). Build the image from this repository (no prebuilt image is published yet):

```sh
docker build -t refract .
docker run -d -p 8080:8080 -v refract-data:/data \
  -e REFRACT_SECRET_KEY="$(openssl rand -base64 32)" \
  -e REFRACT_CF_TEAM_DOMAIN=yourteam.cloudflareaccess.com -e REFRACT_CF_AUD=<aud tag> \
  refract
```

Keep `REFRACT_SECRET_KEY` stable: it encrypts the stored tokens. Refract has no login of its own and expects to sit behind [Cloudflare Access](https://developers.cloudflare.com/cloudflare-one/applications/), whose JWT it verifies. It refuses to start without that configuration unless you set `REFRACT_AUTH=none` (only on a private network).

### Users

Everyone your Access policy lets in gets their own Refract account, identified by the email in the Access token: their own projects, chats, ChatGPT sign-in and GitHub account. Nothing is shared between users. The first person to sign in adopts the data of an installation from before multi-user support. With `REFRACT_AUTH=none` there is a single local user.

### Connect ChatGPT

Sign-in happens on your own computer. Download `refract` for your system from [Releases](https://github.com/ponytojas/Refract/releases) (or run `make login` from a clone), then:

```sh
refract login
```

Choose *Continue with ChatGPT* in the browser, then upload the credential file it writes in **Settings → Assistant**. Each user uploads their own. To restrict the server to one ChatGPT account, set `REFRACT_ALLOWED_SUBJECT` to the subject `refract login` prints. On macOS, a binary downloaded with a browser is quarantined because it isn't signed: run `xattr -d com.apple.quarantine refract` once.

### Connect GitHub (optional)

Projects can push to and pull from any https git remote with a personal access token. To pick or create GitHub repositories from Refract instead, create a [GitHub OAuth App](https://github.com/settings/developers) with **Enable Device Flow** checked (the callback URL is not used), set `REFRACT_GITHUB_CLIENT_ID` to its client ID, and connect in **Settings → Integrations**. No client secret is needed.

## Configuration

| Variable | Purpose |
|---|---|
| `REFRACT_SECRET_KEY` | **Required.** 32 bytes, base64 (`openssl rand -base64 32`). Encrypts stored credentials and tokens. |
| `REFRACT_DATA_DIR` | Projects, database and caches. Default `/data`. |
| `REFRACT_ADDR` | Listen address. Default `:8080`. |
| `REFRACT_AUTH` | `cloudflare` (default) or `none`. |
| `REFRACT_CF_TEAM_DOMAIN`, `REFRACT_CF_AUD` | Your Cloudflare Access team domain and application AUD tag. |
| `REFRACT_ALLOWED_SUBJECT` | Optional: the only ChatGPT account accepted (printed by `refract login`). |
| `REFRACT_MODEL_AGENT` | Default model for the assistant when none is picked in the UI. |
| `REFRACT_MODEL_QUICK` | Default model for quick actions and citations. |
| `REFRACT_COMPILE_TIMEOUT` | Seconds per compile. Default 90. |
| `REFRACT_GITHUB_CLIENT_ID` | Client ID of your GitHub OAuth App (device flow), to connect a GitHub account. |
| `REFRACT_OPENALEX_API_KEY` | Free [OpenAlex key](https://openalex.org/settings/api). Without it, paper searches share a small free daily budget per IP address. |
| `REFRACT_TECTONIC_BUNDLE` | Pinned Tectonic bundle URL (set in the image). |

## Develop

```sh
make dev     # API on :8080 and Vite on :5173 (REFRACT_AUTH=none, data in ./data)
make test    # Go tests, vet and web unit tests
make e2e     # build and run the Playwright smoke test
make docker  # multi-arch image (amd64, arm64) and a smoke test
```

Releases: pushing a tag like `v0.1.0` builds `refract` for macOS, Linux and Windows (amd64 and arm64) and publishes them with checksums (`.github/workflows/release.yml`).

Requires Go, Node and Tectonic (plus `synctex` for PDF sync) on your machine; the image bundles them.
