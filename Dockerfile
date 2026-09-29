# syntax=docker/dockerfile:1

FROM golang:1.26.1 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/glidepath ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/glidepath /glidepath
EXPOSE 8080
ENTRYPOINT ["/glidepath"]
