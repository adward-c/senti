FROM golang:1.22-bookworm AS build
WORKDIR /app
COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /app/backend
RUN go mod download
COPY backend /app/backend
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/senti-api ./cmd/api

FROM debian:bookworm-slim
RUN printf '%s\n' \
  'Types: deb' \
  'URIs: http://deb.debian.org/debian' \
  'Suites: bookworm' \
  'Components: main' \
  'Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg' \
  '' \
  'Types: deb' \
  'URIs: http://deb.debian.org/debian-security' \
  'Suites: bookworm-security' \
  'Components: main' \
  'Signed-By: /usr/share/keyrings/debian-archive-keyring.gpg' \
  > /etc/apt/sources.list.d/debian.sources \
  && apt-get -o Acquire::Retries=3 update \
  && apt-get install -y --no-install-recommends ca-certificates tesseract-ocr tesseract-ocr-chi-sim tesseract-ocr-eng \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /app/senti-api /usr/local/bin/senti-api
RUN mkdir -p /app/uploads
EXPOSE 8080
CMD ["senti-api"]
