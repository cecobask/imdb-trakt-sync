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
    wget -q https://storage.googleapis.com/chrome-for-testing-public/131.0.6778.204/linux64/chrome-linux64.zip && \
    unzip -qq chrome-linux64.zip && \
    rm chrome-linux64.zip
ENV BROWSER_PATH=/app/chrome-linux64/chrome
ENV PATH=$PATH:/app/build
ENTRYPOINT ["its"]
CMD ["sync"]
