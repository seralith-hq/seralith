# seralith.com

The marketing site. Plain HTML and CSS, no build step, no JavaScript, and no
third-party requests.

```sh
python3 -m http.server -d site 8000
```

## Deployment

`.github/workflows/site.yml` deploys this directory to Cloudflare Pages:

| Trigger | Result |
| --- | --- |
| Push to `main` touching `site/` | Production deploy to `seralith.com` |
| Pull request touching `site/` | Preview deploy at a unique `*.seralith.pages.dev` URL |
| Manual run (Actions → Site → Run workflow) | Deploys the selected branch |

Until the repository secrets below exist, the workflow skips the deploy with a
notice instead of failing.

### One-time setup

1. **API token.** Cloudflare dashboard → My Profile → API Tokens → Create
   Token → Create Custom Token. Name it `github-actions-pages`, add the
   permission **Account → Cloudflare Pages → Edit**, limit it to your account,
   and create it.
2. **Account ID.** Cloudflare dashboard → Workers & Pages. The Account ID is in
   the right-hand sidebar.
3. **Secrets.** GitHub → `seralith` → Settings → Secrets and variables →
   Actions → New repository secret: `CLOUDFLARE_API_TOKEN` and
   `CLOUDFLARE_ACCOUNT_ID`.
4. **First deploy.** Actions → Site → Run workflow on `main`. The workflow
   creates the `seralith` Pages project if it doesn't exist.
5. **Domain.** Cloudflare → Workers & Pages → `seralith` → Custom domains → Set
   up a custom domain → `seralith.com`, then again for `www.seralith.com`.
   Because the domain's DNS is already on Cloudflare, the records and
   certificates are created automatically.

If the organization restricts which actions may run, allow `cloudflare/*` under
Settings → Actions → General.

## Files

| File | Purpose |
| --- | --- |
| `index.html`, `styles.css` | The landing page |
| `security.html` | Vulnerability disclosure policy, served at `/security` |
| `404.html` | Served for unknown paths |
| `_headers` | Security headers (CSP, HSTS, framing, referrer, permissions) and caching |
| `.well-known/security.txt` | RFC 9116 security contact; renew `Expires` before 2027-11-25 |
| `robots.txt`, `sitemap.xml` | Crawler guidance |
| `favicon.*`, `apple-touch-icon.png`, `logo.svg`, `og-image.png` | Generated from `brand/`; do not edit by hand |

The Content-Security-Policy forbids scripts and inline styles. Keep styles in
`styles.css`.
