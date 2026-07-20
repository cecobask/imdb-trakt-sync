[![sync](https://github.com/cecobask/imdb-trakt-sync/actions/workflows/sync.yaml/badge.svg)](https://github.com/cecobask/imdb-trakt-sync/actions/workflows/sync.yaml)
[![quality](https://github.com/cecobask/imdb-trakt-sync/actions/workflows/quality.yaml/badge.svg)](https://github.com/cecobask/imdb-trakt-sync/actions/workflows/quality.yaml)

# imdb-trakt-sync

<img src="./assets/logo.png" alt="logo"/>

Command line application that can sync [IMDb](https://www.imdb.com/) and [Trakt](https://trakt.tv/dashboard) user data - watchlist, lists, ratings and history.  
To achieve its goals the application is using the [Trakt API](https://trakt.docs.apiary.io/) and web scraping.  
Keep in mind that this application is performing one-way sync from IMDb to Trakt. This means that any changes made on IMDb will be reflected on Trakt, but not the other way around.

# Configuration

<table>
    <tr>
        <th>FIELD NAME</th>
        <th>DEFAULT VALUE</th>
        <th>ALLOWED VALUES</th>
        <th>DESCRIPTION</th>
    </tr>
    <tr>
        <td>IMDB_AUTH</td>
        <td>cookies</td>
        <td>
            credentials<br />
            cookies<br />
            none
        </td>
        <td>
            Authentication method to be used for IMDb:<br />
            <code>credentials</code> => IMDB_EMAIL + IMDB_PASSWORD fields required<br />
            <code>cookies</code> => IMDB_COOKIEATMAIN field required<br />
            <code>none</code> => IMDB_LISTS field required
        </td>
    </tr>
    <tr>
        <td>IMDB_EMAIL</td>
        <td>-</td>
        <td>-</td>
        <td>IMDb account email address. Only required when IMDB_AUTH => <code>credentials</code></td>
    </tr>
    <tr>
        <td>IMDB_PASSWORD</td>
        <td>-</td>
        <td>-</td>
        <td>IMDb account password. Only required when IMDB_AUTH => <code>credentials</code></td>
    </tr>
    <tr>
        <td>IMDB_COOKIEATMAIN</td>
        <td>-</td>
        <td>-</td>
        <td>
            Cookie value only required when IMDB_AUTH => <code>cookies</code>. Get the following cookie information from
            your browser:<br />
            <code>name: at-main | domain: .imdb.com</code>
        </td>
    </tr>
    <tr>
        <td>IMDB_LISTS</td>
        <td>-</td>
        <td>-</td>
        <td>
            Array of IMDb list IDs that you would like synced to Trakt. If this array is not specified or empty, all
            IMDb lists on your account will be synced to Trakt. In order to get the ID of an IMDb list, open it from a
            browser - the ID is in the URL with format <code>ls#########</code>. If provided as GitHub secret or
            environment variable, define its values as comma-separated list. Keep in mind the <a
                href="https://forums.trakt.tv/t/personal-list-updates/10170#limits-3">Trakt list limits</a>!
        </td>
    </tr>
    <tr>
        <td>IMDB_IGNOREDLISTS</td>
        <td>-</td>
        <td>-</td>
        <td>
            Array of IMDb list IDs that you do <b>NOT</b> want synced to Trakt. This is useful if you would like to
            sync all your lists, but ignore some. In order to get the ID of an IMDb list, open it from a browser - the
            ID is in the URL with format <code>ls#########</code>. If provided as GitHub secret or environment variable,
            define its values as comma-separated list.
        </td>
    </tr>
    <tr>
        <td>IMDB_TRACE</td>
        <td>false</td>
        <td>
            true<br />
            false
        </td>
        <td>Print tracing logs related to browser activities. Can be useful for debugging purposes</td>
    </tr>
    <tr>
        <td>IMDB_HEADLESS</td>
        <td>true</td>
        <td>
            true<br />
            false
        </td>
        <td>
            Whether to run the browser in headless mode or not. Only set this to false when running the syncer locally
        </td>
    </tr>
    <tr>
        <td>IMDB_BROWSERPATH</td>
        <td>-</td>
        <td>-</td>
        <td>
            The location of your preferred web browser. If you leave this value empty, the syncer will attempt to lookup
            common browser locations. You can optionally override its value to use a specific browser
        </td>
    </tr>
    <tr>
        <td>SYNC_MODE</td>
        <td>dry-run</td>
        <td>
            full<br />
            add-only<br />
            dry-run
        </td>
        <td>
            Sync mode to be used when running the application:<br />
            <code>full</code> => add Trakt items that don't exist, delete Trakt items that don't exist on IMDb,
            update<br />
            Trakt items by treating IMDb as the source of truth<br />
            <code>add-only</code> => add Trakt items that do not exist, but do not delete anything<br />
            <code>dry-run</code> => identify what Trakt items would be added / deleted / updated
        </td>
    </tr>
    <tr>
        <td>SYNC_HISTORY</td>
        <td>false</td>
        <td>
            true<br />
            false
        </td>
        <td>Whether to sync history or not. When IMDB_AUTH => <code>none</code>, history sync will be skipped</td>
    </tr>
    <tr>
        <td>SYNC_RATINGS</td>
        <td>true</td>
        <td>
            true<br />
            false
        </td>
        <td>Whether to sync ratings or not. When IMDB_AUTH => <code>none</code>, ratings sync will be skipped</td>
    </tr>
    <tr>
        <td>SYNC_WATCHLIST</td>
        <td>true</td>
        <td>
            true<br />
            false
        </td>
        <td>Whether to sync watchlist or not. When IMDB_AUTH => <code>none</code>, watchlist sync will be skipped</td>
    </tr>
    <tr>
        <td>SYNC_LISTS</td>
        <td>true</td>
        <td>
            true<br />
            false
        </td>
        <td>Whether to sync lists or not. This provides the option to disable syncing of lists</td>
    </tr>
    <tr>
        <td>SYNC_TIMEOUT</td>
        <td>15m</td>
        <td>-</td>
        <td>
            Maximum duration to run the syncer. Users with large libraries might have to increase the timeout value
            accordingly. Valid time units are: ns, us (or µs), ms, s, m, h
        </td>
    </tr>
    <tr>
        <td>TRAKT_CLIENTID</td>
        <td>-</td>
        <td>-</td>
        <td>Trakt app client ID</td>
    </tr>
    <tr>
        <td>TRAKT_CLIENTSECRET</td>
        <td>-</td>
        <td>-</td>
        <td>Trakt app client secret</td>
    </tr>
    <tr>
        <td>TRAKT_TOKENFILE</td>
        <td>trakt-token.json</td>
        <td>-</td>
        <td>
            Path to the file used to store the Trakt access/refresh token. Created automatically the first time the
            application authorizes with Trakt (see <a href="#usage">Usage</a>) and kept up to date automatically
            afterwards
        </td>
    </tr>
</table>

Trakt no longer supports signing in with an email and password from third-party applications - the application
authorizes using Trakt's [device authorization flow](https://trakt.tv/oauth/applications) instead. The first time the
application runs without an existing token file, it prints a verification URL and a short code, e.g.:

```
trakt authorization required: open the verification url in a browser, sign in, and enter the code to continue url=https://trakt.tv/activate code=ABCD-1234 expiresInSeconds=600
```

Open the URL in any browser, sign in however you normally would (Google, Apple, or an email sign-in link), and enter
the code. The application polls in the background and, once approved, saves the resulting token to `TRAKT_TOKENFILE`
so subsequent runs don't need to repeat this step. The refresh token rotates every time it's used, so treat the
token file as live state rather than a static secret - don't regenerate it by hand.

# Usage

The application can be setup to run automatically, based on a custom schedule (_default: once every 12 hours_) using **GitHub Actions**, in a container, or locally on your machine.  
Workflow schedules can be tweaked by editing the [.github/workflows/sync.yaml](.github/workflows/sync.yaml) file and committing the changes.  
Please configure the application to suits your needs, by referring to the [Configuration](#configuration) section, before running it.  
Follow the relevant section below, based on how you want to use the application.

## Run the application using GitHub Actions

1. [Fork the repository](https://github.com/cecobask/imdb-trakt-sync/fork) to your account
2. Create a [Trakt App](https://trakt.tv/oauth/applications). Use **urn:ietf:wg:oauth:2.0:oob** as redirect uri
3. Run the application locally once (see [Run the application locally](#run-the-application-locally)) using the same
   `TRAKT_CLIENTID`/`TRAKT_CLIENTSECRET` to complete the one-time device authorization described in
   [Configuration](#configuration). This creates a token file (`trakt-token.json` by default)
4. Configure the application:
   - Open your fork repository on GitHub
   - Create an individual repository secret for each [Configuration](#configuration) field you need: `Settings` > `Secrets and variables` > `Actions` > `New repository secret`
   - Create a `TRAKT_TOKEN` repository secret with the contents of the token file generated in step 3
   - Create a `GH_PAT` repository secret containing a personal access token with permission to write Actions secrets on this repository - see [Creating the GH_PAT secret](#creating-the-gh_pat-secret) below. This is required because Trakt rotates the refresh token on every use - the workflow updates the `TRAKT_TOKEN` secret with the new value after each run, so future scheduled runs keep working unattended
5. Allow GitHub Actions on your fork repository: `Settings` > `Actions` > `General` > `Allow all actions and reusable workflows`
6. Enable the **sync** workflow: `Actions` > `Workflows` > `sync` > `Enable workflow`
7. Run the **sync** workflow manually: `Actions` > `Workflows` > `sync` > `Run workflow`
8. From now on, GitHub Actions will automatically trigger the **sync** workflow based on your schedule

### Creating the GH_PAT secret

The **sync** workflow needs to overwrite the `TRAKT_TOKEN` repository secret after every run, because Trakt issues a
new refresh token each time the old one is used - the token from step 3 above is only good for one run unless it
keeps getting refreshed and saved back. The default `GITHUB_TOKEN` that Actions provides automatically cannot modify
repository secrets, so a personal access token (PAT) with that specific permission is needed instead. A
fine-grained token scoped to just this repository and just this permission is recommended over a classic token:

1. Go to [github.com/settings/personal-access-tokens/new](https://github.com/settings/personal-access-tokens/new)
   (or navigate manually: your GitHub avatar > `Settings` > `Developer settings` > `Personal access tokens` >
   `Fine-grained tokens` > `Generate new token`)
2. Give it a name (e.g. `imdb-trakt-sync token rotation`) and an expiration. GitHub will stop rotating the token once
   it expires, at which point sync starts failing again and you'll need to generate a new one and update the
   `GH_PAT` secret
3. **Resource owner**: your account (or the organization that owns the fork)
4. **Repository access**: choose `Only select repositories` and pick your `imdb-trakt-sync` fork
5. **Permissions** > `Repository permissions` > set **Secrets** to `Read and write`. This is the only permission
   needed - everything else can stay `No access`
6. Click `Generate token` and copy the value (`github_pat_...`) - GitHub only shows it once
7. Back in your fork: `Settings` > `Secrets and variables` > `Actions` > `New repository secret`, name it `GH_PAT`,
   and paste the token as the value

If you'd rather use a classic token instead (broader access, but works the same way): `Settings` >
`Developer settings` > `Personal access tokens` > `Tokens (classic)` > `Generate new token (classic)`, select the
`repo` scope, generate it, and save it as the `GH_PAT` secret the same way.

## Run the application in a Docker container

1. Install [Docker](https://www.docker.com/get-started)
2. Clone the repository: `git clone git@github.com:cecobask/imdb-trakt-sync.git`
3. Create a [Trakt App](https://trakt.tv/oauth/applications). Use **urn:ietf:wg:oauth:2.0:oob** as redirect uri
4. Configure the application:
   - Create `.env` file with the same contents as [.env.example](.env.example)
   - Populate the `.env` file with your own secret values
   - All secret keys should have `ITS_` prefix
5. Open a terminal window in the repository folder and then:
   - Build a Docker image: `make package`
   - Run the sync workflow in a Docker container: `make sync-container`
   - The first run prints a Trakt verification URL and code - open it in a browser and approve it (see
     [Configuration](#configuration)). The resulting token is persisted to `trakt-token.json` on the host via a
     mounted volume, so later runs of `make sync-container` reuse it without prompting again

## Run the application locally

1. Install [Git](https://git-scm.com/downloads) and [Go](https://go.dev/doc/install)
2. Clone the repository: `git clone git@github.com:cecobask/imdb-trakt-sync.git`
3. Create a [Trakt App](https://trakt.tv/oauth/applications). Use **urn:ietf:wg:oauth:2.0:oob** as redirect uri
4. Open a terminal window in the repository folder and then:
   - Build the syncer: `make build`
   - Configure the syncer: `make configure`
   - Run the syncer: `make sync`
   - The first run prints a Trakt verification URL and code - open it in a browser and approve it (see
     [Configuration](#configuration)). The resulting token is saved to `trakt-token.json`, so later runs reuse it
     without prompting again
