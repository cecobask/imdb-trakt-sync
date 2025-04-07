FROM golang:1.23.4-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -o build/its main.go

FROM ubuntu:24.04
WORKDIR /app
COPY --from=build /app/build ./build
COPY --from=build /app/config.yaml .
ENV PATH=$PATH:/app/build
ENV DEBIAN_FRONTEND=noninteractive
ENV DEBCONF_NOWARNINGS=yes
RUN apt-get update > /dev/null && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    libasound2t64 \
    libgbm1 \
    libgtk-3-0 \
    libnss3 \
    libxss1 \
    libxtst6 \
    unzip \
    wget > /dev/null && \
    rm -rf /var/lib/apt/lists/* && \
    wget -q https://storage.googleapis.com/chromium-browser-snapshots/Linux_x64/1321438/chrome-linux.zip && \
    unzip -q chrome-linux.zip && \
    rm chrome-linux.zip
ENV ITS_IMDB_BROWSERPATH=/app/chrome-linux/chrome
ENTRYPOINT ["its"]
CMD ["sync"]
