# --- web UI ---------------------------------------------------------------
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npx ng build

# --- server -----------------------------------------------------------------
FROM golang:1.27-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/heliocontrol ./cmd/heliocontrol

# --- runtime ----------------------------------------------------------------
FROM alpine:3.22
RUN apk add --no-cache tzdata ca-certificates && adduser -D -u 1000 helio
USER helio
WORKDIR /data
COPY --from=server /out/heliocontrol /usr/local/bin/heliocontrol
ENV TZ=Europe/Vienna
EXPOSE 8080
VOLUME /data
ENTRYPOINT ["heliocontrol", "-config", "/data/heliocontrol.yaml"]
