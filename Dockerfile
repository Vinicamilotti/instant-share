# syntax=docker/dockerfile:1

FROM archlinux:base-devel AS builder

WORKDIR /app

RUN pacman -Syu --noconfirm && pacman -S --noconfirm go nodejs npm

COPY web/package.json web/package-lock.json web/
RUN cd web && npm ci

COPY . .
RUN cd web && npm run build \
    && cd /app \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o instant-share .

FROM scratch

COPY --from=builder /app/instant-share /usr/local/bin/instant-share
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENV INSTANT_SHARE_ADDR=0.0.0.0:8080

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/instant-share"]