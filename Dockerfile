FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -o /hailmary .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /hailmary /app/hailmary
EXPOSE 4815
VOLUME ["/app/data"]
ENV DB_PATH=/app/data/notes.db PORT=4815
CMD ["/app/hailmary"]
