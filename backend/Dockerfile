FROM golang:1.22-bookworm AS build
WORKDIR /app
COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /app/backend
RUN go mod download
COPY backend /app/backend
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/senti-api ./cmd/api

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tesseract-ocr tesseract-ocr-chi-sim tesseract-ocr-eng \
  && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /app/senti-api /usr/local/bin/senti-api
COPY chat-skills /app/chat-skills
RUN mkdir -p /app/uploads
EXPOSE 8080
CMD ["senti-api"]

