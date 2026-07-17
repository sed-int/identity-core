# Build from the repo root: docker build -f services/board/Dockerfile .
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/board ./services/board/cmd

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/board /app/board
ENTRYPOINT ["/app/board"]
