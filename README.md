[![sync](https://github.com/cecobask/imdb-trakt-sync/actions/workflows/sync.yml/badge.svg?event=schedule)](https://github.com/cecobask/imdb-trakt-sync/actions/workflows/sync.yml)  
# imdb-trakt-sync
GoLang app that can sync [IMDb](https://www.imdb.com/) and [Trakt](https://trakt.tv/dashboard) user data - watchlist, lists, ratings and history.  
To achieve its goals the application is using the [Trakt API](https://trakt.docs.apiary.io/) and web scraping the IMDb website.

# Usage
The application can be setup to run automatically, based on a custom schedule (_default: once every 3 hours_) using `GitHub Actions` or locally on your machine.  
Follow the relevant section below, based on how you want to use the application. 

## Run the application using GitHub Actions
1. Fork this repository to your account
2. Configure your GitHub repository secrets using the [.env.example](.env.example) file as reference:
   1. Retrieve the `at-main` and `ubid-main` cookies by logging into your IMDb account and inspecting the cookies using your favourite web browser
   2. [Create Trakt API app](https://trakt.tv/oauth/applications)
   3. Retrieve a Trakt access token:  
      1. Get a Trakt code by opening the Trakt API app that you created in the previous step and click the `Authorize` button
      2. Using the code from the previous step along with your Trakt app's client id & client secret, replace the contents in the [request body](https://reqbin.com/veotsc62) and retrieve your access token from the response
   4. Create repository secrets: `Settings` > `Secrets` > `Actions` > `New repository secret`
3. Enable GitHub Actions for the fork repository
4. Enable the `sync` workflow, as scheduled workflows are disabled by default in fork repositories
5. The `sync` workflow can be triggered manually right away to test if it works. Alternatively, wait for GitHub actions to automatically trigger it every 3 hours

## Run the application locally
1. Clone this repository to your machine
2. Make a copy of the [.env.example](.env.example) file and name it `.env`
3. Populate all the environment variables in that file using the existing values as reference:
   1. Retrieve the `at-main` and `ubid-main` cookies by logging into your IMDb account and inspecting the cookies using your favourite web browser
   2. [Create Trakt API app](https://trakt.tv/oauth/applications)
   3. Retrieve a Trakt access token:
      1. Get a Trakt code by opening the Trakt API app that you created in the previous step and click the `Authorize` button
      2. Using the code from the previous step along with your Trakt app's client id & client secret, replace the contents in the [request body](https://reqbin.com/veotsc62) and retrieve your access token from the response
4. Make sure you have GoLang installed on your machine
5. Open a terminal window in the repository folder and run the application using the command `go run .`