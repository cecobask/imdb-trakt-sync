[![sync](https://github.com/cecobask/imdb-trakt-sync/actions/workflows/sync.yml/badge.svg?event=schedule)](https://github.com/cecobask/imdb-trakt-sync/actions/workflows/sync.yml)  
# imdb-trakt-sync
GoLang app that can sync [IMDb](https://www.imdb.com/) and [Trakt](https://trakt.tv/dashboard) user data - watchlist, ratings and lists.  
For its data needs, the app is communicating with the IMDb and Trakt APIs directly.  
The app can be set up to run on a custom schedule (_default: once every 3 hours_) through GitHub Actions.  

# Usage
The application can be run automatically, based on a custom schedule (_default: once every 3 hours_) using `GitHub Actions` or locally on your machine.  
Follow the relevant section below, based on how you want to use the application. 

## GitHub Actions
1. Fork this repository to your account
2. Configure your GitHub repository secrets using the [.env.example](.env.example) file as reference:
   1. Retrieve the `at-main` and `ubid-main` cookies by logging into your IMDb account and inspecting the cookies using your favourite web browser
   2. [Create Trakt API app](https://trakt.tv/oauth/applications)
   3. Retrieve a Trakt access token:  
      1. Get a Trakt code by opening the Trakt API app that you created in the previous step and click the `Authorize` button
      3. Using the code from the previous step together with your Trakt API app's client id & client secret, replace the values in this request:
      ```shell
      curl --include \
      --request POST \
      --header "Content-Type: application/json" \
      --data-binary "{
      \"code\": \"fd0847dbb559752d932dd3c1ac34ff98d27b11fe2fea5a864f44740cd7919ad0\",
      \"client_id\": \"9b36d8c0db59eff5038aea7a417d73e69aea75b41aac771816d2ef1b3109cc2f\",
      \"client_secret\": \"d6ea27703957b69939b8104ed4524595e210cd2e79af587744a7eb6e58f5b3d2\",
      \"redirect_uri\": \"urn:ietf:wg:oauth:2.0:oob\",
      \"grant_type\": \"authorization_code\"
      }" \
      'https://api.trakt.tv/oauth/token'
      ```
   4. Create repository secrets: `Settings` > `Secrets` > `Actions` > `New repository secret`
3. Enable GitHub Actions for the fork repository
4. Enable the `sync` workflow, as scheduled workflows are disabled by default in fork repositories
5. The `sync` workflow can be triggered manually right away to test if it works. Alternatively, wait for GitHub actions to automatically trigger it every 3 hours

## Run the application locally
1. Clone this repo to your machine
2. Make a copy of the [.env.example](.env.example) file and name it `.env`
3. Populate all the environment variables in that file using the existing values as reference:
   1. Retrieve the `at-main` and `ubid-main` cookies by logging into your IMDb account and inspecting the cookies using your favourite web browser
   2. [Create Trakt API app](https://trakt.tv/oauth/applications)
   3. Retrieve a Trakt access token:
      1. Get a Trakt code by opening the Trakt API app that you created in the previous step and click the `Authorize` button
      3. Using the code from the previous step together with your Trakt API app's client id & client secret, replace the values in this request:
      ```shell
      curl --include \
      --request POST \
      --header "Content-Type: application/json" \
      --data-binary "{
      \"code\": \"fd0847dbb559752d932dd3c1ac34ff98d27b11fe2fea5a864f44740cd7919ad0\",
      \"client_id\": \"9b36d8c0db59eff5038aea7a417d73e69aea75b41aac771816d2ef1b3109cc2f\",
      \"client_secret\": \"d6ea27703957b69939b8104ed4524595e210cd2e79af587744a7eb6e58f5b3d2\",
      \"redirect_uri\": \"urn:ietf:wg:oauth:2.0:oob\",
      \"grant_type\": \"authorization_code\"
      }" \
      'https://api.trakt.tv/oauth/token'
      ```
4. Make sure you have GoLang installed on your machine
5. Open a terminal window in the repository folder and run the application using this command `go run .`