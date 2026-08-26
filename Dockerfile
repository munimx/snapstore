FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod ./
COPY main.go ./
RUN go mod tidy && go build -o /out/snapstore .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/snapstore /app/snapstore
EXPOSE 3000
CMD ["/app/snapstore"]
