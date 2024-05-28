FROM golang:1.22-alpine

WORKDIR /app

COPY ../go.mod ./

RUN go mod download

COPY . .

#RUN go test ./... -v
RUN go build -o /zip_code ./zip_code

EXPOSE 8080

CMD ["/zip_code"]
