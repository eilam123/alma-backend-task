FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /app main.go

FROM alpine:3.21
COPY --from=build /app /app
COPY --from=build /src/data /data
ENTRYPOINT ["/app"]
