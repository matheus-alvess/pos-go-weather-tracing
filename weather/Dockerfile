FROM golang:1.22-alpine

WORKDIR /app

COPY ../go.mod ../go.sum ./

RUN go mod download

COPY . .

#RUN go test ./... -v
RUN go build -o /weather ./weather

EXPOSE 9090

CMD ["/weather"]
